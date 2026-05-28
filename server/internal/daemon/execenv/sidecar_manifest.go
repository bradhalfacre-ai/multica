package execenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// sidecarManifestFile is the on-disk JSON Prepare writes into envRoot to
// record every file and intermediate directory it created inside WorkDir.
// CleanupSidecars reads it back to roll the workdir to its pre-Prepare
// state. The file lives in envRoot (daemon scratch), never in WorkDir,
// so a local_directory run does not litter the user's repo with the
// bookkeeping file used to undo the litter.
const sidecarManifestFile = ".multica_sidecar_manifest.json"

// sidecarManifest records the filesystem mutations writeContextFiles and
// its callees make inside the agent's WorkDir for a single task. The
// manifest is the second half of the contract that makes local_directory
// runs byte-exactly reversible:
//
//   - Files lists absolute paths of regular files we created. A file is
//     recorded only when it did NOT pre-exist; if the user already had a
//     file at the same path (a colliding skill name, for example) we will
//     have just overwritten it, but recording it would let Cleanup
//     compound the damage by deleting the user's path on the way out.
//   - Dirs lists absolute paths of directories we created, in root-first
//     creation order. Cleanup walks the list in reverse so deepest dirs
//     get tried first; rmdir of a directory the user has populated since
//     (e.g. .claude/skills/my-own-skill alongside our .claude/skills/
//     issue-review) fails ENOTEMPTY and is skipped silently — the
//     user's content is preserved without any per-dir bookkeeping. A
//     directory is recorded only when it did NOT pre-exist for the same
//     reason files are conditional.
//
// The manifest is intentionally minimal: it carries the paths needed to
// reverse our writes and nothing else. It is not a log of every operation
// and is not a substitute for the runtime config marker block, which has
// its own dedicated round-trip mechanism in runtime_config.go (the brief
// is appended to user-owned content rather than written into a new sidecar
// directory).
type sidecarManifest struct {
	Files []string `json:"files,omitempty"`
	Dirs  []string `json:"dirs,omitempty"`
}

// recordMkdirAll behaves like os.MkdirAll(path, perm) but additionally
// records every parent directory it had to create (skipping any that
// already existed) into m so CleanupSidecars can rmdir them later. The
// recorded paths are appended in root-first order; Cleanup iterates in
// reverse so the deepest directory is removed first.
//
// When m is nil this is identical to os.MkdirAll — the Reuse path uses
// the nil mode because Reuse runs on cloud workdirs that the GC loop
// wipes wholesale, so per-file cleanup is irrelevant and tracking the
// dirs would just leave stale manifest bytes around.
func recordMkdirAll(path string, perm os.FileMode, m *sidecarManifest) error {
	if path == "" {
		return os.MkdirAll(path, perm)
	}
	if m == nil {
		return os.MkdirAll(path, perm)
	}
	// Walk leaf-first, collecting ancestors that don't currently exist.
	// We stop at the first existing ancestor (or the filesystem root) so
	// pre-existing user directories are never recorded — Cleanup must
	// not rmdir a path the user owned before this task started.
	var toCreate []string
	cur := filepath.Clean(path)
	for {
		if _, err := os.Lstat(cur); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("stat ancestor %s: %w", cur, err)
		}
		toCreate = append(toCreate, cur)
		parent := filepath.Dir(cur)
		if parent == cur || parent == "." {
			break
		}
		cur = parent
	}
	if err := os.MkdirAll(path, perm); err != nil {
		return err
	}
	// Reverse leaf-first → root-first so Cleanup can reverse-iterate
	// to peel directories from the leaves upward.
	for i, j := 0, len(toCreate)-1; i < j; i, j = i+1, j-1 {
		toCreate[i], toCreate[j] = toCreate[j], toCreate[i]
	}
	m.Dirs = append(m.Dirs, toCreate...)
	return nil
}

// recordWriteFile writes data to path with perm and records the path in
// m, BUT only when the file did not pre-exist. A pre-existing file at
// the same path means the user owns the path (most commonly: a skill
// name collision with a user-installed skill, or a user-authored
// issue_context.md they happened to keep around) — Cleanup must leave
// that path alone on the way out, even though we just clobbered its
// contents. We can't restore the original bytes (they're gone), but we
// can at least avoid deleting the file outright on cleanup.
//
// When m is nil this collapses to a plain os.WriteFile, matching the
// Reuse path's nil-mode (see recordMkdirAll for the rationale).
func recordWriteFile(path string, data []byte, perm os.FileMode, m *sidecarManifest) error {
	if m == nil {
		return os.WriteFile(path, data, perm)
	}
	_, statErr := os.Lstat(path)
	preExisted := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("stat target %s: %w", path, statErr)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return err
	}
	if !preExisted {
		m.Files = append(m.Files, path)
	}
	return nil
}

// writeSidecarManifest persists m to {envRoot}/{sidecarManifestFile}.
// Empty manifests are still written so a later Cleanup that finds the
// file knows tracking was attempted (vs. an old build that predates this
// mechanism, where the file is absent and Cleanup must no-op). Failures
// are returned to the caller; the caller treats them as non-fatal because
// a missed manifest only degrades local_directory cleanup, not task
// execution.
func writeSidecarManifest(envRoot string, m *sidecarManifest) error {
	if envRoot == "" {
		return nil
	}
	if m == nil {
		m = &sidecarManifest{}
	}
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal sidecar manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(envRoot, sidecarManifestFile), data, 0o644)
}

// CleanupSidecars rolls the user's workdir back to its pre-Prepare
// state by removing every file the manifest at envRoot records and
// then rmdir-ing every directory it records, deepest first. A
// directory the user has populated since (sibling content, a manually
// added file) is left in place because rmdir on a non-empty directory
// fails ENOTEMPTY; we treat that as "user owns this — stop here" and
// move on without an error.
//
// The function is a no-op when:
//   - envRoot is empty (no daemon scratch for this task),
//   - the manifest file is missing (older build, or Prepare did not run),
//   - the manifest is empty (nothing to undo).
//
// Best-effort by design: missing files and ENOTEMPTY rmdir failures are
// silently swallowed. A genuine I/O error on a recorded path (permission
// denied, busy filesystem) is returned as the first error encountered,
// but cleanup continues for the rest so a single bad path can't strand
// the manifest.
//
// Pair this with CleanupRuntimeConfig on the local_directory cleanup
// path: that function handles the runtime brief inside CLAUDE.md /
// AGENTS.md / GEMINI.md, this one handles the sidecar tree
// (.agent_context/, .multica/, .claude/skills/, .github/skills/,
// .opencode/skills/, skills/, .pi/skills/, .cursor/skills/,
// .kimi/skills/, .kiro/skills/, .agents/skills/, fallback
// .agent_context/skills/). The two together restore the workdir to
// byte-exact pre-task state.
func CleanupSidecars(envRoot string) error {
	if envRoot == "" {
		return nil
	}
	manifestPath := filepath.Join(envRoot, sidecarManifestFile)
	data, err := os.ReadFile(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read sidecar manifest %s: %w", manifestPath, err)
	}
	var m sidecarManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse sidecar manifest %s: %w", manifestPath, err)
	}

	var firstErr error
	captureErr := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}

	for _, f := range m.Files {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			captureErr(fmt.Errorf("remove %s: %w", f, err))
		}
	}

	// Reverse iterate so the deepest directory is tried first. A
	// directory the user has filled with their own content fails
	// rmdir with a non-ENOENT, non-clean error — that's the signal
	// "this directory still has user content" and we leave it alone
	// without surfacing an error.
	for i := len(m.Dirs) - 1; i >= 0; i-- {
		d := m.Dirs[i]
		err := os.Remove(d)
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			continue
		}
		// errno-portable "directory not empty" check: any error other
		// than ENOENT here means the user has populated the directory
		// since Prepare ran. Skip silently.
	}

	if err := os.Remove(manifestPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		captureErr(fmt.Errorf("remove manifest %s: %w", manifestPath, err))
	}

	return firstErr
}

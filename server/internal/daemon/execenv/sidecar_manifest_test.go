package execenv

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// walkRelative returns every relative path inside root (files and directories),
// sorted, with a `dir/` suffix on directories so dir-vs-file mismatches show up
// in the diff. The root itself is reported as "." so a fully-empty directory
// still surfaces a non-empty fingerprint and an empty-vs-missing comparison
// fails loudly instead of looking identical.
func walkRelative(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			entries = append(entries, ".")
			return nil
		}
		if d.IsDir() {
			entries = append(entries, rel+string(os.PathSeparator))
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(entries)
	return entries
}

// snapshotWorkdir captures both the directory listing and the content of every
// regular file inside root, so a round-trip assertion can compare "exactly the
// same bytes everywhere" — not just "no orphan files survived". An empty map
// represents an empty workdir.
type workdirSnapshot struct {
	entries []string
	files   map[string]string
}

func snapshot(t *testing.T, root string) workdirSnapshot {
	t.Helper()
	snap := workdirSnapshot{files: map[string]string{}}
	snap.entries = walkRelative(t, root)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		snap.files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk-for-content %s: %v", root, err)
	}
	return snap
}

func assertSnapshotEqual(t *testing.T, label string, want, got workdirSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(want.entries, got.entries) {
		t.Errorf("[%s] directory listing differs\n want: %v\n  got: %v", label, want.entries, got.entries)
	}
	if !reflect.DeepEqual(want.files, got.files) {
		// Find a small diff first so the failure is actionable.
		for k, wv := range want.files {
			gv, ok := got.files[k]
			if !ok {
				t.Errorf("[%s] missing file %s after round-trip", label, k)
				continue
			}
			if wv != gv {
				t.Errorf("[%s] file %s differs\n want: %q\n  got: %q", label, k, wv, gv)
			}
		}
		for k := range got.files {
			if _, ok := want.files[k]; !ok {
				t.Errorf("[%s] orphan file %s after round-trip", label, k)
			}
		}
	}
}

// runPrepareLikeCycle replays the daemon's local_directory path against the
// supplied workDir and envRoot: writes context files (with manifest tracking),
// injects the runtime brief, then runs the matching cleanups. Tests use this
// to assert byte-exact reversibility without booting the full Prepare/Reuse
// pipeline (which would need a WorkspacesRoot, GC plumbing, etc.).
func runPrepareLikeCycle(t *testing.T, workDir, envRoot, provider string, ctx TaskContextForEnv) {
	t.Helper()
	manifest := &sidecarManifest{}
	if err := writeContextFiles(workDir, provider, ctx, manifest); err != nil {
		t.Fatalf("writeContextFiles(%s): %v", provider, err)
	}
	if err := writeSidecarManifest(envRoot, manifest); err != nil {
		t.Fatalf("writeSidecarManifest(%s): %v", provider, err)
	}
	if _, err := InjectRuntimeConfig(workDir, provider, ctx); err != nil {
		t.Fatalf("InjectRuntimeConfig(%s): %v", provider, err)
	}
	// Mirror daemon.go ordering: runtime config first, sidecars second. The
	// order is incidental — neither cleanup touches the other's paths — but
	// pinning the same order in tests catches an accidental coupling.
	if err := CleanupRuntimeConfig(workDir, provider); err != nil {
		t.Fatalf("CleanupRuntimeConfig(%s): %v", provider, err)
	}
	if err := CleanupSidecars(envRoot); err != nil {
		t.Fatalf("CleanupSidecars(%s): %v", provider, err)
	}
}

// allFileBasedProviders lists every provider whose `writeContextFiles` /
// `InjectRuntimeConfig` writes into the user's workDir. Codex is included
// because it still writes AGENTS.md + .agent_context/ into the workdir (its
// skills live in codex-home, but that's not in workdir and is out of scope
// for this manifest). Adding a new provider that writes into workDir must
// also add it here so the round-trip invariant is enforced for it on day
// one — review the test diff before merging.
var allFileBasedProviders = []string{
	"claude",
	"codex",
	"copilot",
	"opencode",
	"openclaw",
	"hermes",
	"pi",
	"cursor",
	"kimi",
	"kiro",
	"antigravity",
	"gemini",
}

// TestPrepareThenCleanupSidecarsRoundTripEmptyWorkdir is the headline
// invariant the issue (MUL-2784) calls out: a user repo that contained
// nothing related to Multica before a task ran must contain nothing
// related to Multica after the task finishes — no .agent_context/,
// no .claude/skills/, no .multica/, no stub directories. The test
// runs the full Prepare → Inject → Cleanup cycle for every file-based
// provider against a fresh empty workdir and asserts the directory is
// byte-exactly empty again.
func TestPrepareThenCleanupSidecarsRoundTripEmptyWorkdir(t *testing.T) {
	t.Parallel()
	for _, provider := range allFileBasedProviders {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			envRoot := t.TempDir()
			before := snapshot(t, workDir)

			ctx := TaskContextForEnv{
				IssueID: "11111111-2222-3333-4444-555555555555",
				AgentSkills: []SkillContextForEnv{
					{
						Name:        "Issue Review",
						Description: "Review GH issues",
						Content:     "Steps to review",
						Files: []SkillFileContextForEnv{
							{Path: "templates/checklist.md", Content: "- [ ] check"},
						},
					},
					{
						Name:    "PR Review",
						Content: "Review PR diffs",
					},
				},
				ProjectID:    "proj-1",
				ProjectTitle: "Demo",
			}

			runPrepareLikeCycle(t, workDir, envRoot, provider, ctx)

			after := snapshot(t, workDir)
			assertSnapshotEqual(t, provider, before, after)
		})
	}
}

// TestPrepareThenCleanupSidecarsPreservesUserSkillSibling pins the
// non-destructive contract from the issue: if the user already keeps a
// hand-authored skill under the same parent directory we use, Cleanup
// must leave it bit-for-bit intact. The user-skill payload is laid down
// BEFORE Prepare runs and snapshotted; after Cleanup the user's skill
// must still exist and the Multica-written sibling must be gone.
func TestPrepareThenCleanupSidecarsPreservesUserSkillSibling(t *testing.T) {
	t.Parallel()
	// One representative case per provider that writes into a
	// provider-native skill directory. Gemini and Hermes don't have a
	// native discovery path; they fall back to .agent_context/skills/,
	// which is also covered (a user-created sibling under there should
	// also survive). Codex is intentionally excluded — its workspace
	// skills don't live in workdir, so the "user skill sibling"
	// scenario doesn't apply.
	cases := []struct {
		provider      string
		userSkillRel  string // path under workDir
		userSkillFile string // path under userSkillRel
	}{
		{"claude", filepath.Join(".claude", "skills", "my-own"), "SKILL.md"},
		{"copilot", filepath.Join(".github", "skills", "my-own"), "SKILL.md"},
		{"opencode", filepath.Join(".opencode", "skills", "my-own"), "SKILL.md"},
		{"openclaw", filepath.Join("skills", "my-own"), "SKILL.md"},
		{"pi", filepath.Join(".pi", "skills", "my-own"), "SKILL.md"},
		{"cursor", filepath.Join(".cursor", "skills", "my-own"), "SKILL.md"},
		{"kimi", filepath.Join(".kimi", "skills", "my-own"), "SKILL.md"},
		{"kiro", filepath.Join(".kiro", "skills", "my-own"), "SKILL.md"},
		{"antigravity", filepath.Join(".agents", "skills", "my-own"), "SKILL.md"},
		{"hermes", filepath.Join(".agent_context", "skills", "my-own"), "SKILL.md"},
		{"gemini", filepath.Join(".agent_context", "skills", "my-own"), "SKILL.md"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			envRoot := t.TempDir()

			userDir := filepath.Join(workDir, tc.userSkillRel)
			if err := os.MkdirAll(userDir, 0o755); err != nil {
				t.Fatalf("seed user skill dir: %v", err)
			}
			userBody := "---\nname: my-own\n---\n\nUser-authored.\n"
			if err := os.WriteFile(filepath.Join(userDir, tc.userSkillFile), []byte(userBody), 0o644); err != nil {
				t.Fatalf("seed user skill file: %v", err)
			}

			before := snapshot(t, workDir)

			ctx := TaskContextForEnv{
				IssueID: "11111111-2222-3333-4444-555555555555",
				AgentSkills: []SkillContextForEnv{
					{Name: "Issue Review", Content: "ours"},
				},
			}
			runPrepareLikeCycle(t, workDir, envRoot, tc.provider, ctx)

			after := snapshot(t, workDir)
			assertSnapshotEqual(t, tc.provider, before, after)

			// Defensive: independently re-read the user skill to make
			// sure no clever cleanup heuristic stripped its content.
			got, err := os.ReadFile(filepath.Join(userDir, tc.userSkillFile))
			if err != nil {
				t.Fatalf("user skill went missing after round-trip: %v", err)
			}
			if string(got) != userBody {
				t.Errorf("user skill content changed\n want: %q\n  got: %q", userBody, string(got))
			}
		})
	}
}

// TestPrepareThenCleanupSidecarsPreservesUnrelatedUserFiles covers the
// case where the user keeps a non-skill file under a parent we end up
// using — for example `.claude/config.json` next to where we drop
// `.claude/skills/`. Cleanup must rmdir only the directories it created;
// pre-existing siblings (and their parents) must survive.
func TestPrepareThenCleanupSidecarsPreservesUnrelatedUserFiles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		provider string
		userFile string // path under workDir
	}{
		{"claude", filepath.Join(".claude", "settings.json")},
		{"copilot", filepath.Join(".github", "CODEOWNERS")},
		{"opencode", filepath.Join(".opencode", "config.json")},
		{"pi", filepath.Join(".pi", "config.toml")},
		{"cursor", filepath.Join(".cursor", "settings.json")},
		{"kimi", filepath.Join(".kimi", "config.json")},
		{"kiro", filepath.Join(".kiro", "config.json")},
		{"antigravity", filepath.Join(".agents", "config.json")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			envRoot := t.TempDir()

			userPath := filepath.Join(workDir, tc.userFile)
			if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
				t.Fatalf("seed user dir: %v", err)
			}
			userBody := "user content " + tc.provider
			if err := os.WriteFile(userPath, []byte(userBody), 0o644); err != nil {
				t.Fatalf("seed user file: %v", err)
			}

			before := snapshot(t, workDir)

			ctx := TaskContextForEnv{
				IssueID: "11111111-2222-3333-4444-555555555555",
				AgentSkills: []SkillContextForEnv{
					{Name: "Issue Review", Content: "ours"},
				},
			}
			runPrepareLikeCycle(t, workDir, envRoot, tc.provider, ctx)

			after := snapshot(t, workDir)
			assertSnapshotEqual(t, tc.provider, before, after)
		})
	}
}

// TestPrepareThenCleanupSidecarsRepeatedCycles guards against the
// failure mode the issue describes most explicitly — every run
// accumulates one more directory layer than the last. Running the cycle
// twice in a row must leave the workdir in the same state as running it
// once (which is the seed state), with the manifest correctly
// regenerated each cycle.
func TestPrepareThenCleanupSidecarsRepeatedCycles(t *testing.T) {
	t.Parallel()
	for _, provider := range allFileBasedProviders {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			envRoot := t.TempDir()
			before := snapshot(t, workDir)

			ctx := TaskContextForEnv{
				IssueID: "11111111-2222-3333-4444-555555555555",
				AgentSkills: []SkillContextForEnv{
					{Name: "Issue Review", Content: "ours"},
				},
			}
			for i := 0; i < 3; i++ {
				runPrepareLikeCycle(t, workDir, envRoot, provider, ctx)
				after := snapshot(t, workDir)
				assertSnapshotEqual(t, provider, before, after)
			}
		})
	}
}

// TestPrepareThenCleanupSidecarsWithProjectResources extends the
// round-trip to the .multica/project/resources.json branch — a separate
// sidecar write that creates its own intermediate directory tree.
func TestPrepareThenCleanupSidecarsWithProjectResources(t *testing.T) {
	t.Parallel()
	for _, provider := range allFileBasedProviders {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			envRoot := t.TempDir()
			before := snapshot(t, workDir)

			ctx := TaskContextForEnv{
				IssueID:      "11111111-2222-3333-4444-555555555555",
				ProjectID:    "proj-1",
				ProjectTitle: "Demo project",
				ProjectResources: []ProjectResourceForEnv{
					{
						ID:           "res-1",
						ResourceType: "github_repo",
						ResourceRef:  []byte(`{"url":"https://github.com/example/repo"}`),
					},
				},
			}
			runPrepareLikeCycle(t, workDir, envRoot, provider, ctx)

			after := snapshot(t, workDir)
			assertSnapshotEqual(t, provider, before, after)
		})
	}
}

// TestCleanupSidecarsNoOpWhenManifestMissing pins backward compatibility
// with envRoots that predate the manifest mechanism (older daemons, GC'd
// scratch dirs, fresh tempdirs). Cleanup must be a silent no-op rather
// than an error when there's nothing to clean.
func TestCleanupSidecarsNoOpWhenManifestMissing(t *testing.T) {
	t.Parallel()
	envRoot := t.TempDir()
	if err := CleanupSidecars(envRoot); err != nil {
		t.Errorf("CleanupSidecars on empty envRoot returned error: %v", err)
	}
	if err := CleanupSidecars(""); err != nil {
		t.Errorf("CleanupSidecars with empty envRoot returned error: %v", err)
	}
}

// TestCleanupSidecarsLeavesUserContentInTrackedDirIntact is the
// directed unit test for the "non-empty rmdir is silently skipped"
// branch. We build a manifest by hand that claims ownership of a
// directory the user later populated; Cleanup must rmdir-skip and
// leave the user's payload in place without surfacing an error.
func TestCleanupSidecarsLeavesUserContentInTrackedDirIntact(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	envRoot := t.TempDir()

	// Imagine Prepare wrote .multica/sidecar.txt and created
	// .multica/ + .multica/project/, then exited. Between Prepare
	// and Cleanup the user dropped their own file under .multica/.
	managedDir := filepath.Join(workDir, ".multica")
	managedProject := filepath.Join(managedDir, "project")
	managedFile := filepath.Join(managedProject, "resources.json")
	if err := os.MkdirAll(managedProject, 0o755); err != nil {
		t.Fatalf("seed dirs: %v", err)
	}
	if err := os.WriteFile(managedFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed managed file: %v", err)
	}
	userFile := filepath.Join(managedDir, "user-notes.txt")
	if err := os.WriteFile(userFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed user file: %v", err)
	}

	manifest := &sidecarManifest{
		Files: []string{managedFile},
		Dirs:  []string{managedDir, managedProject},
	}
	if err := writeSidecarManifest(envRoot, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if err := CleanupSidecars(envRoot); err != nil {
		t.Errorf("CleanupSidecars: %v", err)
	}

	if _, err := os.Stat(managedFile); !os.IsNotExist(err) {
		t.Errorf("managed file %s should be gone, stat err=%v", managedFile, err)
	}
	if _, err := os.Stat(managedProject); !os.IsNotExist(err) {
		t.Errorf("inner managed dir %s should be empty and removed, stat err=%v", managedProject, err)
	}
	// .multica still holds user-notes.txt, so rmdir must have been
	// skipped silently — the directory must survive.
	got, err := os.ReadFile(userFile)
	if err != nil {
		t.Fatalf("user file went missing: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("user file content changed: %q", string(got))
	}
}

// TestCleanupSidecarsDoesNotRemovePreExistingDirs is the directed unit
// test for the "skip recording pre-existing ancestors" branch in
// recordMkdirAll. A directory the user owned before Prepare must NOT
// appear in the manifest, and therefore must NOT be eligible for rmdir
// during Cleanup — even if Cleanup runs after the user removed the
// last file from inside it.
func TestCleanupSidecarsDoesNotRemovePreExistingDirs(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	envRoot := t.TempDir()

	userDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("seed user dir: %v", err)
	}

	manifest := &sidecarManifest{}
	target := filepath.Join(userDir, "skills", "ours")
	if err := recordMkdirAll(target, 0o755, manifest); err != nil {
		t.Fatalf("recordMkdirAll: %v", err)
	}
	for _, d := range manifest.Dirs {
		if d == userDir {
			t.Fatalf("manifest must not record pre-existing user dir %s\nfull dirs: %v", userDir, manifest.Dirs)
		}
	}

	if err := writeSidecarManifest(envRoot, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := CleanupSidecars(envRoot); err != nil {
		t.Fatalf("CleanupSidecars: %v", err)
	}
	if _, err := os.Stat(userDir); err != nil {
		t.Errorf("pre-existing user dir %s removed by cleanup: %v", userDir, err)
	}
}

// TestRecordWriteFileSkipsPreExistingFile pins the matching rule for
// files: if a user already had a file at the target path (a real
// scenario for skill-name collisions), recording would license Cleanup
// to delete the user's path on the way out. The function must NOT
// record pre-existing files — the user's path stays, even though we
// just clobbered its contents.
func TestRecordWriteFileSkipsPreExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "user.md")
	if err := os.WriteFile(target, []byte("user bytes"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m := &sidecarManifest{}
	if err := recordWriteFile(target, []byte("ours"), 0o644, m); err != nil {
		t.Fatalf("recordWriteFile: %v", err)
	}
	for _, f := range m.Files {
		if f == target {
			t.Errorf("manifest must not record pre-existing user file %s", target)
		}
	}
	// And the content was still overwritten — that's the intended
	// behaviour at write time, even though Cleanup will leave the
	// path alone.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "ours" {
		t.Errorf("expected overwrite at write time, got %q", string(got))
	}
}

// TestSidecarManifestRoundTripJSON pins the on-disk encoding so a
// future field rename or json-tag change doesn't silently break Cleanup
// for envRoots that carry an in-flight manifest at the moment of an
// upgrade.
func TestSidecarManifestRoundTripJSON(t *testing.T) {
	t.Parallel()
	envRoot := t.TempDir()
	original := &sidecarManifest{
		Files: []string{"/x/.agent_context/issue_context.md"},
		Dirs:  []string{"/x/.agent_context"},
	}
	if err := writeSidecarManifest(envRoot, original); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(envRoot, sidecarManifestFile))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"files", "dirs", "issue_context.md", ".agent_context"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("manifest JSON missing %q\n got: %s", want, string(raw))
		}
	}
}

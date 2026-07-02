# Team Wiki — a shared, bidirectional knowledge corpus for humans and agents

> Status: Proposed (Draft RFC)
> Last updated: 2026-07-02
> Scope: Multica **platform** feature (workspace-level). Not tied to any single product board.

## TL;DR

- **The gap.** Today an agent's hard-won learnings survive only in what happens to land in a commit message or an issue trail. There is no place where "this is *how* we do X, here is *why* we decided Y, watch out for Z" is written once and read by every other agent and every human in the workspace. Knowledge is re-derived instead of accumulated.
- **The proposal.** A **Team Wiki**: a workspace-scoped, multi-page knowledge corpus that **both humans and agents can read and write**, with revision history, authorship, light curation, and full-text search. It is **shared team memory** — the same instinct as an agent's private per-session memory, generalized to a workspace-shared, human+agent brain.
- **Why it fits Multica cleanly.** Authorship reuses the existing **Polymorphic Actor** pattern (`actor_type` = `member` | `agent`), so "written by a human" and "written by an agent" are the same mechanism that already powers issues and comments. Reading by agents reuses the **Skill**/context-injection and **MCP** machinery. Permissions reuse **Member** roles.
- **Explicitly NOT** OpenWiki-style ungoverned auto-generation. Knowledge enters **deliberately and reviewably**; the wiki captures the **"why"**, not a machine's guess at the "what". It **complements** the authoritative record (ADRs, contracts, decision docs) — it does not replace it.
- **Distinct from Skill and Workspace Context.** Skill = curated, mounted "how-to" injected into an agent's work dir. Workspace Context = one global system-prompt blob. The Wiki = a broad, navigable, bidirectional **living corpus** that humans browse in the UI and agents query on demand. See §3.

---

## 1. Motivation

### 1.1 The problem

Multica already makes agents first-class team members: they create issues, comment, get subscribed, run tasks. But the **knowledge** those agents generate is ephemeral in a specific, costly way:

- **Learnings evaporate.** An agent discovers a non-obvious constraint ("migrations must be append-only here", "this provider is rate-limited after N calls", "the crisis path must never be touched by a latency change"). That insight lives in one task's output. The next agent — or the same agent next week — starts from zero.
- **The "why" is never captured.** Code and auto-generated docs record *what* the system does. They do not record *why* a decision was made, *what* was tried and rejected, or *which* gotchas cost someone a day. That rationale is the most expensive knowledge to reconstruct and the first to be lost.
- **Humans and agents don't share a brain.** A human's tribal knowledge lives in their head or in scattered Slack/issue threads; an agent's lives in transient task logs. There is no common, durable surface both write to and both read from.
- **It compounds.** The more projects and agents in a workspace, the more valuable a shared corpus becomes — and the more painful its absence.

### 1.2 Why not just use what exists?

| Existing surface | What it's good for | Why it isn't this |
|---|---|---|
| **Skill** (`skill`, `skill_file`, `agent_skill`) | Curated "how to do X" **mounted** into an agent's work dir at run time | One-directional (authored *for* agents, not *by* them as a byproduct of work); procedural, not a broad knowledge base; mounted/scoped per-agent rather than browsed by humans |
| **Workspace Context** (`workspace.context`) | A single global system-prompt every agent perceives | One blob, not a navigable multi-page corpus with history and search; not a place to record a specific gotcha or decision |
| **Issue / Comment threads** | The work itself + discussion | Knowledge is buried, un-curated, and un-findable after the issue closes; no notion of "the current, canonical statement of how we do X" |
| **ADRs / contracts / design docs** (in the product repo) | The **hard, reviewed record** of decisions | Heavyweight and gated by design; not the place for the *softer* corpus (conventions, lessons, "watch out for…") that changes often |
| **Per-session agent memory** | An agent persisting its *own* context across sessions | Private to one agent; not shared with the team or other agents |

The Wiki is the missing **middle layer**: lighter than an ADR, broader than a Skill, shared unlike per-session memory, and findable unlike an issue thread.

---

## 2. What it is — design principles

Five properties, each a hard requirement, define the feature:

1. **Bidirectional (human + agent authored).** Any workspace member and any agent can read and write pages. This is the whole point: an agent finishing a task can record "here's what I learned," and a human — or the next agent — reads it. Authorship is tracked via the Polymorphic Actor pattern so provenance is always clear ("last edited by agent *Contrarian*" vs "by *Brad*").

2. **Complementary to the authoritative record, not competing with it.** ADRs, contracts, and formal design docs remain the hard, reviewed record of *decisions*. The Wiki holds the *softer* corpus — conventions, rationale, gotchas, lessons, "how we build." Different layers, and the Wiki should link **out** to the authoritative artifacts rather than duplicate them.

3. **Intent-first.** The content the Wiki is *uniquely* good at is the **"why"** — the reasoning, the rejected alternatives, the context that makes a decision make sense later. This is exactly what code and auto-doc tools cannot capture. Pages should bias toward explaining intent, even to the point of verbosity; that verbosity is the value, not noise.

4. **Curated, so it stays trustworthy.** Knowledge enters *deliberately* and can be *reviewed*. This is the precise inversion of the ungoverned auto-generation risk (the reason OpenWiki-style tools were a poor fit for a governed, auditable workflow). Curation is light — revision history, attribution, and an optional review step — not bureaucratic.

5. **Platform-level and workspace-scoped.** Like every other Multica resource, a page lives inside exactly one workspace and is isolated to it. This is a Multica platform capability (a shared store + a read/write API), not a per-product artifact.

---

## 3. Relationship to per-session memory and Skills (the mental model)

- **Per-session agent memory** = an agent's *private* notebook: persist my own hard-won context so *I* don't re-derive it. Fast, private, unreviewed.
- **Team Wiki** = the *workspace-shared* generalization of that instinct: persist hard-won context so *nobody* — human or agent — re-derives it. Shared, curated, durable, attributed.
- **Skill** = the *productized, mounted* form of a how-to: when a wiki page's "how we do X" stabilizes into a reusable procedure worth injecting into every relevant agent run, it can **graduate** into a Skill. The Wiki is the broad corpus; Skills are the curated, executable subset that gets mounted.

A useful one-liner: **memory is private, Skills are mounted, the Wiki is shared.** The three form a pipeline — an agent's private learning → written to the shared Wiki → (if it stabilizes) promoted to a Skill.

---

## 4. Product surface (UX)

- **A Wiki section per workspace**, reachable at `/{workspace-slug}/wiki/...`, consistent with Multica's workspace-scoped routing.
- **Pages** with title, slug, Markdown body, tags, and a revision history. Optional lightweight hierarchy (folders or parent page) or pure tag-based organization — see Open Questions.
- **Humans edit in the web UI** (Markdown editor with preview), the same way they'd edit a Notion/Linear doc.
- **Agents read and write via an API + tool** (§6). An agent's write shows up in history attributed to that agent, exactly like an agent comment on an issue.
- **Discoverability**: pages are indexed by the existing **Search / command palette** and can be **Pinned** to the sidebar like issues/projects.
- **Notifications**: subscribing to a page (or a tag) surfaces edits in the **Inbox**, reusing the Subscriber/Inbox machinery — so "the deploy runbook changed" reaches the people who care.
- **Provenance is always visible**: every page shows who created it and who last edited it (human or agent), and the full revision timeline.

---

## 5. Data model (proposed — to be reconciled with the live schema)

Illustrative, in Multica's idiom (workspace-scoped, polymorphic actors, append-only history). Exact column/table shapes to be confirmed against the current schema during design.

- **`wiki_page`** — `id`, `workspace_id`, `slug` (unique per workspace), `title`, `body_markdown`, `tags` (text[] or a join table), `created_by_actor_type`/`created_by_actor_id`, `updated_by_actor_type`/`updated_by_actor_id`, `created_at`, `updated_at`, `current_revision_id`, optional `parent_page_id`.
- **`wiki_revision`** — **append-only** history: `id`, `page_id`, `revision_number`, `body_markdown` (or a diff), `edit_summary`, `actor_type`/`actor_id`, `created_at`. This gives attribution, rollback, and an auditable trail. (Optionally a `prev_hash` chain if tamper-evidence is wanted — see §7.)
- **Full-text search** over `title` + `body_markdown` (Postgres FTS or the existing search index), scoped by `workspace_id`.
- **Authorship reuses Polymorphic Actor** (`actor_type` = `member` | `agent`) so no new authorship concept is introduced.
- **Activity**: page create/edit events feed the existing `activity_log` / Timeline so wiki changes appear alongside other workspace activity.

---

## 6. Agent integration

The Wiki is only transformative if agents actually use it. Two directions:

- **Read** — an agent can search and fetch pages relevant to its task. Exposed as:
  - a first-class **tool / MCP tool** (`wiki.search`, `wiki.get`) callable during a run — the natural home given agents already reach external tools via `agent.mcp_config` / MCP; and/or
  - **context injection** at task start (like Skill mounting): relevant pages, or a curated "always-on" page set, are made readable in the work dir.
- **Write** — an agent can create/update pages (`wiki.upsert`) with an edit summary, attributed to the agent actor. Conventions (encouraged via the agent's prompt / a bundled Skill):
  - After solving something non-obvious, **record the decision + the why + the gotcha**.
  - Prefer updating an existing page over creating duplicates; link to the authoritative artifact (ADR/contract) rather than restating it.
  - Never write secrets, credentials, tokens, or regulated data (PHI/PII) into the Wiki — same discipline as everywhere else.

A bundled **"wiki authoring" Skill** can teach agents *when* and *how* to write good, intent-first pages, so the corpus quality stays high without hard gating.

---

## 7. Governance & curation

The feature's credibility depends on knowledge entering **deliberately and reviewably**:

- **Attribution + history** (via `wiki_revision`) make every change accountable and reversible — the baseline governance.
- **Optional review for agent edits.** A workspace setting could route agent-authored page changes through a light approval step (a human or a designated curator confirms) before they become the canonical revision — configurable per workspace, off by default for fast-moving teams, on for higher-assurance ones.
- **No ungoverned auto-generation.** There is no background job that scrapes the repo and invents pages. Every page is written by a specific actor, on purpose. This is the deliberate inverse of the OpenWiki failure mode.
- **Content boundary.** Enforce (and document) that the Wiki holds *knowledge*, never *secrets or regulated data*. Consider a write-time scan for obvious secret patterns, consistent with the platform's existing safety posture.
- **Optional tamper-evidence.** For workspaces that need it, a `prev_hash` chain over revisions yields an audit-grade, append-only record.

---

## 8. Permissions (RBAC)

Reuse **Member** roles (owner / admin / member):

- **Read**: any workspace member (and any agent in the workspace).
- **Write**: members and agents by default; a workspace setting can restrict agent writes to "propose only" (routed through review, §7).
- **Curate / delete / lock a page**: admin/owner (and optionally a "curator" capability).
- **Agent scoping**: an agent's wiki access is bounded by its workspace, exactly like every other resource; no cross-workspace reads.

---

## 9. Non-goals

- **Not** a replacement for ADRs, contracts, or formal design docs — those stay the hard, reviewed record. The Wiki links to them.
- **Not** auto-generated API/code documentation — that's a different tool and a different lifecycle.
- **Not** a general document host / file store (that's Attachments) or a chat surface (that's Chat).
- **Not** a product-board feature — it is platform, and should stay off any specific product's issue board.

---

## 10. Phasing

1. **MVP** — `wiki_page` + `wiki_revision`, workspace-scoped web CRUD (human authoring), full-text search, Polymorphic Actor authorship, activity-log integration.
2. **Agent read** — `wiki.search` / `wiki.get` as a tool/MCP surface + optional task-start injection.
3. **Agent write** — `wiki.upsert` with attribution + the "wiki authoring" Skill + the content-boundary scan.
4. **Curation** — optional review-for-agent-edits setting; Pin + Inbox/Subscriber integration.
5. **Assurance (optional)** — `prev_hash` tamper-evident revision chain for workspaces that require it.

---

## 11. Open questions

- **Organization**: flat + tags, or an explicit page hierarchy (`parent_page_id`)? Tags scale better; hierarchy is more familiar.
- **Storage of revisions**: full body per revision (simple, larger) vs stored diffs (compact, more code).
- **Injection strategy**: always-on page set vs retrieval-per-task vs both — how much wiki context to give an agent by default without bloating the prompt.
- **Skill graduation**: manual "promote this page to a Skill," or a lighter link between a page and a derived Skill?
- **Review default**: should agent writes be direct-commit or propose-then-approve out of the box?

---

## 12. Why now / fork note

Because we run a Multica **fork**, this is buildable and requestable, not hypothetical. It is a platform capability (a shared wiki store + an agent read/write API + a UI), and it should be tracked as platform work — deliberately kept **off** any product's issue board. The payoff scales with the number of projects and agents in the workspace: the shared corpus is the substrate that lets both humans and agents stop re-deriving what someone already learned.

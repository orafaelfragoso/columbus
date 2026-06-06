# Design: Epics & Tasks

Outcome of a grilling session (2026-06-06). This is the locked design for adding
**epics & tasks** to Columbus. It is the source of truth for the implementation
plan; build it with TDD, one vertical slice at a time.

## 1. Identity & scope

Epics/tasks are **structured memory** — a passive durable-knowledge entity
alongside `memory`. The CLI *stores and retrieves* status, history, comments,
summaries, and references. It **never** drives, gates, or enforces transitions.
This stays inside the locked scope boundary (index/search/memory only; no
orchestration). `status` is just a recorded field.

## 2. Model

### Hierarchy
- **Two levels only:** epic → task. No nesting / sub-tasks.
- Every task **requires exactly one epic** (NOT NULL FK).
- **Re-parenting** a task to another epic is allowed (via `edit`).

### Ids
- `epic_NNN` and `task_NNN`, each from its **own monotonic counter**
  (`epic_seq`, `task_seq` added to `index_meta`), **never reused** on delete —
  mirrors `mem_NNN`.

### Fields (at creation)
- `title` (required), `body`/description (optional), `tags` (optional,
  repeatable — feed filter + FTS), `status` (defaults to `todo`).
- Task additionally requires `--epic`.
- Creation **auto-logs an initial event** (`status=todo`, optional comment).
- **No** priority / assignee / due-date in v1 (all additive later).

### Status
- Fixed, validated, **shared** vocabulary: `todo`, `in_progress`, `blocked`,
  `done`, `cancelled`.
- CLI rejects unknown values but enforces **no transition order** (any → any).
  Validating the vocabulary is data validation (like `memory.Kinds`), not
  orchestration.
- `cancelled` is the **soft** "keep the record, won't happen" path.

### History & comments
- One **append-only event log** per entity. Each event:
  `{ts, new_status NULLABLE, comment NULLABLE}` with **≥1 of status/comment
  present**.
  - Status change + note → completion summary case (`status=done`, comment).
  - Comment-only note (status NULL) → progress note.
- **Current status is denormalized** onto the entity row for fast
  `list --status` queries.
- **No separate summary field** — completion summary is the comment on the
  `done` event; full history is the audit trail.
- Timestamps come from `env.Clock` (as memory does).

### References
- Single generic table: `work_refs(owner_type ∈ {epic,task}, owner_id,
  target_type, target_ref)`.
- Target types: **`file`, `dir`, `memory`, `symbol`** (full parity with memory
  links, plus dir).
  - `file` → indexed path.
  - `dir` → path **prefix**; valid if ≥1 indexed file lives under it.
  - `memory` → `mem_NNN`.
  - `symbol` → symbol name.
- Refs are **drift-checked text** (no hard FK cascade). A deleted/missing target
  becomes a **drift warning**, never an error.

### Deletion / lifecycle
- Both get a hard **`delete --force`** (destructive; id retired — mirrors
  `memory remove`).
- Deleting an epic **cascades** its tasks + their events + refs.
- `cancelled` status remains the soft alternative.
- Deleting a referenced memory leaves a **drift warning** (refs are text, not
  cascaded).

## 3. Discoverability

- **Reverse lookup in `show`:** `show file|symbol|memory` lists epics/tasks that
  reference that entity ("what work touches this file?").
- **Global search:** epics/tasks get a dedicated **FTS table** (titles +
  comments + tags); searchable via `search --kind epic|task` and included in
  `--kind all`. FTS reindexes on create/edit/event.

## 4. Command surface (mirrors `memory`)

Mutations + `list` + `search` under the noun; detailed single-entity view under
`show`.

```
columbus epic add   --title ... [--body ...] [--tag ... ]*
columbus epic edit  <id> [--title|--body|--add-tag|--remove-tag ...]
columbus epic delete <id> --force            # cascades tasks/events/refs
columbus epic list  [--status ...] [--tag ...]
columbus epic search <query> [--limit N]
columbus epic status  <id> --to <status> [--comment ...]   # appends event
columbus epic comment <id> --text ...                       # appends note event
columbus epic ref     <id> [--file p|--dir p|--memory mem_N|--symbol name]*
                          [--remove-ref <spec>]*
columbus epic validate                        # bulk ref-drift scan (warnings)

columbus task add   --epic <epic_id> --title ... [--body ...] [--tag ...]*
columbus task edit  <id> [--title|--body|--epic|--add-tag|--remove-tag ...]  # --epic re-parents
columbus task delete <id> --force
columbus task list  [--epic <id>] [--status ...] [--tag ...]
columbus task search <query> [--limit N]
columbus task status  <id> --to <status> [--comment ...]
columbus task comment <id> --text ...
columbus task ref     <id> [--file|--dir|--memory|--symbol ...]* [--remove-ref ...]*
columbus task validate

columbus show epic <id>     # fields + refs (+ inline drift) + full event history + child tasks + reverse-linked work
columbus show task <id>     # fields + refs (+ inline drift) + full event history + reverse-linked work
```

- `edit` mutates **non-historical** metadata only (no event). `status`/`comment`
  are the only verbs that append to the event log.
- `list` default order: **id ascending** (stable/deterministic, matches
  `memory list`). `list` output stays clean — no per-row drift checks.
- `show epic` includes child tasks (each with current status).

## 5. Schema sketch (new migration `0002_epics_tasks.sql`)

```sql
ALTER TABLE index_meta ADD COLUMN epic_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE index_meta ADD COLUMN task_seq INTEGER NOT NULL DEFAULT 0;

CREATE TABLE epics (
    id         INTEGER PRIMARY KEY,            -- numeric part of epic_NNN
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'todo',   -- denormalized current status
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE tasks (
    id         INTEGER PRIMARY KEY,            -- numeric part of task_NNN
    epic_id    INTEGER NOT NULL REFERENCES epics(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'todo',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_tasks_epic ON tasks(epic_id);

-- Shared, polymorphic tag/event/ref tables (owner_type ∈ 'epic'|'task').
CREATE TABLE work_tags (
    owner_type TEXT NOT NULL,
    owner_id   INTEGER NOT NULL,
    tag        TEXT NOT NULL,
    UNIQUE (owner_type, owner_id, tag)
);

CREATE TABLE work_events (
    id         INTEGER PRIMARY KEY,
    owner_type TEXT NOT NULL,
    owner_id   INTEGER NOT NULL,
    new_status TEXT,                           -- NULL = comment-only note
    comment    TEXT,                           -- NULL = status-only change
    created_at TEXT NOT NULL
    -- CHECK (new_status IS NOT NULL OR comment IS NOT NULL)
);
CREATE INDEX idx_work_events_owner ON work_events(owner_type, owner_id);

CREATE TABLE work_refs (
    id          INTEGER PRIMARY KEY,
    owner_type  TEXT NOT NULL,
    owner_id    INTEGER NOT NULL,
    target_type TEXT NOT NULL,                 -- file|dir|memory|symbol
    target_ref  TEXT NOT NULL,
    UNIQUE (owner_type, owner_id, target_type, target_ref)
);
CREATE INDEX idx_work_refs_target ON work_refs(target_type, target_ref); -- reverse lookup
CREATE INDEX idx_work_refs_owner  ON work_refs(owner_type, owner_id);

CREATE VIRTUAL TABLE work_fts USING fts5 (
    title, body, tags, comments,
    owner_type UNINDEXED, owner_id UNINDEXED,
    tokenize = 'unicode61'
);
```

Note: cascade for `tasks` is enforced by FK; `work_tags`/`work_events`/
`work_refs` use no owner FK (polymorphic), so the delete path must explicitly
clean them up under the writer lock. Watch the documented trap: **no reads
inside `WithTx`** (single-conn deadlock) — gather ids first, then mutate.

## 6. Export/import (in scope, complex)

**Unified knowledge doc now.** Extend the versioned `ExportDoc` to carry
memories + epics + tasks + events + refs in one portable document.

- On `import --preserve-ids`: restore ids verbatim (error on collision), advance
  `epic_seq`/`task_seq`/`mem_seq`.
- On `import` **without** `--preserve-ids`: remap `epic`/`task`/`memory` ids to
  freshly allocated ones, then **fix up cross-entity references** — a task's
  `target_type=memory` ref must point at the remapped memory id. This remapping
  is the riskiest logic in the feature → give it its own dedicated TDD pass with
  explicit collision + cross-ref cases.

## 7. Explicitly out of scope (v1)

- Orchestration / execution / transition enforcement (plugin-side, always).
- Sub-tasks / nested epics.
- Priority / assignee / due-date.
- task→task dependencies (refs are file/dir/memory/symbol only).
- Free-form/external (URL/ticket) refs.

## 8. Contract / exit codes

Reuse existing codes: missing entity → `NOT_FOUND`; invalid status / bad args →
`USAGE`; lock contention → `INDEX_LOCKED`/transient. No new exit codes expected.

## 9. Suggested build order (vertical slices, TDD)

1. Migration `0002` + store repos (epics CRUD, seq allocation) — round-trip test.
2. Tasks CRUD + required-epic FK + re-parent + cascade-delete.
3. Tags + `work_events` (status/comment verbs, denormalized status, initial event).
4. `work_refs` (file/dir/memory/symbol) + drift validation + per-noun `validate`.
5. `show epic|task` (detail + history + child tasks + inline drift).
6. Reverse lookup in `show file|symbol|memory`.
7. `work_fts` + global `search --kind epic|task` + `--kind all`.
8. Unified export/import + cross-entity id remapping (own slice, heavy tests).
9. Docs: README command table + exit codes; help examples.
```

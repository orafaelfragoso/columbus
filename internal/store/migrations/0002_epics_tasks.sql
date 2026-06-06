-- 0002_epics_tasks: structured memory. Epics and tasks are passive
-- durable-knowledge entities alongside memories: the store records status,
-- history, comments and references; it never drives or enforces transitions.

-- Per-project monotonic id counters; never reused on delete (mirror mem_seq).
ALTER TABLE index_meta ADD COLUMN epic_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE index_meta ADD COLUMN task_seq INTEGER NOT NULL DEFAULT 0;

-- Epics: id is the numeric part of epic_NNN. status is denormalized from the
-- latest work_events row for fast list --status queries.
CREATE TABLE epics (
    id         INTEGER PRIMARY KEY,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'todo',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Tasks: every task requires exactly one epic (NOT NULL FK). Deleting an epic
-- cascades its tasks; the polymorphic work_* rows are cleaned up explicitly.
CREATE TABLE tasks (
    id         INTEGER PRIMARY KEY,
    epic_id    INTEGER NOT NULL REFERENCES epics(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'todo',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_tasks_epic ON tasks(epic_id);

-- Shared, polymorphic association tables (owner_type IN ('epic','task')). No
-- owner FK: the delete path cleans these up explicitly under the writer lock.
CREATE TABLE work_tags (
    owner_type TEXT NOT NULL,
    owner_id   INTEGER NOT NULL,
    tag        TEXT NOT NULL,
    UNIQUE (owner_type, owner_id, tag)
);

-- Append-only event log: one row per status change and/or comment. At least one
-- of new_status / comment is present (enforced in the writer).
CREATE TABLE work_events (
    id         INTEGER PRIMARY KEY,
    owner_type TEXT NOT NULL,
    owner_id   INTEGER NOT NULL,
    new_status TEXT,
    comment    TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_work_events_owner ON work_events(owner_type, owner_id);

-- Drift-checked text references (file|dir|memory|symbol). No hard FK: a missing
-- target is a drift warning, never an error.
CREATE TABLE work_refs (
    id          INTEGER PRIMARY KEY,
    owner_type  TEXT NOT NULL,
    owner_id    INTEGER NOT NULL,
    target_type TEXT NOT NULL,
    target_ref  TEXT NOT NULL,
    UNIQUE (owner_type, owner_id, target_type, target_ref)
);
CREATE INDEX idx_work_refs_target ON work_refs(target_type, target_ref);
CREATE INDEX idx_work_refs_owner  ON work_refs(owner_type, owner_id);

-- FTS5 over epic/task title + body + tags + comments for global search.
CREATE VIRTUAL TABLE work_fts USING fts5 (
    title,
    body,
    tags,
    comments,
    owner_type UNINDEXED,
    owner_id   UNINDEXED,
    tokenize = 'unicode61'
);

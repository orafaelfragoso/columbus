-- 0001_init: base schema. Metadata + graph + git anchors only. Never code
-- bodies, never authoritative line numbers (resolved live at query time).

-- Singleton index metadata + memory id counter.
CREATE TABLE index_meta (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    schema_version  INTEGER NOT NULL,
    project_id      TEXT    NOT NULL DEFAULT '',
    indexed_head    TEXT    NOT NULL DEFAULT '',
    dirty           INTEGER NOT NULL DEFAULT 0,
    mem_seq         INTEGER NOT NULL DEFAULT 0,
    files_count     INTEGER NOT NULL DEFAULT 0,
    symbols_count   INTEGER NOT NULL DEFAULT 0,
    last_indexed_at TEXT    NOT NULL DEFAULT ''
);

-- Files: path stored once, referenced by id everywhere. Content hash is the
-- change key (git blob oid for tracked, sha256 for untracked).
CREATE TABLE files (
    id             INTEGER PRIMARY KEY,
    path           TEXT    NOT NULL UNIQUE,
    language       TEXT    NOT NULL DEFAULT '',
    package        TEXT    NOT NULL DEFAULT '',
    role           TEXT    NOT NULL DEFAULT 'other',
    blob_oid       TEXT    NOT NULL DEFAULT '',
    content_sha256 TEXT    NOT NULL DEFAULT '',
    grain_eligible INTEGER NOT NULL DEFAULT 1
);

-- Symbols: structural identity = (file_id, name, kind, container). Line ranges
-- are NOT stored here; they are resolved live.
CREATE TABLE symbols (
    id        INTEGER PRIMARY KEY,
    file_id   INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name      TEXT    NOT NULL,
    kind      TEXT    NOT NULL,
    container TEXT    NOT NULL DEFAULT '',
    signature TEXT    NOT NULL DEFAULT '',
    exported  INTEGER NOT NULL DEFAULT 0,
    UNIQUE (file_id, name, kind, container)
);
CREATE INDEX idx_symbols_file ON symbols(file_id);
CREATE INDEX idx_symbols_name ON symbols(name);

-- Raw import specifiers tied to a file; best-effort resolution to a file id.
CREATE TABLE imports (
    id               INTEGER PRIMARY KEY,
    file_id          INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    specifier        TEXT    NOT NULL,
    resolved_file_id INTEGER REFERENCES files(id) ON DELETE SET NULL
);
CREATE INDEX idx_imports_file ON imports(file_id);

CREATE TABLE exports (
    id      INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name    TEXT    NOT NULL
);
CREATE INDEX idx_exports_file ON exports(file_id);

-- Resolved file -> file edges (best-effort).
CREATE TABLE dep_edges (
    id           INTEGER PRIMARY KEY,
    from_file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    to_file_id   INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    UNIQUE (from_file_id, to_file_id)
);
CREATE INDEX idx_dep_edges_to ON dep_edges(to_file_id);

CREATE TABLE test_links (
    id           INTEGER PRIMARY KEY,
    impl_file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    test_file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    UNIQUE (impl_file_id, test_file_id)
);

-- TODO references; re-validated live.
CREATE TABLE todos (
    id      INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    line    INTEGER NOT NULL DEFAULT 0,
    text    TEXT    NOT NULL
);
CREATE INDEX idx_todos_file ON todos(file_id);

-- Memories: id is the numeric part of mem_NNN; never reused (allocated from
-- index_meta.mem_seq under the writer lock).
CREATE TABLE memories (
    id         INTEGER PRIMARY KEY,
    kind       TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE memory_tags (
    memory_id INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    tag       TEXT    NOT NULL,
    UNIQUE (memory_id, tag)
);

CREATE TABLE memory_evidence (
    id                   INTEGER PRIMARY KEY,
    memory_id            INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    path                 TEXT    NOT NULL,
    line_start           INTEGER NOT NULL,
    line_end             INTEGER NOT NULL,
    blob_oid_at_creation TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_memory_evidence_mem ON memory_evidence(memory_id);

CREATE TABLE memory_links (
    id          INTEGER PRIMARY KEY,
    memory_id   INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    target_type TEXT    NOT NULL,
    target_ref  TEXT    NOT NULL,
    UNIQUE (memory_id, target_type, target_ref)
);
CREATE INDEX idx_memory_links_mem ON memory_links(memory_id);

-- FTS5 over metadata only (never code bodies). grain = symbol|file; ref_id maps
-- back to the source row.
CREATE VIRTUAL TABLE code_fts USING fts5 (
    name,
    signature,
    path,
    package,
    grain  UNINDEXED,
    ref_id UNINDEXED,
    tokenize = 'unicode61'
);

CREATE VIRTUAL TABLE memory_fts USING fts5 (
    title,
    body,
    tags,
    memory_id UNINDEXED,
    tokenize = 'unicode61'
);

-- Seed the singleton meta row.
INSERT INTO index_meta (id, schema_version) VALUES (1, 1);

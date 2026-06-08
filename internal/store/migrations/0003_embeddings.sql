-- 0003_embeddings: local semantic layer. Vectors are DERIVED from code/text at
-- index time; the DB still stores NO code bodies. content_sha gates re-embed.

-- One virtual table holds every vector; chunk_meta carries the polymorphic key.
CREATE VIRTUAL TABLE vec_chunks USING vec0(
    embedding float[384]
);

CREATE TABLE chunk_meta (
    rowid       INTEGER PRIMARY KEY,   -- == vec_chunks rowid
    owner_type  TEXT    NOT NULL,      -- symbol|file|memory|epic|story|task
    owner_id    INTEGER NOT NULL,      -- symbols.id / files.id / memories.id / ...
    model       TEXT    NOT NULL,      -- pinned model id (re-embed on change)
    content_sha TEXT    NOT NULL,      -- sha256 of embedded text; skip if same
    UNIQUE (owner_type, owner_id, model)
);
CREATE INDEX idx_chunk_meta_owner ON chunk_meta(owner_type, owner_id);

ALTER TABLE index_meta ADD COLUMN embed_model TEXT NOT NULL DEFAULT '';
ALTER TABLE index_meta ADD COLUMN embed_dim   INTEGER NOT NULL DEFAULT 0;

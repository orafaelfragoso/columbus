-- 0005_potion_vectors: switch local semantic vectors from 384-d BGE/ONNX to
-- 256-d potion-code-16M Model2Vec. Existing vector rows are model-specific and
-- dimension-specific, so they are discarded and rebuilt by the next reindex.

DROP TABLE IF EXISTS vec_chunks;
DROP TABLE IF EXISTS chunk_meta;

CREATE VIRTUAL TABLE vec_chunks USING vec0(
    embedding float[256] distance_metric=cosine
);

CREATE TABLE chunk_meta (
    rowid       INTEGER PRIMARY KEY,   -- == vec_chunks rowid
    owner_type  TEXT    NOT NULL,      -- symbol|file|memory|epic|story|task
    owner_id    INTEGER NOT NULL,
    model       TEXT    NOT NULL,
    content_sha TEXT    NOT NULL,
    UNIQUE (owner_type, owner_id, model)
);
CREATE INDEX idx_chunk_meta_owner ON chunk_meta(owner_type, owner_id);

UPDATE index_meta SET embed_model = '', embed_dim = 0 WHERE id = 1;

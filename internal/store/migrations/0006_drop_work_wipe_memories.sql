-- 0006_drop_work_wipe_memories: 0.3.0 drops the project-management layer
-- (epics/stories/tasks) entirely and resets memories for the new 3-kind model
-- (adr|plan|documentation). Old memory kinds do not map cleanly, so memories
-- are wiped (clean slate) rather than remapped.

DROP TABLE IF EXISTS work_fts;
DROP TABLE IF EXISTS work_refs;
DROP TABLE IF EXISTS work_events;
DROP TABLE IF EXISTS work_tags;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS stories;
DROP TABLE IF EXISTS epics;

DELETE FROM memory_evidence;
DELETE FROM memory_links;
DELETE FROM memory_tags;
DELETE FROM memory_fts;
DELETE FROM memories;
UPDATE index_meta SET mem_seq = 0 WHERE id = 1;

-- Drop vectors owned by removed entity types and wiped memories.
DELETE FROM vec_chunks WHERE rowid IN (
    SELECT rowid FROM chunk_meta WHERE owner_type IN ('memory', 'epic', 'story', 'task')
);
DELETE FROM chunk_meta WHERE owner_type IN ('memory', 'epic', 'story', 'task');

ALTER TABLE index_meta DROP COLUMN epic_seq;
ALTER TABLE index_meta DROP COLUMN story_seq;
ALTER TABLE index_meta DROP COLUMN task_seq;

-- 0004_stories: insert a story tier between epics and tasks. Stories belong to
-- an epic; tasks now belong to a story (was: directly to an epic). Existing
-- tasks are repointed to a synthetic "General" story per epic so no task is
-- orphaned. tasks.epic_id is kept as a denormalized convenience for now.

-- Per-project monotonic id counter for story_NNN (mirrors epic_seq/task_seq).
ALTER TABLE index_meta ADD COLUMN story_seq INTEGER NOT NULL DEFAULT 0;

CREATE TABLE stories (
    id         INTEGER PRIMARY KEY,
    epic_id    INTEGER NOT NULL REFERENCES epics(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'todo',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_stories_epic ON stories(epic_id);

-- Tasks gain story_id. Deleting a story cascades its tasks (mirrors epic).
ALTER TABLE tasks ADD COLUMN story_id INTEGER REFERENCES stories(id) ON DELETE CASCADE;

-- Backfill: one "General" story per epic that currently has tasks. stories is a
-- fresh table, so omitting id autoassigns dense rowids in epic-id order.
INSERT INTO stories (epic_id, title, body, status, created_at, updated_at)
SELECT e.id, 'General', '', 'todo', e.created_at, e.updated_at
FROM epics e
WHERE EXISTS (SELECT 1 FROM tasks t WHERE t.epic_id = e.id)
ORDER BY e.id;

-- Repoint each existing task to its epic's General story.
UPDATE tasks
SET story_id = (SELECT s.id FROM stories s WHERE s.epic_id = tasks.epic_id AND s.title = 'General')
WHERE story_id IS NULL;

-- Index the backfilled stories for keyword search.
INSERT INTO work_fts (title, body, tags, comments, owner_type, owner_id)
SELECT s.title, s.body, '', '', 'story', s.id FROM stories s;

-- Advance the story counter past the backfilled rows so new ids never collide.
UPDATE index_meta SET story_seq = (SELECT COALESCE(MAX(id), 0) FROM stories) WHERE id = 1;

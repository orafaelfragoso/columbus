package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrate0004BackfillsGeneralStories verifies the story tier migration
// repoints existing tasks to a synthesized "General" story per epic, leaving no
// orphans.
func TestMigrate0004BackfillsGeneralStories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "columbus.sqlite")
	dsn := "file:" + path + "?_foreign_keys=on"
	raw, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	migs, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	// Build a pre-story (v3) database with one epic and two tasks.
	for _, m := range migs {
		if m.version <= 3 {
			if err := applyMigration(raw, m); err != nil {
				t.Fatalf("apply %s: %v", m.name, err)
			}
		}
	}
	stmts := []string{
		`INSERT INTO epics (id, title, body, status, created_at, updated_at) VALUES (1, 'E1', '', 'todo', 't', 't')`,
		`INSERT INTO epics (id, title, body, status, created_at, updated_at) VALUES (2, 'E2 no tasks', '', 'todo', 't', 't')`,
		`INSERT INTO tasks (id, epic_id, title, body, status, created_at, updated_at) VALUES (1, 1, 'T1', '', 'todo', 't', 't')`,
		`INSERT INTO tasks (id, epic_id, title, body, status, created_at, updated_at) VALUES (2, 1, 'T2', '', 'todo', 't', 't')`,
	}
	for _, s := range stmts {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	raw.Close()

	db, err := Open(path) // applies 0004
	if err != nil {
		t.Fatalf("Open over v3 DB: %v", err)
	}
	defer db.Close()

	// Every task must now point at a story; none orphaned.
	var orphans int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM tasks WHERE story_id IS NULL`).Scan(&orphans); err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("orphan tasks after migration: %d", orphans)
	}

	// Exactly one General story exists (for epic 1, which had tasks); epic 2 had
	// no tasks so no story was synthesized.
	stories, err := db.ListStories(0, "", "")
	if err != nil {
		t.Fatalf("ListStories: %v", err)
	}
	if len(stories) != 1 || stories[0].EpicID != 1 || stories[0].Title != "General" {
		t.Fatalf("stories = %+v, want one General story under epic 1", stories)
	}

	// Both tasks point at that story.
	for _, tid := range []int64{1, 2} {
		full, ok, err := db.TaskFull(tid)
		if err != nil || !ok {
			t.Fatalf("TaskFull(%d) ok=%v err=%v", tid, ok, err)
		}
		if full.StoryID != stories[0].ID {
			t.Errorf("task %d story_id = %d, want %d", tid, full.StoryID, stories[0].ID)
		}
	}

	// The story counter advanced past the backfilled rows.
	var storySeq int64
	if err := db.SQL().QueryRow(`SELECT story_seq FROM index_meta WHERE id = 1`).Scan(&storySeq); err != nil {
		t.Fatalf("read story_seq: %v", err)
	}
	if storySeq != stories[0].ID {
		t.Errorf("story_seq = %d, want %d", storySeq, stories[0].ID)
	}
}

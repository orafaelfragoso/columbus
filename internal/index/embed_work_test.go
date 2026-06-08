package index

import (
	"testing"

	"github.com/orafaelfragoso/columbus/internal/store"
)

// seedWorkItems creates an epic > story > task chain and a memory directly in
// the store, returning their ids.
func seedWorkItems(t *testing.T, db *store.DB) (epicID, storyID, taskID, memID int64) {
	t.Helper()
	err := db.WithTx(func(tx *store.Tx) error {
		var e error
		if epicID, e = tx.NextEpicSeq(); e != nil {
			return e
		}
		if e = tx.InsertEpic(epicID, "Search pipeline", "embed and rank", "todo", "t", "t"); e != nil {
			return e
		}
		if storyID, e = tx.NextStorySeq(); e != nil {
			return e
		}
		if e = tx.InsertStory(storyID, epicID, "Tokenizer", "wordpiece parity", "todo", "t", "t"); e != nil {
			return e
		}
		if taskID, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		if e = tx.InsertTask(taskID, epicID, storyID, "Pad batches", "", "todo", "t", "t"); e != nil {
			return e
		}
		if memID, e = tx.NextMemSeq(); e != nil {
			return e
		}
		return tx.InsertMemory(memID, "decision", "Use CLS pooling", "token 0 then normalize", "t", "t")
	})
	if err != nil {
		t.Fatalf("seedWorkItems: %v", err)
	}
	return
}

func TestEmbedWorkItemsPopulatesVectors(t *testing.T) {
	ix, _ := newIndexer(t)
	ix.Embedder = &fakeEmbedder{}
	seedWorkItems(t, ix.DB)

	if _, err := ix.Run(ModeFull); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, ot := range []string{"epic", "story", "task", "memory"} {
		if got := vecCount(t, ix, ot); got != 1 {
			t.Errorf("%s vectors = %d, want 1", ot, got)
		}
	}
}

func TestEmbedWorkItemsSkipUnchanged(t *testing.T) {
	ix, _ := newIndexer(t)
	fe := &fakeEmbedder{}
	ix.Embedder = fe
	seedWorkItems(t, ix.DB)
	if _, err := ix.Run(ModeIncremental); err != nil {
		t.Fatal(err)
	}

	fe.calls = 0
	fe.seen = nil
	res, err := ix.Run(ModeIncremental)
	if err != nil {
		t.Fatal(err)
	}
	// No file or work text changed -> nothing re-embedded.
	if fe.calls != 0 || res.Embedded != 0 {
		t.Errorf("second run embedded: calls=%d embedded=%d, want 0", fe.calls, res.Embedded)
	}
}

func TestDeleteEpicDropsWorkVectors(t *testing.T) {
	ix, _ := newIndexer(t)
	ix.Embedder = &fakeEmbedder{}
	epicID, _, _, _ := seedWorkItems(t, ix.DB)
	if _, err := ix.Run(ModeFull); err != nil {
		t.Fatal(err)
	}
	// Sanity: vectors exist before delete.
	if vecCount(t, ix, "story")+vecCount(t, ix, "task") != 2 {
		t.Fatal("expected story+task vectors before delete")
	}
	if err := ix.DB.WithTx(func(tx *store.Tx) error { return tx.DeleteEpic(epicID) }); err != nil {
		t.Fatalf("DeleteEpic: %v", err)
	}
	for _, ot := range []string{"epic", "story", "task"} {
		if got := vecCount(t, ix, ot); got != 0 {
			t.Errorf("%s vectors after cascade = %d, want 0", ot, got)
		}
	}
}

package memory

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// The unified knowledge document (schema v2) carries memories alongside the
// structured-memory entities (epics and tasks with their tags, references and
// event history) in one portable file.

// ExportEvent is one entry in an epic's or task's append-only history.
type ExportEvent struct {
	Status    string `json:"status,omitempty"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at"`
}

// ExportRef is a portable epic/task reference.
type ExportRef struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
}

// ExportEpic is one epic in the export document.
type ExportEpic struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Body      string        `json:"body,omitempty"`
	Status    string        `json:"status"`
	Tags      []string      `json:"tags,omitempty"`
	Refs      []ExportRef   `json:"refs,omitempty"`
	Events    []ExportEvent `json:"events,omitempty"`
	CreatedAt string        `json:"created_at,omitempty"`
	UpdatedAt string        `json:"updated_at,omitempty"`
}

// ExportTask is one task in the export document. Epic is the parent epic_NNN id.
type ExportTask struct {
	ID        string        `json:"id"`
	Epic      string        `json:"epic"`
	Title     string        `json:"title"`
	Body      string        `json:"body,omitempty"`
	Status    string        `json:"status"`
	Tags      []string      `json:"tags,omitempty"`
	Refs      []ExportRef   `json:"refs,omitempty"`
	Events    []ExportEvent `json:"events,omitempty"`
	CreatedAt string        `json:"created_at,omitempty"`
	UpdatedAt string        `json:"updated_at,omitempty"`
}

func formatEpicID(id int64) string { return fmt.Sprintf("epic_%03d", id) }
func formatTaskID(id int64) string { return fmt.Sprintf("task_%03d", id) }

func parseWorkID(id, prefix string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimPrefix(id, prefix), 10, 64)
	if err != nil || v <= 0 {
		return 0, contract.Errorf(contract.CodeConfigInvalid, "invalid id %q (want %sNNN)", id, prefix)
	}
	return v, nil
}

// exportEpics gathers every epic with its tags, refs and event history.
func (m *Manager) exportEpics() ([]ExportEpic, error) {
	ids, err := m.DB.AllEpicIDs()
	if err != nil {
		return nil, err
	}
	var out []ExportEpic
	for _, id := range ids {
		full, ok, err := m.DB.EpicFull(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		events, err := m.DB.WorkEvents("epic", id)
		if err != nil {
			return nil, err
		}
		out = append(out, ExportEpic{
			ID: formatEpicID(id), Title: full.Title, Body: full.Body, Status: full.Status,
			Tags: full.Tags, Refs: refsToExport(full.Refs), Events: eventsToExport(events),
			CreatedAt: full.CreatedAt, UpdatedAt: full.UpdatedAt,
		})
	}
	return out, nil
}

// exportTasks gathers every task with its tags, refs and event history.
func (m *Manager) exportTasks() ([]ExportTask, error) {
	ids, err := m.DB.AllTaskIDs()
	if err != nil {
		return nil, err
	}
	var out []ExportTask
	for _, id := range ids {
		full, ok, err := m.DB.TaskFull(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		events, err := m.DB.WorkEvents("task", id)
		if err != nil {
			return nil, err
		}
		out = append(out, ExportTask{
			ID: formatTaskID(id), Epic: formatEpicID(full.EpicID), Title: full.Title, Body: full.Body,
			Status: full.Status, Tags: full.Tags, Refs: refsToExport(full.Refs), Events: eventsToExport(events),
			CreatedAt: full.CreatedAt, UpdatedAt: full.UpdatedAt,
		})
	}
	return out, nil
}

func refsToExport(refs []store.WorkRef) []ExportRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]ExportRef, len(refs))
	for i, r := range refs {
		out[i] = ExportRef{TargetType: r.TargetType, TargetRef: r.TargetRef}
	}
	return out
}

func eventsToExport(events []store.WorkEvent) []ExportEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]ExportEvent, len(events))
	for i, e := range events {
		out[i] = ExportEvent{Status: e.NewStatus, Comment: e.Comment, CreatedAt: e.CreatedAt}
	}
	return out
}

// writeEpic restores an epic row, its tags, refs (memory refs remapped through
// memMap) and event history, and rebuilds its FTS row.
func writeEpic(tx *store.Tx, id int64, rec ExportEpic, memMap map[int64]int64, reassign bool) error {
	if err := tx.InsertEpic(id, rec.Title, rec.Body, statusOrDefault(rec.Status), rec.CreatedAt, rec.UpdatedAt); err != nil {
		return err
	}
	return writeWorkAssociations(tx, "epic", id, rec.Title, rec.Body, rec.Tags, rec.Refs, rec.Events, memMap, reassign)
}

// writeTask restores a task row (re-parented to epicID), its associations and
// FTS row.
func writeTask(tx *store.Tx, id, epicID int64, rec ExportTask, memMap map[int64]int64, reassign bool) error {
	if err := tx.InsertTask(id, epicID, rec.Title, rec.Body, statusOrDefault(rec.Status), rec.CreatedAt, rec.UpdatedAt); err != nil {
		return err
	}
	return writeWorkAssociations(tx, "task", id, rec.Title, rec.Body, rec.Tags, rec.Refs, rec.Events, memMap, reassign)
}

// writeWorkAssociations restores an owner's tags, refs (memory refs remapped
// through memMap) and events, then rebuilds its FTS row from title/body/tags
// and the event comments.
func writeWorkAssociations(tx *store.Tx, ownerType string, id int64, title, body string, tags []string, refs []ExportRef, events []ExportEvent, memMap map[int64]int64, reassign bool) error {
	for _, tag := range tags {
		if err := tx.AddWorkTag(ownerType, id, tag); err != nil {
			return err
		}
	}
	for _, ref := range refs {
		target, keep := remapRef(ref, memMap, reassign)
		if !keep {
			continue
		}
		if err := tx.AddWorkRef(ownerType, id, ref.TargetType, target); err != nil {
			return err
		}
	}
	var comments []string
	for _, ev := range events {
		if err := tx.AppendWorkEvent(ownerType, id, ev.Status, ev.Comment, ev.CreatedAt); err != nil {
			return err
		}
		if ev.Comment != "" {
			comments = append(comments, ev.Comment)
		}
	}
	return tx.ReindexWorkFTS(ownerType, id, title, body, strings.Join(tags, " "), strings.Join(comments, " "))
}

// remapRef resolves a reference's stored target for import. A memory ref is
// rewritten to the imported memory's new id when its old id was remapped; in
// reassign mode a memory ref whose old id is absent from the document is
// DROPPED (keep=false) rather than passed through, since the bare numeric id
// would point at an unrelated local memory. Under preserve-ids the original id
// is correct (ids are kept), so it passes through. Non-memory refs always pass
// through unchanged.
func remapRef(ref ExportRef, memMap map[int64]int64, reassign bool) (target string, keep bool) {
	if ref.TargetType != "memory" {
		return ref.TargetRef, true
	}
	old, err := ParseID(ref.TargetRef)
	if err != nil {
		return ref.TargetRef, true
	}
	if newID, ok := memMap[old]; ok {
		return FormatID(newID), true
	}
	if reassign {
		return "", false
	}
	return ref.TargetRef, true
}

func statusOrDefault(s string) string {
	if s == "" {
		return "todo"
	}
	return s
}

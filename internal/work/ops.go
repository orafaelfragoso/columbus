package work

import (
	"strings"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// EpicAddParams are the inputs to EpicAdd.
type EpicAddParams struct {
	Title string
	Body  string
	Tags  []string
}

// EpicAdd creates an epic (status todo) and auto-logs the initial event.
func (m *Manager) EpicAdd(p EpicAddParams) (EpicResult, error) {
	if strings.TrimSpace(p.Title) == "" {
		return EpicResult{}, contract.Errorf(contract.CodeUsage, "epic --title is required")
	}
	now := m.now()
	var id int64
	err := m.DB.WithTx(func(tx *store.Tx) error {
		var e error
		if id, e = tx.NextEpicSeq(); e != nil {
			return e
		}
		if e = tx.InsertEpic(id, p.Title, p.Body, StatusDefault, now, now); e != nil {
			return e
		}
		for _, tag := range dedupe(p.Tags) {
			if e = tx.AddWorkTag("epic", id, tag); e != nil {
				return e
			}
		}
		return tx.AppendWorkEvent("epic", id, StatusDefault, "", now)
	})
	if err != nil {
		return EpicResult{}, err
	}
	return m.loadEpic(id)
}

// TaskAddParams are the inputs to TaskAdd.
type TaskAddParams struct {
	Epic  string
	Title string
	Body  string
	Tags  []string
}

// TaskAdd creates a task under an existing epic (status todo) and auto-logs the
// initial event. A missing epic is NOT_FOUND.
func (m *Manager) TaskAdd(p TaskAddParams) (TaskResult, error) {
	if strings.TrimSpace(p.Title) == "" {
		return TaskResult{}, contract.Errorf(contract.CodeUsage, "task --title is required")
	}
	epicID, err := ParseEpicID(p.Epic)
	if err != nil {
		return TaskResult{}, err
	}
	if ok, err := m.DB.EpicExists(epicID); err != nil {
		return TaskResult{}, err
	} else if !ok {
		return TaskResult{}, notFound("epic", p.Epic)
	}
	now := m.now()
	var id int64
	err = m.DB.WithTx(func(tx *store.Tx) error {
		var e error
		if id, e = tx.NextTaskSeq(); e != nil {
			return e
		}
		if e = tx.InsertTask(id, epicID, p.Title, p.Body, StatusDefault, now, now); e != nil {
			return e
		}
		for _, tag := range dedupe(p.Tags) {
			if e = tx.AddWorkTag("task", id, tag); e != nil {
				return e
			}
		}
		return tx.AppendWorkEvent("task", id, StatusDefault, "", now)
	})
	if err != nil {
		return TaskResult{}, err
	}
	return m.loadTask(id)
}

// EpicEditParams are partial changes; nil pointers mean "unchanged".
type EpicEditParams struct {
	Title      *string
	Body       *string
	AddTags    []string
	RemoveTags []string
}

func (p EpicEditParams) empty() bool {
	return p.Title == nil && p.Body == nil && len(p.AddTags) == 0 && len(p.RemoveTags) == 0
}

// EpicEdit applies partial non-historical changes to an epic (no event).
func (m *Manager) EpicEdit(idStr string, p EpicEditParams) (EpicResult, error) {
	id, err := ParseEpicID(idStr)
	if err != nil {
		return EpicResult{}, err
	}
	if p.empty() {
		return EpicResult{}, contract.Errorf(contract.CodeUsage, "edit requires at least one change")
	}
	cur, ok, err := m.DB.EpicFull(id)
	if err != nil {
		return EpicResult{}, err
	}
	if !ok {
		return EpicResult{}, notFound("epic", idStr)
	}
	title, body := cur.Title, cur.Body
	if p.Title != nil {
		title = *p.Title
	}
	if p.Body != nil {
		body = *p.Body
	}
	err = m.DB.WithTx(func(tx *store.Tx) error {
		if e := tx.UpdateEpic(id, title, body, m.now()); e != nil {
			return e
		}
		return applyTagChanges(tx, "epic", id, p.AddTags, p.RemoveTags)
	})
	if err != nil {
		return EpicResult{}, err
	}
	return m.loadEpic(id)
}

// TaskEditParams are partial changes; a non-nil Epic re-parents the task.
type TaskEditParams struct {
	Title      *string
	Body       *string
	Epic       *string
	AddTags    []string
	RemoveTags []string
}

func (p TaskEditParams) empty() bool {
	return p.Title == nil && p.Body == nil && p.Epic == nil &&
		len(p.AddTags) == 0 && len(p.RemoveTags) == 0
}

// TaskEdit applies partial non-historical changes to a task, including
// re-parenting to another (existing) epic.
func (m *Manager) TaskEdit(idStr string, p TaskEditParams) (TaskResult, error) {
	id, err := ParseTaskID(idStr)
	if err != nil {
		return TaskResult{}, err
	}
	if p.empty() {
		return TaskResult{}, contract.Errorf(contract.CodeUsage, "edit requires at least one change")
	}
	cur, ok, err := m.DB.TaskFull(id)
	if err != nil {
		return TaskResult{}, err
	}
	if !ok {
		return TaskResult{}, notFound("task", idStr)
	}
	newEpicID := cur.EpicID
	if p.Epic != nil {
		newEpicID, err = ParseEpicID(*p.Epic)
		if err != nil {
			return TaskResult{}, err
		}
		if ok, err := m.DB.EpicExists(newEpicID); err != nil {
			return TaskResult{}, err
		} else if !ok {
			return TaskResult{}, notFound("epic", *p.Epic)
		}
	}
	title, body := cur.Title, cur.Body
	if p.Title != nil {
		title = *p.Title
	}
	if p.Body != nil {
		body = *p.Body
	}
	err = m.DB.WithTx(func(tx *store.Tx) error {
		if e := tx.UpdateTask(id, title, body, m.now()); e != nil {
			return e
		}
		if p.Epic != nil {
			if e := tx.ReparentTask(id, newEpicID, m.now()); e != nil {
				return e
			}
		}
		return applyTagChanges(tx, "task", id, p.AddTags, p.RemoveTags)
	})
	if err != nil {
		return TaskResult{}, err
	}
	return m.loadTask(id)
}

// EpicStatus appends a status-change event and denormalizes the new status.
func (m *Manager) EpicStatus(idStr, to, comment string) (EpicResult, error) {
	id, err := m.statusPrep("epic", idStr, to)
	if err != nil {
		return EpicResult{}, err
	}
	now := m.now()
	err = m.DB.WithTx(func(tx *store.Tx) error {
		if e := tx.AppendWorkEvent("epic", id, to, comment, now); e != nil {
			return e
		}
		return tx.SetEpicStatus(id, to, now)
	})
	if err != nil {
		return EpicResult{}, err
	}
	return m.loadEpic(id)
}

// TaskStatus appends a status-change event and denormalizes the new status.
func (m *Manager) TaskStatus(idStr, to, comment string) (TaskResult, error) {
	id, err := m.statusPrep("task", idStr, to)
	if err != nil {
		return TaskResult{}, err
	}
	now := m.now()
	err = m.DB.WithTx(func(tx *store.Tx) error {
		if e := tx.AppendWorkEvent("task", id, to, comment, now); e != nil {
			return e
		}
		return tx.SetTaskStatus(id, to, now)
	})
	if err != nil {
		return TaskResult{}, err
	}
	return m.loadTask(id)
}

// statusPrep validates the target status and the owner's existence.
func (m *Manager) statusPrep(ownerType, idStr, to string) (int64, error) {
	id, err := m.parseOwner(ownerType, idStr)
	if err != nil {
		return 0, err
	}
	if !validStatus(to) {
		return 0, invalidStatus(to)
	}
	ok, err := m.ownerExists(ownerType, id)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, notFound(ownerType, idStr)
	}
	return id, nil
}

// EpicComment appends a comment-only note (status stays NULL).
func (m *Manager) EpicComment(idStr, text string) (EpicResult, error) {
	id, err := m.commentPrep("epic", idStr, text)
	if err != nil {
		return EpicResult{}, err
	}
	if err := m.appendComment("epic", id, text); err != nil {
		return EpicResult{}, err
	}
	return m.loadEpic(id)
}

// TaskComment appends a comment-only note (status stays NULL).
func (m *Manager) TaskComment(idStr, text string) (TaskResult, error) {
	id, err := m.commentPrep("task", idStr, text)
	if err != nil {
		return TaskResult{}, err
	}
	if err := m.appendComment("task", id, text); err != nil {
		return TaskResult{}, err
	}
	return m.loadTask(id)
}

func (m *Manager) commentPrep(ownerType, idStr, text string) (int64, error) {
	id, err := m.parseOwner(ownerType, idStr)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(text) == "" {
		return 0, contract.Errorf(contract.CodeUsage, "comment requires --text")
	}
	ok, err := m.ownerExists(ownerType, id)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, notFound(ownerType, idStr)
	}
	return id, nil
}

func (m *Manager) appendComment(ownerType string, id int64, text string) error {
	now := m.now()
	return m.DB.WithTx(func(tx *store.Tx) error {
		return tx.AppendWorkEvent(ownerType, id, "", text, now)
	})
}

// EpicDelete hard-deletes an epic (cascading its tasks). force is required.
func (m *Manager) EpicDelete(idStr string, force bool) (DeleteResult, error) {
	id, err := ParseEpicID(idStr)
	if err != nil {
		return DeleteResult{}, err
	}
	if !force {
		return DeleteResult{}, contract.Errorf(contract.CodeUsage, "delete requires --force (destructive; id retired)")
	}
	ok, err := m.DB.EpicExists(id)
	if err != nil {
		return DeleteResult{}, err
	}
	if !ok {
		return DeleteResult{}, notFound("epic", idStr)
	}
	if err := m.DB.WithTx(func(tx *store.Tx) error { return tx.DeleteEpic(id) }); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{command: "epic", ID: FormatEpicID(id), Removed: true}, nil
}

// TaskDelete hard-deletes a task. force is required.
func (m *Manager) TaskDelete(idStr string, force bool) (DeleteResult, error) {
	id, err := ParseTaskID(idStr)
	if err != nil {
		return DeleteResult{}, err
	}
	if !force {
		return DeleteResult{}, contract.Errorf(contract.CodeUsage, "delete requires --force (destructive; id retired)")
	}
	ok, err := m.DB.TaskExists(id)
	if err != nil {
		return DeleteResult{}, err
	}
	if !ok {
		return DeleteResult{}, notFound("task", idStr)
	}
	if err := m.DB.WithTx(func(tx *store.Tx) error { return tx.DeleteTask(id) }); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{command: "task", ID: FormatTaskID(id), Removed: true}, nil
}

// EpicList returns epic summaries filtered by optional status and tag.
func (m *Manager) EpicList(status, tag string) (EpicListResult, error) {
	if status != "" && !validStatus(status) {
		return EpicListResult{}, invalidStatus(status)
	}
	briefs, err := m.DB.ListEpics(status, tag)
	if err != nil {
		return EpicListResult{}, err
	}
	res := EpicListResult{Status: status, Tag: tag, Counts: map[string]int{}}
	for _, b := range briefs {
		res.Epics = append(res.Epics, EpicRef{ID: FormatEpicID(b.ID), Title: b.Title, Status: b.Status})
		res.Counts[b.Status]++
	}
	res.Total = len(res.Epics)
	return res, nil
}

// TaskList returns task summaries filtered by optional epic, status and tag.
func (m *Manager) TaskList(epic, status, tag string) (TaskListResult, error) {
	if status != "" && !validStatus(status) {
		return TaskListResult{}, invalidStatus(status)
	}
	var epicID int64
	if epic != "" {
		var err error
		if epicID, err = ParseEpicID(epic); err != nil {
			return TaskListResult{}, err
		}
	}
	briefs, err := m.DB.ListTasks(epicID, status, tag)
	if err != nil {
		return TaskListResult{}, err
	}
	res := TaskListResult{Epic: epic, Status: status, Tag: tag, Counts: map[string]int{}}
	for _, b := range briefs {
		res.Tasks = append(res.Tasks, TaskRef{ID: FormatTaskID(b.ID), Epic: FormatEpicID(b.EpicID), Title: b.Title, Status: b.Status})
		res.Counts[b.Status]++
	}
	res.Total = len(res.Tasks)
	return res, nil
}

// --- shared helpers over the polymorphic owner type ---

func (m *Manager) parseOwner(ownerType, idStr string) (int64, error) {
	if ownerType == "epic" {
		return ParseEpicID(idStr)
	}
	return ParseTaskID(idStr)
}

func (m *Manager) ownerExists(ownerType string, id int64) (bool, error) {
	if ownerType == "epic" {
		return m.DB.EpicExists(id)
	}
	return m.DB.TaskExists(id)
}

func applyTagChanges(tx *store.Tx, ownerType string, id int64, add, remove []string) error {
	for _, t := range remove {
		if e := tx.RemoveWorkTag(ownerType, id, t); e != nil {
			return e
		}
	}
	for _, t := range dedupe(add) {
		if e := tx.AddWorkTag(ownerType, id, t); e != nil {
			return e
		}
	}
	return nil
}

// reindexFTS rebuilds an owner's FTS row from its current title/body/tags and
// the comments in its event log. Reads happen outside the write tx (the writer
// holds the single connection).
func (m *Manager) reindexFTS(ownerType string, id int64) error {
	var title, body string
	var tags []string
	switch ownerType {
	case "epic":
		full, ok, err := m.DB.EpicFull(id)
		if err != nil || !ok {
			return err
		}
		title, body, tags = full.Title, full.Body, full.Tags
	default:
		full, ok, err := m.DB.TaskFull(id)
		if err != nil || !ok {
			return err
		}
		title, body, tags = full.Title, full.Body, full.Tags
	}
	events, err := m.DB.WorkEvents(ownerType, id)
	if err != nil {
		return err
	}
	var comments []string
	for _, e := range events {
		if e.Comment != "" {
			comments = append(comments, e.Comment)
		}
	}
	return m.DB.WithTx(func(tx *store.Tx) error {
		return tx.ReindexWorkFTS(ownerType, id, title, body, strings.Join(tags, " "), strings.Join(comments, " "))
	})
}

func (m *Manager) loadEpic(id int64) (EpicResult, error) {
	if err := m.reindexFTS("epic", id); err != nil {
		return EpicResult{}, err
	}
	full, ok, err := m.DB.EpicFull(id)
	if err != nil {
		return EpicResult{}, err
	}
	if !ok {
		return EpicResult{}, notFound("epic", FormatEpicID(id))
	}
	return epicResultFrom(full), nil
}

func (m *Manager) loadTask(id int64) (TaskResult, error) {
	if err := m.reindexFTS("task", id); err != nil {
		return TaskResult{}, err
	}
	full, ok, err := m.DB.TaskFull(id)
	if err != nil {
		return TaskResult{}, err
	}
	if !ok {
		return TaskResult{}, notFound("task", FormatTaskID(id))
	}
	return taskResultFrom(full), nil
}

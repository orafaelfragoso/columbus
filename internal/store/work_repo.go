package store

import "strings"

// WorkRef is a drift-checked reference from an epic/task to a file, dir, memory
// or symbol.
type WorkRef struct {
	TargetType string
	TargetRef  string
}

// Epic is a full epic record with its associations.
type Epic struct {
	ID        int64
	Title     string
	Body      string
	Status    string
	CreatedAt string
	UpdatedAt string
	Tags      []string
	Refs      []WorkRef
}

// EpicBrief is an epic summary for list/search output.
type EpicBrief struct {
	ID     int64
	Title  string
	Status string
}

// EpicFull fetches an epic and its tags by numeric id.
func (d *DB) EpicFull(id int64) (Epic, bool, error) {
	var e Epic
	err := d.db.QueryRow(`SELECT id, title, body, status, created_at, updated_at FROM epics WHERE id = ?`, id).
		Scan(&e.ID, &e.Title, &e.Body, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return Epic{}, false, nil
		}
		return Epic{}, false, storeErr(err)
	}
	tags, err := d.pathQuery(`SELECT tag FROM work_tags WHERE owner_type = 'epic' AND owner_id = ? ORDER BY tag`, id)
	if err != nil {
		return Epic{}, false, err
	}
	e.Tags = tags
	refs, err := d.workRefs("epic", id)
	if err != nil {
		return Epic{}, false, err
	}
	e.Refs = refs
	return e, true, nil
}

// workRefs returns an owner's references ordered deterministically.
func (d *DB) workRefs(ownerType string, ownerID int64) ([]WorkRef, error) {
	rows, err := d.db.Query(`SELECT target_type, target_ref FROM work_refs
		WHERE owner_type = ? AND owner_id = ? ORDER BY target_type, target_ref`, ownerType, ownerID)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []WorkRef
	for rows.Next() {
		var r WorkRef
		if err := rows.Scan(&r.TargetType, &r.TargetRef); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// WorkOwner is an epic or task that references some target (reverse lookup).
type WorkOwner struct {
	OwnerType string // "epic" | "task"
	OwnerID   int64
	Title     string
	Status    string
}

// WorkForTarget returns the epics and tasks that reference a given target
// (exact target_type + target_ref match), ordered deterministically.
func (d *DB) WorkForTarget(targetType, targetRef string) ([]WorkOwner, error) {
	rows, err := d.db.Query(`SELECT r.owner_type, r.owner_id,
			COALESCE(e.title, t.title, ''), COALESCE(e.status, t.status, '')
		FROM work_refs r
		LEFT JOIN epics e ON r.owner_type = 'epic' AND e.id = r.owner_id
		LEFT JOIN tasks t ON r.owner_type = 'task' AND t.id = r.owner_id
		WHERE r.target_type = ? AND r.target_ref = ?
		ORDER BY r.owner_type, r.owner_id`, targetType, targetRef)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []WorkOwner
	for rows.Next() {
		var o WorkOwner
		if err := rows.Scan(&o.OwnerType, &o.OwnerID, &o.Title, &o.Status); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// SearchWorkFTS returns the epics/tasks matching the FTS query, best first.
func (d *DB) SearchWorkFTS(match string, limit int) ([]WorkOwner, error) {
	rows, err := d.db.Query(`SELECT f.owner_type, f.owner_id,
			COALESCE(e.title, t.title, ''), COALESCE(e.status, t.status, '')
		FROM work_fts f
		LEFT JOIN epics e ON f.owner_type = 'epic' AND e.id = f.owner_id
		LEFT JOIN tasks t ON f.owner_type = 'task' AND t.id = f.owner_id
		WHERE work_fts MATCH ? ORDER BY bm25(work_fts) LIMIT ?`, match, limit)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []WorkOwner
	for rows.Next() {
		var o WorkOwner
		if err := rows.Scan(&o.OwnerType, &o.OwnerID, &o.Title, &o.Status); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// HasFilesUnderDir reports whether at least one indexed file lives under the
// given directory prefix (whole-segment match; a trailing slash is tolerated).
func (d *DB) HasFilesUnderDir(dir string) (bool, error) {
	prefix := strings.TrimSuffix(dir, "/")
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM files WHERE path = ? OR path LIKE ? || '/%'`,
		prefix, prefix).Scan(&n); err != nil {
		return false, storeErr(err)
	}
	return n > 0, nil
}

// ListEpics returns epic summaries filtered by optional status and tag,
// ordered by id ascending (stable/deterministic).
func (d *DB) ListEpics(status, tag string) ([]EpicBrief, error) {
	query := `SELECT DISTINCT e.id, e.title, e.status FROM epics e`
	var args []any
	var where []string
	if tag != "" {
		query += ` JOIN work_tags t ON t.owner_type = 'epic' AND t.owner_id = e.id`
		where = append(where, `t.tag = ?`)
		args = append(args, tag)
	}
	if status != "" {
		where = append(where, `e.status = ?`)
		args = append(args, status)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY e.id`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []EpicBrief
	for rows.Next() {
		var b EpicBrief
		if err := rows.Scan(&b.ID, &b.Title, &b.Status); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AllEpicIDs returns every epic id ascending (for validate/export).
func (d *DB) AllEpicIDs() ([]int64, error) {
	return d.idList(`SELECT id FROM epics ORDER BY id`)
}

// Task is a full task record with its associations.
type Task struct {
	ID        int64
	EpicID    int64
	Title     string
	Body      string
	Status    string
	CreatedAt string
	UpdatedAt string
	Tags      []string
	Refs      []WorkRef
}

// TaskBrief is a task summary for list/search output.
type TaskBrief struct {
	ID     int64
	EpicID int64
	Title  string
	Status string
}

// TaskFull fetches a task and its tags by numeric id.
func (d *DB) TaskFull(id int64) (Task, bool, error) {
	var ta Task
	err := d.db.QueryRow(`SELECT id, epic_id, title, body, status, created_at, updated_at FROM tasks WHERE id = ?`, id).
		Scan(&ta.ID, &ta.EpicID, &ta.Title, &ta.Body, &ta.Status, &ta.CreatedAt, &ta.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return Task{}, false, nil
		}
		return Task{}, false, storeErr(err)
	}
	tags, err := d.pathQuery(`SELECT tag FROM work_tags WHERE owner_type = 'task' AND owner_id = ? ORDER BY tag`, id)
	if err != nil {
		return Task{}, false, err
	}
	ta.Tags = tags
	refs, err := d.workRefs("task", id)
	if err != nil {
		return Task{}, false, err
	}
	ta.Refs = refs
	return ta, true, nil
}

// ListTasks returns task summaries filtered by optional epic id (0 = any),
// status and tag, ordered by id ascending.
func (d *DB) ListTasks(epicID int64, status, tag string) ([]TaskBrief, error) {
	query := `SELECT DISTINCT t.id, t.epic_id, t.title, t.status FROM tasks t`
	var args []any
	var where []string
	if tag != "" {
		query += ` JOIN work_tags wt ON wt.owner_type = 'task' AND wt.owner_id = t.id`
		where = append(where, `wt.tag = ?`)
		args = append(args, tag)
	}
	if epicID != 0 {
		where = append(where, `t.epic_id = ?`)
		args = append(args, epicID)
	}
	if status != "" {
		where = append(where, `t.status = ?`)
		args = append(args, status)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY t.id`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []TaskBrief
	for rows.Next() {
		var b TaskBrief
		if err := rows.Scan(&b.ID, &b.EpicID, &b.Title, &b.Status); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AllTaskIDs returns every task id ascending.
func (d *DB) AllTaskIDs() ([]int64, error) {
	return d.idList(`SELECT id FROM tasks ORDER BY id`)
}

// TaskExists reports whether a task id exists.
func (d *DB) TaskExists(id int64) (bool, error) {
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = ?`, id).Scan(&n); err != nil {
		return false, storeErr(err)
	}
	return n > 0, nil
}

// WorkEvent is one entry in an epic's or task's append-only event log. An empty
// NewStatus means a comment-only note; an empty Comment means a status-only
// change.
type WorkEvent struct {
	NewStatus string
	Comment   string
	CreatedAt string
}

// WorkEvents returns an owner's event log in chronological (id ascending) order.
func (d *DB) WorkEvents(ownerType string, ownerID int64) ([]WorkEvent, error) {
	rows, err := d.db.Query(`SELECT COALESCE(new_status, ''), COALESCE(comment, ''), created_at
		FROM work_events WHERE owner_type = ? AND owner_id = ? ORDER BY id`, ownerType, ownerID)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []WorkEvent
	for rows.Next() {
		var e WorkEvent
		if err := rows.Scan(&e.NewStatus, &e.Comment, &e.CreatedAt); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EpicExists reports whether an epic id exists.
func (d *DB) EpicExists(id int64) (bool, error) {
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM epics WHERE id = ?`, id).Scan(&n); err != nil {
		return false, storeErr(err)
	}
	return n > 0, nil
}

// idList runs a query returning a single int64 column into a slice.
func (d *DB) idList(query string, args ...any) ([]int64, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

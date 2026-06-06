package store

import "strings"

// Epic is a full epic record with its associations.
type Epic struct {
	ID        int64
	Title     string
	Body      string
	Status    string
	CreatedAt string
	UpdatedAt string
	Tags      []string
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
	return e, true, nil
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

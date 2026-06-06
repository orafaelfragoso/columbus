package store

// NextEpicSeq increments and returns the per-project epic id counter inside the
// transaction. IDs are never reused.
func (t *Tx) NextEpicSeq() (int64, error) {
	var n int64
	if err := t.tx.QueryRow(`UPDATE index_meta SET epic_seq = epic_seq + 1 WHERE id = 1 RETURNING epic_seq`).Scan(&n); err != nil {
		return 0, storeErr(err)
	}
	return n, nil
}

// SetEpicSeqAtLeast raises the epic id counter to at least n (after a
// --preserve-ids import).
func (t *Tx) SetEpicSeqAtLeast(n int64) error {
	_, err := t.tx.Exec(`UPDATE index_meta SET epic_seq = MAX(epic_seq, ?) WHERE id = 1`, n)
	return storeErr(err)
}

// InsertEpic writes an epic row.
func (t *Tx) InsertEpic(id int64, title, body, status, createdAt, updatedAt string) error {
	_, err := t.tx.Exec(`INSERT INTO epics (id, title, body, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, title, body, status, createdAt, updatedAt)
	return storeErr(err)
}

// UpdateEpic updates the non-historical metadata of an epic (title/body). Status
// changes flow through the event log, not here.
func (t *Tx) UpdateEpic(id int64, title, body, updatedAt string) error {
	_, err := t.tx.Exec(`UPDATE epics SET title = ?, body = ?, updated_at = ? WHERE id = ?`,
		title, body, updatedAt, id)
	return storeErr(err)
}

// NextTaskSeq increments and returns the per-project task id counter.
func (t *Tx) NextTaskSeq() (int64, error) {
	var n int64
	if err := t.tx.QueryRow(`UPDATE index_meta SET task_seq = task_seq + 1 WHERE id = 1 RETURNING task_seq`).Scan(&n); err != nil {
		return 0, storeErr(err)
	}
	return n, nil
}

// SetTaskSeqAtLeast raises the task id counter to at least n.
func (t *Tx) SetTaskSeqAtLeast(n int64) error {
	_, err := t.tx.Exec(`UPDATE index_meta SET task_seq = MAX(task_seq, ?) WHERE id = 1`, n)
	return storeErr(err)
}

// InsertTask writes a task row. The NOT NULL epic_id FK rejects a missing epic.
func (t *Tx) InsertTask(id, epicID int64, title, body, status, createdAt, updatedAt string) error {
	_, err := t.tx.Exec(`INSERT INTO tasks (id, epic_id, title, body, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, epicID, title, body, status, createdAt, updatedAt)
	return storeErr(err)
}

// UpdateTask updates the non-historical metadata of a task (title/body).
func (t *Tx) UpdateTask(id int64, title, body, updatedAt string) error {
	_, err := t.tx.Exec(`UPDATE tasks SET title = ?, body = ?, updated_at = ? WHERE id = ?`,
		title, body, updatedAt, id)
	return storeErr(err)
}

// ReparentTask moves a task to a different epic.
func (t *Tx) ReparentTask(id, epicID int64, updatedAt string) error {
	_, err := t.tx.Exec(`UPDATE tasks SET epic_id = ?, updated_at = ? WHERE id = ?`, epicID, updatedAt, id)
	return storeErr(err)
}

// DeleteTask hard-deletes a task and its polymorphic associations.
func (t *Tx) DeleteTask(id int64) error {
	if err := t.deleteWorkAssociations("task", id); err != nil {
		return err
	}
	_, err := t.tx.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return storeErr(err)
}

// DeleteEpic hard-deletes an epic and everything hanging off it: child tasks
// (FK cascade) plus the polymorphic work_tags/events/refs/fts rows for the epic
// and each child task (no owner FK, so cleaned up explicitly).
func (t *Tx) DeleteEpic(id int64) error {
	taskIDs, err := t.taskIDsForEpic(id)
	if err != nil {
		return err
	}
	for _, tid := range taskIDs {
		if err := t.deleteWorkAssociations("task", tid); err != nil {
			return err
		}
	}
	if err := t.deleteWorkAssociations("epic", id); err != nil {
		return err
	}
	_, err = t.tx.Exec(`DELETE FROM epics WHERE id = ?`, id)
	return storeErr(err)
}

// taskIDsForEpic returns the ids of an epic's child tasks (tx-scoped read so it
// is safe inside WithTx and sees uncommitted writes).
func (t *Tx) taskIDsForEpic(epicID int64) ([]int64, error) {
	rows, err := t.tx.Query(`SELECT id FROM tasks WHERE epic_id = ?`, epicID)
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

// deleteWorkAssociations removes the polymorphic tags/events/refs/fts rows for
// one owner.
func (t *Tx) deleteWorkAssociations(ownerType string, ownerID int64) error {
	for _, table := range []string{"work_tags", "work_events", "work_refs"} {
		if _, err := t.tx.Exec(`DELETE FROM `+table+` WHERE owner_type = ? AND owner_id = ?`, ownerType, ownerID); err != nil {
			return storeErr(err)
		}
	}
	return t.DeleteWorkFTS(ownerType, ownerID)
}

// AppendWorkEvent writes one append-only event. An empty newStatus or comment
// is stored as NULL (distinguishing status-only changes from comment-only
// notes). The caller guarantees at least one is non-empty.
func (t *Tx) AppendWorkEvent(ownerType string, ownerID int64, newStatus, comment, createdAt string) error {
	_, err := t.tx.Exec(`INSERT INTO work_events (owner_type, owner_id, new_status, comment, created_at)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)`, ownerType, ownerID, newStatus, comment, createdAt)
	return storeErr(err)
}

// SetEpicStatus updates the denormalized current status of an epic.
func (t *Tx) SetEpicStatus(id int64, status, updatedAt string) error {
	_, err := t.tx.Exec(`UPDATE epics SET status = ?, updated_at = ? WHERE id = ?`, status, updatedAt, id)
	return storeErr(err)
}

// SetTaskStatus updates the denormalized current status of a task.
func (t *Tx) SetTaskStatus(id int64, status, updatedAt string) error {
	_, err := t.tx.Exec(`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`, status, updatedAt, id)
	return storeErr(err)
}

// AddWorkTag adds a tag to an epic or task (idempotent).
func (t *Tx) AddWorkTag(ownerType string, ownerID int64, tag string) error {
	_, err := t.tx.Exec(`INSERT OR IGNORE INTO work_tags (owner_type, owner_id, tag) VALUES (?, ?, ?)`,
		ownerType, ownerID, tag)
	return storeErr(err)
}

// EpicExists / TaskExists report existence through the transaction's own
// connection — required for preserve-ids collision checks inside WithTx, where
// a pool read would deadlock against the single writer connection.
func (t *Tx) EpicExists(id int64) (bool, error) {
	var n int
	if err := t.tx.QueryRow(`SELECT COUNT(*) FROM epics WHERE id = ?`, id).Scan(&n); err != nil {
		return false, storeErr(err)
	}
	return n > 0, nil
}

func (t *Tx) TaskExists(id int64) (bool, error) {
	var n int
	if err := t.tx.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = ?`, id).Scan(&n); err != nil {
		return false, storeErr(err)
	}
	return n > 0, nil
}

// AddWorkRef adds a drift-checked reference to an epic or task (idempotent).
func (t *Tx) AddWorkRef(ownerType string, ownerID int64, targetType, targetRef string) error {
	_, err := t.tx.Exec(`INSERT OR IGNORE INTO work_refs (owner_type, owner_id, target_type, target_ref)
		VALUES (?, ?, ?, ?)`, ownerType, ownerID, targetType, targetRef)
	return storeErr(err)
}

// RemoveWorkRef removes a reference from an epic or task.
func (t *Tx) RemoveWorkRef(ownerType string, ownerID int64, targetType, targetRef string) error {
	_, err := t.tx.Exec(`DELETE FROM work_refs WHERE owner_type = ? AND owner_id = ? AND target_type = ? AND target_ref = ?`,
		ownerType, ownerID, targetType, targetRef)
	return storeErr(err)
}

// RemoveWorkTag removes a tag from an epic or task.
func (t *Tx) RemoveWorkTag(ownerType string, ownerID int64, tag string) error {
	_, err := t.tx.Exec(`DELETE FROM work_tags WHERE owner_type = ? AND owner_id = ? AND tag = ?`,
		ownerType, ownerID, tag)
	return storeErr(err)
}

// ReindexWorkFTS rebuilds an owner's FTS row from its current title/body/tags
// and concatenated event comments.
func (t *Tx) ReindexWorkFTS(ownerType string, ownerID int64, title, body, tags, comments string) error {
	if err := t.DeleteWorkFTS(ownerType, ownerID); err != nil {
		return err
	}
	_, err := t.tx.Exec(`INSERT INTO work_fts (title, body, tags, comments, owner_type, owner_id)
		VALUES (?, ?, ?, ?, ?, ?)`, title, body, tags, comments, ownerType, ownerID)
	return storeErr(err)
}

// DeleteWorkFTS removes an owner's FTS row.
func (t *Tx) DeleteWorkFTS(ownerType string, ownerID int64) error {
	_, err := t.tx.Exec(`DELETE FROM work_fts WHERE owner_type = ? AND owner_id = ?`, ownerType, ownerID)
	return storeErr(err)
}

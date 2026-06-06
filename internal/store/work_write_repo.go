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

// DeleteEpic hard-deletes an epic and its polymorphic work_tags/events/refs/fts
// rows (no owner FK, so cleaned up explicitly under the writer lock).
func (t *Tx) DeleteEpic(id int64) error {
	if err := t.deleteWorkAssociations("epic", id); err != nil {
		return err
	}
	_, err := t.tx.Exec(`DELETE FROM epics WHERE id = ?`, id)
	return storeErr(err)
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

// AddWorkTag adds a tag to an epic or task (idempotent).
func (t *Tx) AddWorkTag(ownerType string, ownerID int64, tag string) error {
	_, err := t.tx.Exec(`INSERT OR IGNORE INTO work_tags (owner_type, owner_id, tag) VALUES (?, ?, ?)`,
		ownerType, ownerID, tag)
	return storeErr(err)
}

// DeleteWorkFTS removes an owner's FTS row.
func (t *Tx) DeleteWorkFTS(ownerType string, ownerID int64) error {
	_, err := t.tx.Exec(`DELETE FROM work_fts WHERE owner_type = ? AND owner_id = ?`, ownerType, ownerID)
	return storeErr(err)
}

package store

import "strings"

// NextMemSeq increments and returns the per-project memory id counter inside
// the transaction. IDs are never reused.
func (t *Tx) NextMemSeq() (int64, error) {
	var n int64
	if err := t.tx.QueryRow(`UPDATE index_meta SET mem_seq = mem_seq + 1 WHERE id = 1 RETURNING mem_seq`).Scan(&n); err != nil {
		return 0, storeErr(err)
	}
	return n, nil
}

// InsertMemory writes a memory row.
func (t *Tx) InsertMemory(id int64, kind, title, body, createdAt, updatedAt string) error {
	_, err := t.tx.Exec(`INSERT INTO memories (id, kind, title, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, kind, title, body, createdAt, updatedAt)
	return storeErr(err)
}

// UpdateMemory updates the mutable fields of a memory.
func (t *Tx) UpdateMemory(id int64, kind, title, body, updatedAt string) error {
	_, err := t.tx.Exec(`UPDATE memories SET kind = ?, title = ?, body = ?, updated_at = ? WHERE id = ?`,
		kind, title, body, updatedAt, id)
	return storeErr(err)
}

// DeleteMemory hard-deletes a memory; cascades drop tags/evidence/links. The
// FTS row must be removed separately (virtual table, no FK).
func (t *Tx) DeleteMemory(id int64) error {
	if err := t.DeleteMemoryFTS(id); err != nil {
		return err
	}
	_, err := t.tx.Exec(`DELETE FROM memories WHERE id = ?`, id)
	return storeErr(err)
}

// AddTag / RemoveTag manage a memory's tags.
func (t *Tx) AddTag(id int64, tag string) error {
	_, err := t.tx.Exec(`INSERT OR IGNORE INTO memory_tags (memory_id, tag) VALUES (?, ?)`, id, tag)
	return storeErr(err)
}

func (t *Tx) RemoveTag(id int64, tag string) error {
	_, err := t.tx.Exec(`DELETE FROM memory_tags WHERE memory_id = ? AND tag = ?`, id, tag)
	return storeErr(err)
}

// AddEvidence / RemoveEvidence manage git-anchored evidence.
func (t *Tx) AddEvidence(id int64, path string, start, end int, blobOID string) error {
	_, err := t.tx.Exec(`INSERT INTO memory_evidence (memory_id, path, line_start, line_end, blob_oid_at_creation)
		VALUES (?, ?, ?, ?, ?)`, id, path, start, end, blobOID)
	return storeErr(err)
}

func (t *Tx) RemoveEvidence(id int64, path string, start, end int) error {
	_, err := t.tx.Exec(`DELETE FROM memory_evidence WHERE memory_id = ? AND path = ? AND line_start = ? AND line_end = ?`,
		id, path, start, end)
	return storeErr(err)
}

// AddLink / RemoveLink manage file/symbol links.
func (t *Tx) AddLink(id int64, targetType, targetRef string) error {
	_, err := t.tx.Exec(`INSERT OR IGNORE INTO memory_links (memory_id, target_type, target_ref) VALUES (?, ?, ?)`,
		id, targetType, targetRef)
	return storeErr(err)
}

func (t *Tx) RemoveLink(id int64, targetType, targetRef string) error {
	_, err := t.tx.Exec(`DELETE FROM memory_links WHERE memory_id = ? AND target_type = ? AND target_ref = ?`,
		id, targetType, targetRef)
	return storeErr(err)
}

// DeleteMemoryFTS removes a memory's FTS row.
func (t *Tx) DeleteMemoryFTS(id int64) error {
	_, err := t.tx.Exec(`DELETE FROM memory_fts WHERE memory_id = ?`, id)
	return storeErr(err)
}

// ReindexMemoryFTS rebuilds a memory's FTS row from its current title/body/tags.
func (t *Tx) ReindexMemoryFTS(id int64, title, body string, tags []string) error {
	if err := t.DeleteMemoryFTS(id); err != nil {
		return err
	}
	_, err := t.tx.Exec(`INSERT INTO memory_fts (title, body, tags, memory_id) VALUES (?, ?, ?, ?)`,
		title, body, strings.Join(tags, " "), id)
	return storeErr(err)
}

// MemoryExists reports whether a memory id exists.
func (d *DB) MemoryExists(id int64) (bool, error) {
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE id = ?`, id).Scan(&n); err != nil {
		return false, storeErr(err)
	}
	return n > 0, nil
}

// ListMemories returns memory summaries filtered by optional kind and tag.
func (d *DB) ListMemories(kind, tag string) ([]MemoryBrief, error) {
	query := `SELECT DISTINCT m.id, m.kind, m.title FROM memories m`
	var args []any
	var where []string
	if tag != "" {
		query += ` JOIN memory_tags t ON t.memory_id = m.id`
		where = append(where, `t.tag = ?`)
		args = append(args, tag)
	}
	if kind != "" {
		where = append(where, `m.kind = ?`)
		args = append(args, kind)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY m.id`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []MemoryBrief
	for rows.Next() {
		var m MemoryBrief
		if err := rows.Scan(&m.ID, &m.Kind, &m.Title); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AllMemoryIDs returns every memory id (for validate).
func (d *DB) AllMemoryIDs() ([]int64, error) {
	rows, err := d.db.Query(`SELECT id FROM memories ORDER BY id`)
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

package store

import (
	"database/sql"
	"errors"
)

// MemoryBrief is a memory summary for search/enrichment.
type MemoryBrief struct {
	ID    int64
	Kind  string
	Title string
	Tags  []string
}

// SearchMemoryFTS returns memory ids matching the FTS query, best first.
func (d *DB) SearchMemoryFTS(match string, limit int) ([]int64, error) {
	rows, err := d.db.Query(`SELECT memory_id FROM memory_fts
		WHERE memory_fts MATCH ? ORDER BY bm25(memory_fts) LIMIT ?`, match, limit)
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

// MemoryBriefByID fetches a memory summary by id.
func (d *DB) MemoryBriefByID(id int64) (MemoryBrief, bool, error) {
	var m MemoryBrief
	err := d.db.QueryRow(`SELECT id, kind, title FROM memories WHERE id = ?`, id).Scan(&m.ID, &m.Kind, &m.Title)
	if err != nil {
		if isNoRows(err) {
			return MemoryBrief{}, false, nil
		}
		return MemoryBrief{}, false, storeErr(err)
	}
	return m, true, nil
}

// MemoriesForTarget returns memories linked to a file path or symbol name.
func (d *DB) MemoriesForTarget(targetType, targetRef string) ([]MemoryBrief, error) {
	rows, err := d.db.Query(`SELECT m.id, m.kind, m.title FROM memory_links l
		JOIN memories m ON m.id = l.memory_id
		WHERE l.target_type = ? AND l.target_ref = ? ORDER BY m.id`, targetType, targetRef)
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

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

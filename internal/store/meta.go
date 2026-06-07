package store

import (
	"database/sql"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

// Meta is the singleton index-metadata row.
type Meta struct {
	SchemaVersion int
	ProjectID     string
	IndexedHead   string
	Dirty         bool
	MemSeq        int64
	FilesCount    int
	SymbolsCount  int
	LastIndexedAt string
}

// MetaRepo reads and writes the index_meta singleton.
type MetaRepo struct {
	db *sql.DB
}

// Get returns the singleton metadata row.
func (r *MetaRepo) Get() (Meta, error) {
	var m Meta
	var dirty int
	err := r.db.QueryRow(`SELECT schema_version, project_id, indexed_head, dirty,
		mem_seq, files_count, symbols_count, last_indexed_at FROM index_meta WHERE id = 1`).
		Scan(&m.SchemaVersion, &m.ProjectID, &m.IndexedHead, &dirty,
			&m.MemSeq, &m.FilesCount, &m.SymbolsCount, &m.LastIndexedAt)
	if err != nil {
		return Meta{}, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	m.Dirty = dirty != 0
	return m, nil
}

// SetProjectID stores the project identifier.
func (r *MetaRepo) SetProjectID(id string) error {
	_, err := r.db.Exec(`UPDATE index_meta SET project_id = ? WHERE id = 1`, id)
	if err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return nil
}

// NextMemSeq atomically increments and returns the memory id counter. IDs are
// never reused. Must be called under the writer lock (a transaction or the
// single-connection pool guarantees this within a process).
func (r *MetaRepo) NextMemSeq() (int64, error) {
	var n int64
	err := r.db.QueryRow(`UPDATE index_meta SET mem_seq = mem_seq + 1 WHERE id = 1
		RETURNING mem_seq`).Scan(&n)
	if err != nil {
		return 0, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return n, nil
}

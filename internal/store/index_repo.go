package store

import (
	"strconv"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

// FileRecord is a file row to be written.
type FileRecord struct {
	Path          string
	Language      string
	Package       string
	Role          string
	BlobOID       string
	ContentSHA256 string
	GrainEligible bool
}

// SymbolRecord is a symbol row to be written.
type SymbolRecord struct {
	Name      string
	Kind      string
	Container string
	Signature string
	Exported  bool
}

// FileHashes returns path -> effective content hash (blob_oid for tracked,
// else content_sha256) for every indexed file. Used for change detection.
func (d *DB) FileHashes() (map[string]string, error) {
	rows, err := d.db.Query(`SELECT path,
		CASE WHEN blob_oid != '' THEN blob_oid ELSE content_sha256 END FROM files`)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			return nil, storeErr(err)
		}
		out[p] = h
	}
	return out, rows.Err()
}

// ClearIndex removes all index data (files cascade to symbols/imports/exports/
// todos/dep_edges/test_links) and the code FTS, preserving memories and their
// links/evidence.
func (t *Tx) ClearIndex() error {
	for _, stmt := range []string{
		`DELETE FROM files`,
		`DELETE FROM code_fts`,
		`UPDATE index_meta SET files_count = 0, symbols_count = 0 WHERE id = 1`,
	} {
		if _, err := t.tx.Exec(stmt); err != nil {
			return storeErr(err)
		}
	}
	return nil
}

// DeleteFileByPath removes a file and its dependent rows (cascade) plus its FTS
// rows.
func (t *Tx) DeleteFileByPath(path string) error {
	if _, err := t.tx.Exec(`DELETE FROM code_fts WHERE path = ?`, path); err != nil {
		return storeErr(err)
	}
	if _, err := t.tx.Exec(`DELETE FROM files WHERE path = ?`, path); err != nil {
		return storeErr(err)
	}
	return nil
}

// PutFile replaces a file and all its derived rows, writing the file row, its
// symbols, imports, exports and todos, plus the corresponding FTS rows. Returns
// the number of symbols written.
func (t *Tx) PutFile(f FileRecord, syms []SymbolRecord, imports, exports []string, todos []TodoRecord) (int, error) {
	if err := t.DeleteFileByPath(f.Path); err != nil {
		return 0, err
	}
	res, err := t.tx.Exec(`INSERT INTO files (path, language, package, role, blob_oid, content_sha256, grain_eligible)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.Path, f.Language, f.Package, f.Role, f.BlobOID, f.ContentSHA256, boolToInt(f.GrainEligible))
	if err != nil {
		return 0, storeErr(err)
	}
	fileID, err := res.LastInsertId()
	if err != nil {
		return 0, storeErr(err)
	}

	// File-grain FTS row.
	if _, err := t.tx.Exec(`INSERT INTO code_fts (name, signature, path, package, grain, ref_id)
		VALUES ('', '', ?, ?, 'file', ?)`, f.Path, f.Package, strconv.FormatInt(fileID, 10)); err != nil {
		return 0, storeErr(err)
	}

	for _, s := range syms {
		sres, err := t.tx.Exec(`INSERT INTO symbols (file_id, name, kind, container, signature, exported)
			VALUES (?, ?, ?, ?, ?, ?)`, fileID, s.Name, s.Kind, s.Container, s.Signature, boolToInt(s.Exported))
		if err != nil {
			return 0, storeErr(err)
		}
		symID, err := sres.LastInsertId()
		if err != nil {
			return 0, storeErr(err)
		}
		if _, err := t.tx.Exec(`INSERT INTO code_fts (name, signature, path, package, grain, ref_id)
			VALUES (?, ?, ?, ?, 'symbol', ?)`, s.Name, s.Signature, f.Path, f.Package, strconv.FormatInt(symID, 10)); err != nil {
			return 0, storeErr(err)
		}
	}

	for _, spec := range imports {
		if _, err := t.tx.Exec(`INSERT INTO imports (file_id, specifier) VALUES (?, ?)`, fileID, spec); err != nil {
			return 0, storeErr(err)
		}
	}
	for _, name := range exports {
		if _, err := t.tx.Exec(`INSERT INTO exports (file_id, name) VALUES (?, ?)`, fileID, name); err != nil {
			return 0, storeErr(err)
		}
	}
	for _, td := range todos {
		if _, err := t.tx.Exec(`INSERT INTO todos (file_id, line, text) VALUES (?, ?, ?)`, fileID, td.Line, td.Text); err != nil {
			return 0, storeErr(err)
		}
	}
	return len(syms), nil
}

// TodoRecord is a todo row to be written.
type TodoRecord struct {
	Line int
	Text string
}

// SetIndexState records HEAD, dirty flag and stats after an index run.
func (t *Tx) SetIndexState(head string, dirty bool, filesCount, symbolsCount int, indexedAt string) error {
	_, err := t.tx.Exec(`UPDATE index_meta SET indexed_head = ?, dirty = ?, files_count = ?,
		symbols_count = ?, last_indexed_at = ? WHERE id = 1`,
		head, boolToInt(dirty), filesCount, symbolsCount, indexedAt)
	if err != nil {
		return storeErr(err)
	}
	return nil
}

// CountFiles returns the number of indexed files inside the transaction.
func (t *Tx) CountFiles() (int, error) { return t.count("SELECT COUNT(*) FROM files") }

// CountSymbols returns the number of indexed symbols inside the transaction.
func (t *Tx) CountSymbols() (int, error) { return t.count("SELECT COUNT(*) FROM symbols") }

func (t *Tx) count(query string) (int, error) {
	var n int
	if err := t.tx.QueryRow(query).Scan(&n); err != nil {
		return 0, storeErr(err)
	}
	return n, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func storeErr(err error) error {
	if err == nil {
		return nil
	}
	return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
}

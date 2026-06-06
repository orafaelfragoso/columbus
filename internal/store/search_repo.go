package store

import "database/sql"

// CodeHit is one FTS candidate from code_fts.
type CodeHit struct {
	Grain string // "symbol" | "file"
	RefID int64
}

// SearchCodeFTS returns up to limit candidates matching the FTS query, best
// (lowest bm25) first. The bm25 score is a candidate-generation signal only and
// is intentionally not returned (final ranking is deterministic).
func (d *DB) SearchCodeFTS(match string, limit int) ([]CodeHit, error) {
	rows, err := d.db.Query(`SELECT grain, ref_id FROM code_fts
		WHERE code_fts MATCH ? ORDER BY bm25(code_fts) LIMIT ?`, match, limit)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []CodeHit
	for rows.Next() {
		var grain string
		var ref int64
		if err := rows.Scan(&grain, &ref); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, CodeHit{Grain: grain, RefID: ref})
	}
	return out, rows.Err()
}

// SymbolRow is a symbol joined with its file metadata.
type SymbolRow struct {
	ID        int64
	FileID    int64
	Name      string
	Kind      string
	Container string
	Signature string
	Exported  bool
	Path      string
	Package   string
	Role      string
	Language  string
}

// SymbolByID fetches a symbol with its file metadata.
func (d *DB) SymbolByID(id int64) (SymbolRow, bool, error) {
	var s SymbolRow
	var exported int
	err := d.db.QueryRow(`SELECT s.id, s.file_id, s.name, s.kind, s.container, s.signature, s.exported,
		f.path, f.package, f.role, f.language
		FROM symbols s JOIN files f ON f.id = s.file_id WHERE s.id = ?`, id).
		Scan(&s.ID, &s.FileID, &s.Name, &s.Kind, &s.Container, &s.Signature, &exported,
			&s.Path, &s.Package, &s.Role, &s.Language)
	if err == sql.ErrNoRows {
		return SymbolRow{}, false, nil
	}
	if err != nil {
		return SymbolRow{}, false, storeErr(err)
	}
	s.Exported = exported != 0
	return s, true, nil
}

// FileByID fetches a file row by id.
func (d *DB) FileByID(id int64) (FileRow, bool, error) {
	var f FileRow
	err := d.db.QueryRow(`SELECT id, path, role, language, package FROM files WHERE id = ?`, id).
		Scan(&f.ID, &f.Path, &f.Role, &f.Language, &f.Package)
	if err == sql.ErrNoRows {
		return FileRow{}, false, nil
	}
	if err != nil {
		return FileRow{}, false, storeErr(err)
	}
	return f, true, nil
}

// FileByPath fetches a file row by exact path.
func (d *DB) FileByPath(path string) (FileRow, bool, error) {
	var f FileRow
	err := d.db.QueryRow(`SELECT id, path, role, language, package FROM files WHERE path = ?`, path).
		Scan(&f.ID, &f.Path, &f.Role, &f.Language, &f.Package)
	if err == sql.ErrNoRows {
		return FileRow{}, false, nil
	}
	if err != nil {
		return FileRow{}, false, storeErr(err)
	}
	return f, true, nil
}

// ImportsOf returns resolved import target paths for a file (1-hop out).
func (d *DB) ImportsOf(fileID int64) ([]string, error) {
	return d.pathQuery(`SELECT f.path FROM dep_edges e JOIN files f ON f.id = e.to_file_id
		WHERE e.from_file_id = ? ORDER BY f.path`, fileID)
}

// ImportedBy returns paths of files that import the given file (1-hop in).
func (d *DB) ImportedBy(fileID int64) ([]string, error) {
	return d.pathQuery(`SELECT f.path FROM dep_edges e JOIN files f ON f.id = e.from_file_id
		WHERE e.to_file_id = ? ORDER BY f.path`, fileID)
}

// TestsOf returns test file paths linked to an implementation file.
func (d *DB) TestsOf(fileID int64) ([]string, error) {
	return d.pathQuery(`SELECT f.path FROM test_links t JOIN files f ON f.id = t.test_file_id
		WHERE t.impl_file_id = ? ORDER BY f.path`, fileID)
}

// ImportedByCount returns the in-degree of a file (centrality signal).
func (d *DB) ImportedByCount(fileID int64) (int, error) {
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM dep_edges WHERE to_file_id = ?`, fileID).Scan(&n); err != nil {
		return 0, storeErr(err)
	}
	return n, nil
}

func (d *DB) pathQuery(query string, args ...any) ([]string, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

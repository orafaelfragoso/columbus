package store

// FileRow is a file with its identity and classification.
type FileRow struct {
	ID       int64
	Path     string
	Role     string
	Language string
	Package  string
}

// ImportRow is a raw import specifier tied to a file id.
type ImportRow struct {
	FileID    int64
	Specifier string
}

// AllFiles returns every indexed file (id, path, role, language, package).
func (d *DB) AllFiles() ([]FileRow, error) {
	rows, err := d.db.Query(`SELECT id, path, role, language, package FROM files ORDER BY id`)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []FileRow
	for rows.Next() {
		var f FileRow
		if err := rows.Scan(&f.ID, &f.Path, &f.Role, &f.Language, &f.Package); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// AllImports returns every raw import specifier with its file id.
func (d *DB) AllImports() ([]ImportRow, error) {
	rows, err := d.db.Query(`SELECT file_id, specifier FROM imports`)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []ImportRow
	for rows.Next() {
		var r ImportRow
		if err := rows.Scan(&r.FileID, &r.Specifier); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReplaceGraph clears and rewrites the resolved dependency edges and test links.
func (t *Tx) ReplaceGraph(edges [][2]int64, testLinks [][2]int64) error {
	for _, stmt := range []string{`DELETE FROM dep_edges`, `DELETE FROM test_links`} {
		if _, err := t.tx.Exec(stmt); err != nil {
			return storeErr(err)
		}
	}
	for _, e := range edges {
		if _, err := t.tx.Exec(`INSERT OR IGNORE INTO dep_edges (from_file_id, to_file_id) VALUES (?, ?)`, e[0], e[1]); err != nil {
			return storeErr(err)
		}
	}
	for _, l := range testLinks {
		if _, err := t.tx.Exec(`INSERT OR IGNORE INTO test_links (impl_file_id, test_file_id) VALUES (?, ?)`, l[0], l[1]); err != nil {
			return storeErr(err)
		}
	}
	return nil
}

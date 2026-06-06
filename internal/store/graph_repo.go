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

// PathEdge is a directed edge between two file paths.
type PathEdge struct {
	From string
	To   string
}

// PathSpecifier is an import specifier tied to the importing file's path.
type PathSpecifier struct {
	Path      string
	Specifier string
}

// AllDepEdges returns every resolved file->file import edge as path pairs,
// ordered deterministically.
func (d *DB) AllDepEdges() ([]PathEdge, error) {
	return d.pathEdges(`SELECT f1.path, f2.path FROM dep_edges d
		JOIN files f1 ON f1.id = d.from_file_id
		JOIN files f2 ON f2.id = d.to_file_id
		ORDER BY f1.path, f2.path`)
}

// AllTestLinks returns every impl->test link as path pairs, ordered
// deterministically.
func (d *DB) AllTestLinks() ([]PathEdge, error) {
	return d.pathEdges(`SELECT fi.path, ft.path FROM test_links tl
		JOIN files fi ON fi.id = tl.impl_file_id
		JOIN files ft ON ft.id = tl.test_file_id
		ORDER BY fi.path, ft.path`)
}

func (d *DB) pathEdges(query string) ([]PathEdge, error) {
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []PathEdge
	for rows.Next() {
		var e PathEdge
		if err := rows.Scan(&e.From, &e.To); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UnresolvedImports returns the import specifiers that did not resolve to an
// indexed file, tied to the importing file's path, ordered deterministically.
func (d *DB) UnresolvedImports() ([]PathSpecifier, error) {
	rows, err := d.db.Query(`SELECT f.path, i.specifier FROM imports i
		JOIN files f ON f.id = i.file_id
		WHERE i.resolved_file_id IS NULL
		ORDER BY f.path, i.specifier`)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []PathSpecifier
	for rows.Next() {
		var p PathSpecifier
		if err := rows.Scan(&p.Path, &p.Specifier); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, p)
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

package store

// SymbolsByName returns all symbols with the given name (across files).
func (d *DB) SymbolsByName(name string) ([]SymbolRow, error) {
	return d.scanSymbols(`SELECT s.id, s.file_id, s.name, s.kind, s.container, s.signature, s.exported,
		f.path, f.package, f.role, f.language
		FROM symbols s JOIN files f ON f.id = s.file_id
		WHERE s.name = ? ORDER BY f.path, s.container`, name)
}

// SymbolsInFile returns the symbols defined in a file, ordered for outline use.
func (d *DB) SymbolsInFile(fileID int64) ([]SymbolRow, error) {
	return d.scanSymbols(`SELECT s.id, s.file_id, s.name, s.kind, s.container, s.signature, s.exported,
		f.path, f.package, f.role, f.language
		FROM symbols s JOIN files f ON f.id = s.file_id
		WHERE s.file_id = ? ORDER BY s.id`, fileID)
}

func (d *DB) scanSymbols(query string, args ...any) ([]SymbolRow, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []SymbolRow
	for rows.Next() {
		var s SymbolRow
		var exported int
		if err := rows.Scan(&s.ID, &s.FileID, &s.Name, &s.Kind, &s.Container, &s.Signature, &exported,
			&s.Path, &s.Package, &s.Role, &s.Language); err != nil {
			return nil, storeErr(err)
		}
		s.Exported = exported != 0
		out = append(out, s)
	}
	return out, rows.Err()
}

// SuggestPaths returns indexed paths containing the substring (did-you-mean).
func (d *DB) SuggestPaths(substr string, limit int) ([]string, error) {
	return d.pathQuery(`SELECT path FROM files WHERE path LIKE '%' || ? || '%' ORDER BY path LIMIT ?`, substr, limit)
}

// SuggestSymbols returns symbol names similar to the query (did-you-mean):
// names that contain the query, are a prefix of it, or have it as a prefix.
func (d *DB) SuggestSymbols(query string, limit int) ([]string, error) {
	rows, err := d.db.Query(`SELECT DISTINCT name FROM symbols
		WHERE name LIKE '%' || ? || '%'
		   OR ? LIKE name || '%'
		   OR name LIKE ? || '%'
		ORDER BY name LIMIT ?`, query, query, query, limit)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

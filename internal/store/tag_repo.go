package store

// TagCount is one tag and the number of knowledge entities carrying it across
// the whole project (memories + epics/stories/tasks).
type TagCount struct {
	Tag   string
	Count int
}

// DistinctTags returns every distinct tag attached to any knowledge entity
// (memory or work item) with its usage count, ordered by tag ascending.
func (d *DB) DistinctTags() ([]TagCount, error) {
	const query = `
SELECT tag, COUNT(*) AS n FROM (
    SELECT tag FROM memory_tags
    UNION ALL
    SELECT tag FROM work_tags
)
GROUP BY tag
ORDER BY tag`
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []TagCount
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

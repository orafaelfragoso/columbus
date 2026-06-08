package store

import "strings"

// Story is a full story record with its associations. A story belongs to one
// epic and owns zero or more tasks.
type Story struct {
	ID        int64
	EpicID    int64
	Title     string
	Body      string
	Status    string
	CreatedAt string
	UpdatedAt string
	Tags      []string
	Refs      []WorkRef
}

// StoryBrief is a story summary for list/search output.
type StoryBrief struct {
	ID     int64
	EpicID int64
	Title  string
	Status string
}

// StoryFull fetches a story and its tags/refs by numeric id.
func (d *DB) StoryFull(id int64) (Story, bool, error) {
	var s Story
	err := d.db.QueryRow(`SELECT id, epic_id, title, body, status, created_at, updated_at FROM stories WHERE id = ?`, id).
		Scan(&s.ID, &s.EpicID, &s.Title, &s.Body, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return Story{}, false, nil
		}
		return Story{}, false, storeErr(err)
	}
	tags, err := d.pathQuery(`SELECT tag FROM work_tags WHERE owner_type = 'story' AND owner_id = ? ORDER BY tag`, id)
	if err != nil {
		return Story{}, false, err
	}
	s.Tags = tags
	refs, err := d.workRefs("story", id)
	if err != nil {
		return Story{}, false, err
	}
	s.Refs = refs
	return s, true, nil
}

// StoryEpicID returns the parent epic id of a story, and whether it exists.
func (d *DB) StoryEpicID(id int64) (int64, bool, error) {
	var epicID int64
	err := d.db.QueryRow(`SELECT epic_id FROM stories WHERE id = ?`, id).Scan(&epicID)
	if err != nil {
		if isNoRows(err) {
			return 0, false, nil
		}
		return 0, false, storeErr(err)
	}
	return epicID, true, nil
}

// ListStories returns story summaries filtered by optional epic id (0 = any),
// status and tag, ordered by id ascending.
func (d *DB) ListStories(epicID int64, status, tag string) ([]StoryBrief, error) {
	query := `SELECT DISTINCT s.id, s.epic_id, s.title, s.status FROM stories s`
	var args []any
	var where []string
	if tag != "" {
		query += ` JOIN work_tags wt ON wt.owner_type = 'story' AND wt.owner_id = s.id`
		where = append(where, `wt.tag = ?`)
		args = append(args, tag)
	}
	if epicID != 0 {
		where = append(where, `s.epic_id = ?`)
		args = append(args, epicID)
	}
	if status != "" {
		where = append(where, `s.status = ?`)
		args = append(args, status)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY s.id`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []StoryBrief
	for rows.Next() {
		var b StoryBrief
		if err := rows.Scan(&b.ID, &b.EpicID, &b.Title, &b.Status); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AllStoryIDs returns every story id ascending (for validate/embed).
func (d *DB) AllStoryIDs() ([]int64, error) {
	return d.idList(`SELECT id FROM stories ORDER BY id`)
}

// StoryExists reports whether a story id exists.
func (d *DB) StoryExists(id int64) (bool, error) {
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM stories WHERE id = ?`, id).Scan(&n); err != nil {
		return false, storeErr(err)
	}
	return n > 0, nil
}

// WorkOwner returns the title and status of a single epic/story/task owner.
func (d *DB) WorkOwner(ownerType string, id int64) (WorkOwner, bool, error) {
	var table string
	switch ownerType {
	case "epic":
		table = "epics"
	case "story":
		table = "stories"
	case "task":
		table = "tasks"
	default:
		return WorkOwner{}, false, nil
	}
	o := WorkOwner{OwnerType: ownerType, OwnerID: id}
	err := d.db.QueryRow(`SELECT title, status FROM `+table+` WHERE id = ?`, id).Scan(&o.Title, &o.Status)
	if err != nil {
		if isNoRows(err) {
			return WorkOwner{}, false, nil
		}
		return WorkOwner{}, false, storeErr(err)
	}
	return o, true, nil
}

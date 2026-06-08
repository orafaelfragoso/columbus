package work

import (
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// StoryAddParams are the inputs to StoryAdd.
type StoryAddParams struct {
	Epic  string
	Title string
	Body  string
	Tags  []string
}

// StoryAdd creates a story under an existing epic (status todo) and auto-logs
// the initial event. A missing epic is NOT_FOUND.
func (m *Manager) StoryAdd(p StoryAddParams) (StoryResult, error) {
	if strings.TrimSpace(p.Title) == "" {
		return StoryResult{}, contract.Errorf(contract.CodeUsage, "story --title is required")
	}
	epicID, err := ParseEpicID(p.Epic)
	if err != nil {
		return StoryResult{}, err
	}
	if ok, err := m.DB.EpicExists(epicID); err != nil {
		return StoryResult{}, err
	} else if !ok {
		return StoryResult{}, notFound("epic", p.Epic)
	}
	now := m.now()
	var id int64
	err = m.DB.WithTx(func(tx *store.Tx) error {
		var e error
		if id, e = tx.NextStorySeq(); e != nil {
			return e
		}
		if e = tx.InsertStory(id, epicID, p.Title, p.Body, StatusDefault, now, now); e != nil {
			return e
		}
		for _, tag := range dedupe(p.Tags) {
			if e = tx.AddWorkTag("story", id, tag); e != nil {
				return e
			}
		}
		return tx.AppendWorkEvent("story", id, StatusDefault, "", now)
	})
	if err != nil {
		return StoryResult{}, err
	}
	return m.loadStory(id)
}

// StoryEditParams are partial changes; a non-nil Epic re-parents the story.
type StoryEditParams struct {
	Title      *string
	Body       *string
	Epic       *string
	AddTags    []string
	RemoveTags []string
}

func (p StoryEditParams) empty() bool {
	return p.Title == nil && p.Body == nil && p.Epic == nil &&
		len(p.AddTags) == 0 && len(p.RemoveTags) == 0
}

// StoryEdit applies partial non-historical changes to a story, including
// re-parenting to another (existing) epic.
func (m *Manager) StoryEdit(idStr string, p StoryEditParams) (StoryResult, error) {
	id, err := ParseStoryID(idStr)
	if err != nil {
		return StoryResult{}, err
	}
	if p.empty() {
		return StoryResult{}, contract.Errorf(contract.CodeUsage, "edit requires at least one change")
	}
	cur, ok, err := m.DB.StoryFull(id)
	if err != nil {
		return StoryResult{}, err
	}
	if !ok {
		return StoryResult{}, notFound("story", idStr)
	}
	newEpicID := cur.EpicID
	if p.Epic != nil {
		newEpicID, err = ParseEpicID(*p.Epic)
		if err != nil {
			return StoryResult{}, err
		}
		if ok, err := m.DB.EpicExists(newEpicID); err != nil {
			return StoryResult{}, err
		} else if !ok {
			return StoryResult{}, notFound("epic", *p.Epic)
		}
	}
	title, body := cur.Title, cur.Body
	if p.Title != nil {
		title = *p.Title
	}
	if p.Body != nil {
		body = *p.Body
	}
	err = m.DB.WithTx(func(tx *store.Tx) error {
		if e := tx.UpdateStory(id, title, body, m.now()); e != nil {
			return e
		}
		if p.Epic != nil {
			if e := tx.ReparentStory(id, newEpicID, m.now()); e != nil {
				return e
			}
		}
		return applyTagChanges(tx, "story", id, p.AddTags, p.RemoveTags)
	})
	if err != nil {
		return StoryResult{}, err
	}
	return m.loadStory(id)
}

// StoryStatus appends a status-change event and denormalizes the new status.
func (m *Manager) StoryStatus(idStr, to, comment string) (StoryResult, error) {
	id, err := m.statusPrep("story", idStr, to)
	if err != nil {
		return StoryResult{}, err
	}
	now := m.now()
	err = m.DB.WithTx(func(tx *store.Tx) error {
		if e := tx.AppendWorkEvent("story", id, to, comment, now); e != nil {
			return e
		}
		return tx.SetStoryStatus(id, to, now)
	})
	if err != nil {
		return StoryResult{}, err
	}
	return m.loadStory(id)
}

// StoryComment appends a comment-only note (status stays NULL).
func (m *Manager) StoryComment(idStr, text string) (StoryResult, error) {
	id, err := m.commentPrep("story", idStr, text)
	if err != nil {
		return StoryResult{}, err
	}
	if err := m.appendComment("story", id, text); err != nil {
		return StoryResult{}, err
	}
	return m.loadStory(id)
}

// StoryDelete hard-deletes a story (cascading its tasks). force is required.
func (m *Manager) StoryDelete(idStr string, force bool) (DeleteResult, error) {
	id, err := ParseStoryID(idStr)
	if err != nil {
		return DeleteResult{}, err
	}
	if !force {
		return DeleteResult{}, contract.Errorf(contract.CodeUsage, "delete requires --force (destructive; id retired)")
	}
	ok, err := m.DB.StoryExists(id)
	if err != nil {
		return DeleteResult{}, err
	}
	if !ok {
		return DeleteResult{}, notFound("story", idStr)
	}
	if err := m.DB.WithTx(func(tx *store.Tx) error { return tx.DeleteStory(id) }); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{command: "story", ID: FormatStoryID(id), Removed: true}, nil
}

// StoryList returns story summaries filtered by optional epic, status and tag.
func (m *Manager) StoryList(epic, status, tag string) (StoryListResult, error) {
	if status != "" && !validStatus(status) {
		return StoryListResult{}, invalidStatus(status)
	}
	var epicID int64
	if epic != "" {
		var err error
		if epicID, err = ParseEpicID(epic); err != nil {
			return StoryListResult{}, err
		}
	}
	briefs, err := m.DB.ListStories(epicID, status, tag)
	if err != nil {
		return StoryListResult{}, err
	}
	res := StoryListResult{Epic: epic, Status: status, Tag: tag, Counts: map[string]int{}}
	for _, b := range briefs {
		res.Stories = append(res.Stories, StoryRef{ID: FormatStoryID(b.ID), Epic: FormatEpicID(b.EpicID), Title: b.Title, Status: b.Status})
		res.Counts[b.Status]++
	}
	res.Total = len(res.Stories)
	return res, nil
}

func (m *Manager) loadStory(id int64) (StoryResult, error) {
	if err := m.reindexFTS("story", id); err != nil {
		return StoryResult{}, err
	}
	full, ok, err := m.DB.StoryFull(id)
	if err != nil {
		return StoryResult{}, err
	}
	if !ok {
		return StoryResult{}, notFound("story", FormatStoryID(id))
	}
	return storyResultFrom(full), nil
}

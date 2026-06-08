package knowledge

import "github.com/orafaelfragoso/columbus/internal/work"

// editWork applies metadata changes (title/body/tags and re-parenting) to a
// work entity. Parent re-parents a story to an epic or a task to a story; it is
// rejected for epics, which have no parent.
func (m *Manager) editWork(kind, id string, p UpdateParams) (Item, error) {
	wm := m.work()
	switch kind {
	case "epic":
		if p.Parent != nil {
			return Item{}, parentNotAllowed("epic")
		}
		r, err := wm.EpicEdit(id, work.EpicEditParams{
			Title: p.Title, Body: p.Body, AddTags: p.AddTags, RemoveTags: p.RemoveTags,
		})
		return itemFromEpic(r), err
	case "story":
		r, err := wm.StoryEdit(id, work.StoryEditParams{
			Title: p.Title, Body: p.Body, Epic: p.Parent, AddTags: p.AddTags, RemoveTags: p.RemoveTags,
		})
		return itemFromStory(r), err
	default: // task
		r, err := wm.TaskEdit(id, work.TaskEditParams{
			Title: p.Title, Body: p.Body, Story: p.Parent, AddTags: p.AddTags, RemoveTags: p.RemoveTags,
		})
		return itemFromTask(r), err
	}
}

func (m *Manager) statusWork(kind, id, status, comment string) (Item, error) {
	wm := m.work()
	switch kind {
	case "epic":
		r, err := wm.EpicStatus(id, status, comment)
		return itemFromEpic(r), err
	case "story":
		r, err := wm.StoryStatus(id, status, comment)
		return itemFromStory(r), err
	default:
		r, err := wm.TaskStatus(id, status, comment)
		return itemFromTask(r), err
	}
}

func (m *Manager) commentWork(kind, id, comment string) (Item, error) {
	wm := m.work()
	switch kind {
	case "epic":
		r, err := wm.EpicComment(id, comment)
		return itemFromEpic(r), err
	case "story":
		r, err := wm.StoryComment(id, comment)
		return itemFromStory(r), err
	default:
		r, err := wm.TaskComment(id, comment)
		return itemFromTask(r), err
	}
}

func (m *Manager) refWork(kind, id string, add, remove []work.RefSpec) (Item, error) {
	wm := m.work()
	switch kind {
	case "epic":
		r, err := wm.EpicRef(id, add, remove)
		return itemFromEpic(r), err
	case "story":
		r, err := wm.StoryRef(id, add, remove)
		return itemFromStory(r), err
	default:
		r, err := wm.TaskRef(id, add, remove)
		return itemFromTask(r), err
	}
}

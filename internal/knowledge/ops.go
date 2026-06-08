package knowledge

import (
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/work"
)

// AddParams are the unified inputs to Add. Field relevance depends on kind:
// Parent links a story to its epic or a task to its story; Type is the context
// sub-kind; Evidence/Refs apply to context entries.
type AddParams struct {
	Type     string // context sub-kind (memory.Kinds); required for context
	Title    string
	Body     string
	Parent   string // epic_NNN for a story, story_NNN for a task
	Tags     []string
	Refs     []string // context links: file:<path> | symbol:<name>
	Evidence []string // context only: path:start-end
}

// Add creates a knowledge entity of the given kind.
func (m *Manager) Add(kind string, p AddParams) (Item, error) {
	if err := requireMutableKind(kind); err != nil {
		return Item{}, err
	}
	switch kind {
	case "epic":
		r, err := m.work().EpicAdd(work.EpicAddParams{Title: p.Title, Body: p.Body, Tags: p.Tags})
		return itemFromEpic(r), err
	case "story":
		r, err := m.work().StoryAdd(work.StoryAddParams{Epic: p.Parent, Title: p.Title, Body: p.Body, Tags: p.Tags})
		return itemFromStory(r), err
	case "task":
		r, err := m.work().TaskAdd(work.TaskAddParams{Story: p.Parent, Title: p.Title, Body: p.Body, Tags: p.Tags})
		return itemFromTask(r), err
	default: // context
		ev, err := parseEvidence(p.Evidence)
		if err != nil {
			return Item{}, err
		}
		lk, err := parseLinks(p.Refs)
		if err != nil {
			return Item{}, err
		}
		r, err := m.mem().Add(memory.AddParams{
			Kind: p.Type, Title: p.Title, Body: p.Body, Evidence: ev, Links: lk, Tags: p.Tags,
		})
		return itemFromMemory(r), err
	}
}

// UpdateParams are partial changes; nil pointers / empty slices mean unchanged.
// Status and Comment apply to work kinds (they append events); Type re-types a
// context entry. Refs/Evidence map to work references or context links/evidence.
type UpdateParams struct {
	Title          *string
	Body           *string
	Parent         *string // re-parent a story (to an epic) or task (to a story)
	Type           *string // re-type a context entry
	Status         string  // work kinds: append a status event
	Comment        string  // work kinds: append a comment event
	AddTags        []string
	RemoveTags     []string
	AddRefs        []string
	RemoveRefs     []string
	AddEvidence    []string // context only
	RemoveEvidence []string // context only
}

func (p UpdateParams) hasMeta() bool {
	return p.Title != nil || p.Body != nil || p.Parent != nil ||
		len(p.AddTags) > 0 || len(p.RemoveTags) > 0
}

func (p UpdateParams) empty() bool {
	return !p.hasMeta() && p.Type == nil && p.Status == "" && p.Comment == "" &&
		len(p.AddRefs) == 0 && len(p.RemoveRefs) == 0 &&
		len(p.AddEvidence) == 0 && len(p.RemoveEvidence) == 0
}

// Update applies partial changes to an existing entity. For work kinds it may
// span several events (metadata edit, status change, comment, reference edits)
// and returns the entity's final state.
func (m *Manager) Update(kind, id string, p UpdateParams) (Item, error) {
	if err := requireMutableKind(kind); err != nil {
		return Item{}, err
	}
	if p.empty() {
		return Item{}, contract.Errorf(contract.CodeUsage, "update requires at least one change")
	}
	if kind == "context" {
		return m.updateContext(id, p)
	}
	return m.updateWork(kind, id, p)
}

// updateWork chains the work engine's edit/status/comment/ref operations and
// returns the last result (each call reloads the full entity).
func (m *Manager) updateWork(kind, id string, p UpdateParams) (Item, error) {
	if p.Type != nil {
		return Item{}, contract.Errorf(contract.CodeUsage, "--type applies only to context entries")
	}
	if p.AddEvidence != nil || p.RemoveEvidence != nil {
		return Item{}, contract.Errorf(contract.CodeUsage, "--evidence applies only to context entries")
	}
	addRefs, removeRefs, err := parseWorkRefs(p.AddRefs, p.RemoveRefs)
	if err != nil {
		return Item{}, err
	}

	var last Item
	step := func(it Item, err error) error {
		if err != nil {
			return err
		}
		last = it
		return nil
	}

	if p.hasMeta() {
		if err := step(m.editWork(kind, id, p)); err != nil {
			return Item{}, err
		}
	}
	if p.Status != "" {
		if err := step(m.statusWork(kind, id, p.Status, p.Comment)); err != nil {
			return Item{}, err
		}
	} else if p.Comment != "" {
		if err := step(m.commentWork(kind, id, p.Comment)); err != nil {
			return Item{}, err
		}
	}
	if len(addRefs) > 0 || len(removeRefs) > 0 {
		if err := step(m.refWork(kind, id, addRefs, removeRefs)); err != nil {
			return Item{}, err
		}
	}
	return last, nil
}

func (m *Manager) updateContext(id string, p UpdateParams) (Item, error) {
	if p.Parent != nil {
		return Item{}, contract.Errorf(contract.CodeUsage, "context entries have no parent")
	}
	if p.Status != "" || p.Comment != "" {
		return Item{}, contract.Errorf(contract.CodeUsage, "status and comment apply only to work kinds")
	}
	addEv, err := parseEvidence(p.AddEvidence)
	if err != nil {
		return Item{}, err
	}
	rmEv, err := parseEvidence(p.RemoveEvidence)
	if err != nil {
		return Item{}, err
	}
	addLk, err := parseLinks(p.AddRefs)
	if err != nil {
		return Item{}, err
	}
	rmLk, err := parseLinks(p.RemoveRefs)
	if err != nil {
		return Item{}, err
	}
	r, err := m.mem().Edit(id, memory.EditParams{
		Title: p.Title, Body: p.Body, Kind: p.Type,
		AddTags: p.AddTags, RemoveTags: p.RemoveTags,
		AddEvidence: addEv, RemoveEvidence: rmEv,
		AddLinks: addLk, RemoveLinks: rmLk,
	})
	return itemFromMemory(r), err
}

// Remove hard-deletes an entity. force is required for every kind.
func (m *Manager) Remove(kind, id string, force bool) (RemoveResult, error) {
	if err := requireMutableKind(kind); err != nil {
		return RemoveResult{}, err
	}
	if !force {
		return RemoveResult{}, contract.Errorf(contract.CodeUsage, "remove requires --force (destructive; id retired)")
	}
	switch kind {
	case "epic":
		r, err := m.work().EpicDelete(id, true)
		return RemoveResult{Kind: kind, ID: r.ID, Removed: r.Removed}, err
	case "story":
		r, err := m.work().StoryDelete(id, true)
		return RemoveResult{Kind: kind, ID: r.ID, Removed: r.Removed}, err
	case "task":
		r, err := m.work().TaskDelete(id, true)
		return RemoveResult{Kind: kind, ID: r.ID, Removed: r.Removed}, err
	default: // context
		r, err := m.mem().Remove(id)
		return RemoveResult{Kind: kind, ID: r.ID, Removed: r.Removed}, err
	}
}

// ListFilter narrows a list. Parent filters work children by their parent id;
// Type filters context entries by sub-kind; Status/Tag are general.
type ListFilter struct {
	Type   string
	Parent string
	Status string
	Tag    string
}

// List returns a unified projection of one kind. For tag it returns a
// TagListResult; for every other kind a ListResult.
func (m *Manager) List(kind string, f ListFilter) (render.Payload, error) {
	if !validKind(kind) {
		return nil, unknownKind(kind)
	}
	switch kind {
	case "epic":
		r, err := m.work().EpicList(f.Status, f.Tag)
		if err != nil {
			return nil, err
		}
		return epicList(r, f), nil
	case "story":
		r, err := m.work().StoryList(f.Parent, f.Status, f.Tag)
		if err != nil {
			return nil, err
		}
		return storyList(r, f), nil
	case "task":
		epicFilter, storyFilter := splitTaskParent(f.Parent)
		r, err := m.work().TaskList(epicFilter, storyFilter, f.Status, f.Tag)
		if err != nil {
			return nil, err
		}
		return taskList(r, f), nil
	case "context":
		r, err := m.mem().List(f.Type, f.Tag)
		if err != nil {
			return nil, err
		}
		return contextList(r, f), nil
	default: // tag
		tags, err := m.DB.DistinctTags()
		if err != nil {
			return nil, err
		}
		out := TagListResult{Total: len(tags)}
		for _, t := range tags {
			out.Tags = append(out.Tags, TagCount{Tag: t.Tag, Count: t.Count})
		}
		return out, nil
	}
}

// splitTaskParent routes a --parent value to the right task-list filter: an
// epic_ id filters by epic, a story_ id (or empty) by story.
func splitTaskParent(parent string) (epic, story string) {
	if strings.HasPrefix(parent, "epic_") {
		return parent, ""
	}
	return "", parent
}

// --- list converters ---

func epicList(r work.EpicListResult, f ListFilter) ListResult {
	out := ListResult{Kind: "epic", Status: f.Status, Tag: f.Tag, Total: r.Total, Counts: r.Counts}
	for _, e := range r.Epics {
		out.Items = append(out.Items, ListItem{Kind: "epic", ID: e.ID, Title: e.Title, Status: e.Status})
	}
	return out
}

func storyList(r work.StoryListResult, f ListFilter) ListResult {
	out := ListResult{Kind: "story", Parent: f.Parent, Status: f.Status, Tag: f.Tag, Total: r.Total, Counts: r.Counts}
	for _, s := range r.Stories {
		out.Items = append(out.Items, ListItem{Kind: "story", ID: s.ID, Parent: s.Epic, Title: s.Title, Status: s.Status})
	}
	return out
}

func taskList(r work.TaskListResult, f ListFilter) ListResult {
	out := ListResult{Kind: "task", Parent: f.Parent, Status: f.Status, Tag: f.Tag, Total: r.Total, Counts: r.Counts}
	for _, t := range r.Tasks {
		out.Items = append(out.Items, ListItem{Kind: "task", ID: t.ID, Parent: t.Story, Title: t.Title, Status: t.Status})
	}
	return out
}

func contextList(r memory.ListResult, f ListFilter) ListResult {
	out := ListResult{Kind: "context", Type: f.Type, Tag: f.Tag, Total: r.Total, Counts: r.Counts}
	for _, mr := range r.Memories {
		out.Items = append(out.Items, ListItem{Kind: "context", ID: mr.ID, Type: mr.Kind, Title: mr.Title})
	}
	return out
}

// --- shared parsing helpers ---

func parseEvidence(specs []string) ([]memory.EvidenceSpec, error) {
	var out []memory.EvidenceSpec
	for _, s := range specs {
		ev, err := memory.ParseEvidence(s)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func parseLinks(specs []string) ([]memory.LinkSpec, error) {
	var out []memory.LinkSpec
	for _, s := range specs {
		l, err := memory.ParseLink(s)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func parseWorkRefs(adds, removes []string) (add, remove []work.RefSpec, err error) {
	for _, s := range adds {
		spec, perr := work.ParseRef(s)
		if perr != nil {
			return nil, nil, perr
		}
		add = append(add, spec)
	}
	for _, s := range removes {
		spec, perr := work.ParseRef(s)
		if perr != nil {
			return nil, nil, perr
		}
		remove = append(remove, spec)
	}
	return add, remove, nil
}

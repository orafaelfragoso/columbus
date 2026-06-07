package show

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// WorkRefView is a reference with its current drift status.
type WorkRefView struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
	Resolved   bool   `json:"resolved"`
}

// WorkEventView is one entry in the append-only history.
type WorkEventView struct {
	Status    string `json:"status,omitempty"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at"`
}

// ChildTask is a task summary under an epic.
type ChildTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// EpicView is the typed result of `show epic`.
type EpicView struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Body      string          `json:"body,omitempty"`
	Status    string          `json:"status"`
	Tags      []string        `json:"tags,omitempty"`
	Refs      []WorkRefView   `json:"refs,omitempty"`
	Events    []WorkEventView `json:"events,omitempty"`
	Tasks     []ChildTask     `json:"tasks,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
}

func (EpicView) CommandName() string { return "show" }

func (r EpicView) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s\n", r.ID, r.Status, r.Title)
	if r.Body != "" {
		fmt.Fprintf(w, "\n%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	renderRefs(w, r.Refs)
	renderTasks(w, r.Tasks)
	renderEvents(w, r.Events)
	return nil
}

func (r EpicView) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%s)\n\n**%s**\n\n%s\n", r.ID, r.Status, r.Title, r.Body)
	if len(r.Tasks) > 0 {
		fmt.Fprintf(w, "\n## Tasks\n\n")
		for _, ta := range r.Tasks {
			fmt.Fprintf(w, "- **%s** [%s] %s\n", ta.ID, ta.Status, ta.Title)
		}
	}
	return nil
}

// TaskView is the typed result of `show task`.
type TaskView struct {
	ID        string          `json:"id"`
	Epic      string          `json:"epic"`
	Title     string          `json:"title"`
	Body      string          `json:"body,omitempty"`
	Status    string          `json:"status"`
	Tags      []string        `json:"tags,omitempty"`
	Refs      []WorkRefView   `json:"refs,omitempty"`
	Events    []WorkEventView `json:"events,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
}

func (TaskView) CommandName() string { return "show" }

func (r TaskView) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s (%s)\n", r.ID, r.Status, r.Title, r.Epic)
	if r.Body != "" {
		fmt.Fprintf(w, "\n%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	renderRefs(w, r.Refs)
	renderEvents(w, r.Events)
	return nil
}

func (r TaskView) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%s)\n\n**%s** — epic %s\n\n%s\n", r.ID, r.Status, r.Title, r.Epic, r.Body)
	return nil
}

func renderRefs(w io.Writer, refs []WorkRefView) {
	for _, ref := range refs {
		drift := ""
		if !ref.Resolved {
			drift = " (drift)"
		}
		fmt.Fprintf(w, "ref: %s:%s%s\n", ref.TargetType, ref.TargetRef, drift)
	}
}

func renderTasks(w io.Writer, tasks []ChildTask) {
	for _, ta := range tasks {
		fmt.Fprintf(w, "task: %s [%s] %s\n", ta.ID, ta.Status, ta.Title)
	}
}

func renderEvents(w io.Writer, events []WorkEventView) {
	for _, e := range events {
		switch {
		case e.Status != "" && e.Comment != "":
			fmt.Fprintf(w, "  %s -> %s: %s\n", e.CreatedAt, e.Status, e.Comment)
		case e.Status != "":
			fmt.Fprintf(w, "  %s -> %s\n", e.CreatedAt, e.Status)
		default:
			fmt.Fprintf(w, "  %s note: %s\n", e.CreatedAt, e.Comment)
		}
	}
}

// Epic returns an epic with its tags, refs (inline drift), event history and
// child tasks.
func (s *Shower) Epic(id string) (EpicView, error) {
	n, err := parseWorkID(id, "epic_", "epic")
	if err != nil {
		return EpicView{}, err
	}
	full, ok, err := s.DB.EpicFull(n)
	if err != nil {
		return EpicView{}, err
	}
	if !ok {
		return EpicView{}, notFound("epic", id, nil)
	}
	view := EpicView{
		ID: formatWorkID("epic_", full.ID), Title: full.Title, Body: full.Body, Status: full.Status,
		Tags: full.Tags, Refs: s.refViews(full.Refs), Events: s.eventViews("epic", full.ID),
		CreatedAt: full.CreatedAt, UpdatedAt: full.UpdatedAt,
	}
	tasks, err := s.DB.ListTasks(full.ID, "", "")
	if err != nil {
		return EpicView{}, err
	}
	for _, ta := range tasks {
		view.Tasks = append(view.Tasks, ChildTask{ID: formatWorkID("task_", ta.ID), Title: ta.Title, Status: ta.Status})
	}
	return view, nil
}

// Task returns a task with its tags, refs (inline drift) and event history.
func (s *Shower) Task(id string) (TaskView, error) {
	n, err := parseWorkID(id, "task_", "task")
	if err != nil {
		return TaskView{}, err
	}
	full, ok, err := s.DB.TaskFull(n)
	if err != nil {
		return TaskView{}, err
	}
	if !ok {
		return TaskView{}, notFound("task", id, nil)
	}
	return TaskView{
		ID: formatWorkID("task_", full.ID), Epic: formatWorkID("epic_", full.EpicID),
		Title: full.Title, Body: full.Body, Status: full.Status, Tags: full.Tags,
		Refs: s.refViews(full.Refs), Events: s.eventViews("task", full.ID),
		CreatedAt: full.CreatedAt, UpdatedAt: full.UpdatedAt,
	}, nil
}

func (s *Shower) refViews(refs []store.WorkRef) []WorkRefView {
	if len(refs) == 0 {
		return nil
	}
	out := make([]WorkRefView, len(refs))
	for i, r := range refs {
		out[i] = WorkRefView{TargetType: r.TargetType, TargetRef: r.TargetRef, Resolved: s.refResolves(r)}
	}
	return out
}

func (s *Shower) eventViews(ownerType string, id int64) []WorkEventView {
	events, err := s.DB.WorkEvents(ownerType, id)
	s.logErr("WorkEvents", err)
	if len(events) == 0 {
		return nil
	}
	out := make([]WorkEventView, len(events))
	for i, e := range events {
		out[i] = WorkEventView{Status: e.NewStatus, Comment: e.Comment, CreatedAt: e.CreatedAt}
	}
	return out
}

// refResolves reports whether a work reference target exists in the index.
func (s *Shower) refResolves(ref store.WorkRef) bool {
	switch ref.TargetType {
	case "file":
		_, ok, _ := s.DB.FileByPath(ref.TargetRef)
		return ok
	case "dir":
		ok, _ := s.DB.HasFilesUnderDir(ref.TargetRef)
		return ok
	case "memory":
		v, err := strconv.ParseInt(strings.TrimPrefix(ref.TargetRef, "mem_"), 10, 64)
		if err != nil || v <= 0 {
			return false
		}
		ok, _ := s.DB.MemoryExists(v)
		return ok
	case "symbol":
		rows, _ := s.DB.SymbolsByName(ref.TargetRef)
		return len(rows) > 0
	}
	return false
}

func parseWorkID(id, prefix, label string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimPrefix(id, prefix), 10, 64)
	if err != nil || v <= 0 {
		return 0, contract.Errorf(contract.CodeUsage, "invalid %s id %q (want %sNNN)", label, id, prefix)
	}
	return v, nil
}

func formatWorkID(prefix string, id int64) string { return fmt.Sprintf("%s%03d", prefix, id) }

// WorkRefBack is an epic or task that references the entity being shown
// (reverse lookup: "what work touches this file/symbol/memory?").
type WorkRefBack struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// workRefsTo returns the epics/tasks that reference a given target.
func (s *Shower) workRefsTo(targetType, ref string) []WorkRefBack {
	owners, err := s.DB.WorkForTarget(targetType, ref)
	s.logErr("WorkForTarget", err)
	if len(owners) == 0 {
		return nil
	}
	out := make([]WorkRefBack, len(owners))
	for i, o := range owners {
		out[i] = WorkRefBack{Kind: o.OwnerType, ID: formatWorkID(o.OwnerType+"_", o.OwnerID), Title: o.Title, Status: o.Status}
	}
	return out
}

func renderWork(w io.Writer, work []WorkRefBack) {
	for _, b := range work {
		fmt.Fprintf(w, "%s %s [%s]: %s\n", b.Kind, b.ID, b.Status, b.Title)
	}
}

package work

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// Ref is a rendered epic/task reference.
type Ref struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
}

// EpicResult is the typed result of an epic mutation (a single record echo).
type EpicResult struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags,omitempty"`
	Refs      []Ref    `json:"refs,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func (EpicResult) CommandName() string { return "epic" }

func (r EpicResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s\n", r.ID, r.Status, r.Title)
	if r.Body != "" {
		fmt.Fprintf(w, "%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	for _, ref := range r.Refs {
		fmt.Fprintf(w, "ref: %s:%s\n", ref.TargetType, ref.TargetRef)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	return nil
}

func (r EpicResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%s)\n\n**%s**\n\n%s\n", r.ID, r.Status, r.Title, r.Body)
	return nil
}

// TaskResult is the typed result of a task mutation (a single record echo).
type TaskResult struct {
	ID        string   `json:"id"`
	Epic      string   `json:"epic"`
	Story     string   `json:"story,omitempty"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags,omitempty"`
	Refs      []Ref    `json:"refs,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func (TaskResult) CommandName() string { return "task" }

func (r TaskResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s (%s)\n", r.ID, r.Status, r.Title, r.Story)
	if r.Body != "" {
		fmt.Fprintf(w, "%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	for _, ref := range r.Refs {
		fmt.Fprintf(w, "ref: %s:%s\n", ref.TargetType, ref.TargetRef)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	return nil
}

func (r TaskResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%s)\n\n**%s** — story %s (epic %s)\n\n%s\n", r.ID, r.Status, r.Title, r.Story, r.Epic, r.Body)
	return nil
}

// StoryResult is the typed result of a story mutation (a single record echo).
type StoryResult struct {
	ID        string   `json:"id"`
	Epic      string   `json:"epic"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags,omitempty"`
	Refs      []Ref    `json:"refs,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func (StoryResult) CommandName() string { return "story" }

func (r StoryResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s (%s)\n", r.ID, r.Status, r.Title, r.Epic)
	if r.Body != "" {
		fmt.Fprintf(w, "%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	for _, ref := range r.Refs {
		fmt.Fprintf(w, "ref: %s:%s\n", ref.TargetType, ref.TargetRef)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	return nil
}

func (r StoryResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%s)\n\n**%s** — epic %s\n\n%s\n", r.ID, r.Status, r.Title, r.Epic, r.Body)
	return nil
}

// DeleteResult is the typed result of an epic/task delete. The command field is
// set by the caller so the json envelope names the right noun.
type DeleteResult struct {
	command string
	ID      string `json:"id"`
	Removed bool   `json:"removed"`
}

func (r DeleteResult) CommandName() string { return r.command }
func (r DeleteResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "removed %s\n", r.ID)
	return nil
}
func (r DeleteResult) RenderLLM(w io.Writer, o render.Options) error { return r.RenderText(w, o) }

// EpicRef / TaskRef are list-row summaries.
type EpicRef struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type TaskRef struct {
	ID     string `json:"id"`
	Epic   string `json:"epic"`
	Story  string `json:"story,omitempty"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// StoryRef is a story list-row summary.
type StoryRef struct {
	ID     string `json:"id"`
	Epic   string `json:"epic"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// EpicListResult is the typed result of epic list.
type EpicListResult struct {
	Status string         `json:"status,omitempty"`
	Tag    string         `json:"tag,omitempty"`
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
	Epics  []EpicRef      `json:"epics"`
}

func (EpicListResult) CommandName() string { return "epic" }

func (r EpicListResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%d epics\n", r.Total)
	for _, e := range r.Epics {
		fmt.Fprintf(w, "  %s [%s] %s\n", e.ID, e.Status, e.Title)
	}
	renderCounts(w, r.Counts)
	return nil
}

func (r EpicListResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Epics (%d)\n\n", r.Total)
	for _, e := range r.Epics {
		fmt.Fprintf(w, "- **%s** [%s] %s\n", e.ID, e.Status, e.Title)
	}
	return nil
}

// TaskListResult is the typed result of task list.
type TaskListResult struct {
	Epic   string         `json:"epic,omitempty"`
	Story  string         `json:"story,omitempty"`
	Status string         `json:"status,omitempty"`
	Tag    string         `json:"tag,omitempty"`
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
	Tasks  []TaskRef      `json:"tasks"`
}

func (TaskListResult) CommandName() string { return "task" }

func (r TaskListResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%d tasks\n", r.Total)
	for _, ta := range r.Tasks {
		fmt.Fprintf(w, "  %s [%s] %s (%s)\n", ta.ID, ta.Status, ta.Title, ta.Story)
	}
	renderCounts(w, r.Counts)
	return nil
}

func (r TaskListResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Tasks (%d)\n\n", r.Total)
	for _, ta := range r.Tasks {
		fmt.Fprintf(w, "- **%s** [%s] %s (%s)\n", ta.ID, ta.Status, ta.Title, ta.Story)
	}
	return nil
}

// StoryListResult is the typed result of story list.
type StoryListResult struct {
	Epic    string         `json:"epic,omitempty"`
	Status  string         `json:"status,omitempty"`
	Tag     string         `json:"tag,omitempty"`
	Total   int            `json:"total"`
	Counts  map[string]int `json:"counts"`
	Stories []StoryRef     `json:"stories"`
}

func (StoryListResult) CommandName() string { return "story" }

func (r StoryListResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%d stories\n", r.Total)
	for _, s := range r.Stories {
		fmt.Fprintf(w, "  %s [%s] %s (%s)\n", s.ID, s.Status, s.Title, s.Epic)
	}
	renderCounts(w, r.Counts)
	return nil
}

func (r StoryListResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Stories (%d)\n\n", r.Total)
	for _, s := range r.Stories {
		fmt.Fprintf(w, "- **%s** [%s] %s (%s)\n", s.ID, s.Status, s.Title, s.Epic)
	}
	return nil
}

// RefStatus is one reference's drift outcome.
type RefStatus struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
	Resolved   bool   `json:"resolved"`
}

// WorkValidation is the drift report for one epic or task.
type WorkValidation struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Status   string      `json:"status"`
	Refs     []RefStatus `json:"refs,omitempty"`
	Warnings []string    `json:"warnings,omitempty"`
}

// ValidateResult is the typed result of epic/task validate. The command field
// is set by the caller so the json envelope names the right noun.
type ValidateResult struct {
	command    string
	Total      int              `json:"total"`
	Unresolved int              `json:"unresolved"`
	Healthy    bool             `json:"healthy"`
	Entities   []WorkValidation `json:"entities"`
}

func (r ValidateResult) CommandName() string { return r.command }

func (r *ValidateResult) add(v WorkValidation) {
	for _, ref := range v.Refs {
		if !ref.Resolved {
			r.Unresolved++
		}
	}
	r.Entities = append(r.Entities, v)
}

func (r *ValidateResult) finalize() {
	r.Total = len(r.Entities)
	r.Healthy = r.Unresolved == 0
}

func (r ValidateResult) RenderText(w io.Writer, _ render.Options) error {
	for _, e := range r.Entities {
		marker := "ok"
		if len(e.Warnings) > 0 {
			marker = "drift"
		}
		fmt.Fprintf(w, "%s [%s] %s — %s\n", e.ID, e.Status, e.Title, marker)
		for _, warn := range e.Warnings {
			fmt.Fprintf(w, "    %s\n", warn)
		}
	}
	fmt.Fprintf(w, "\n%d entities; %d with unresolved refs\n", r.Total, r.Unresolved)
	return nil
}

func (r ValidateResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Validation\n\n- total: %d\n- unresolved: %d\n- healthy: %t\n",
		r.Total, r.Unresolved, r.Healthy)
	return nil
}

func renderCounts(w io.Writer, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	fmt.Fprintf(w, "by status: %s\n", strings.Join(parts, ", "))
}

func epicResultFrom(e store.Epic) EpicResult {
	return EpicResult{
		ID: FormatEpicID(e.ID), Title: e.Title, Body: e.Body, Status: e.Status,
		Tags: e.Tags, Refs: refsFrom(e.Refs), CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

func taskResultFrom(ta store.Task) TaskResult {
	return TaskResult{
		ID: FormatTaskID(ta.ID), Epic: FormatEpicID(ta.EpicID), Story: FormatStoryID(ta.StoryID),
		Title: ta.Title, Body: ta.Body,
		Status: ta.Status, Tags: ta.Tags, Refs: refsFrom(ta.Refs), CreatedAt: ta.CreatedAt, UpdatedAt: ta.UpdatedAt,
	}
}

func storyResultFrom(s store.Story) StoryResult {
	return StoryResult{
		ID: FormatStoryID(s.ID), Epic: FormatEpicID(s.EpicID), Title: s.Title, Body: s.Body, Status: s.Status,
		Tags: s.Tags, Refs: refsFrom(s.Refs), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func refsFrom(in []store.WorkRef) []Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]Ref, len(in))
	for i, r := range in {
		out[i] = Ref{TargetType: r.TargetType, TargetRef: r.TargetRef}
	}
	return out
}

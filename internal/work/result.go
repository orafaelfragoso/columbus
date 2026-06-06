package work

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/render"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// EpicResult is the typed result of an epic mutation (a single record echo).
type EpicResult struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags,omitempty"`
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
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func (TaskResult) CommandName() string { return "task" }

func (r TaskResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s (%s)\n", r.ID, r.Status, r.Title, r.Epic)
	if r.Body != "" {
		fmt.Fprintf(w, "%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	return nil
}

func (r TaskResult) RenderLLM(w io.Writer, _ render.Options) error {
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
		fmt.Fprintf(w, "  %s [%s] %s (%s)\n", ta.ID, ta.Status, ta.Title, ta.Epic)
	}
	renderCounts(w, r.Counts)
	return nil
}

func (r TaskListResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Tasks (%d)\n\n", r.Total)
	for _, ta := range r.Tasks {
		fmt.Fprintf(w, "- **%s** [%s] %s (%s)\n", ta.ID, ta.Status, ta.Title, ta.Epic)
	}
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
		Tags: e.Tags, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

func taskResultFrom(ta store.Task) TaskResult {
	return TaskResult{
		ID: FormatTaskID(ta.ID), Epic: FormatEpicID(ta.EpicID), Title: ta.Title, Body: ta.Body,
		Status: ta.Status, Tags: ta.Tags, CreatedAt: ta.CreatedAt, UpdatedAt: ta.UpdatedAt,
	}
}

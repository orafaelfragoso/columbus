package knowledge

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/work"
)

// commandName is the json envelope command for every unified knowledge payload.
const commandName = "memory"

// Ref is a unified reference/link on an item.
type Ref struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
}

// Evidence is a unified evidence anchor (context items only).
type Evidence struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

// Item is the unified single-record echo for add/update across every kind.
type Item struct {
	Kind      string     `json:"kind"`
	ID        string     `json:"id"`
	Type      string     `json:"type,omitempty"`   // context sub-kind (decision/...)
	Parent    string     `json:"parent,omitempty"` // epic of a story, story of a task
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	Status    string     `json:"status,omitempty"` // work kinds only
	Tags      []string   `json:"tags,omitempty"`
	Refs      []Ref      `json:"refs,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
	CreatedAt string     `json:"created_at,omitempty"`
	UpdatedAt string     `json:"updated_at,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
}

func (Item) CommandName() string { return commandName }

func (r Item) RenderText(w io.Writer, _ render.Options) error {
	head := r.Status
	if head == "" {
		head = r.Type
	}
	if head != "" {
		fmt.Fprintf(w, "%s [%s] %s", r.ID, head, r.Title)
	} else {
		fmt.Fprintf(w, "%s %s", r.ID, r.Title)
	}
	if r.Parent != "" {
		fmt.Fprintf(w, " (%s)", r.Parent)
	}
	fmt.Fprintln(w)
	if r.Body != "" {
		fmt.Fprintf(w, "%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	for _, e := range r.Evidence {
		fmt.Fprintf(w, "evidence: %s:%d-%d\n", e.Path, e.LineStart, e.LineEnd)
	}
	for _, ref := range r.Refs {
		fmt.Fprintf(w, "ref: %s:%s\n", ref.TargetType, ref.TargetRef)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	return nil
}

func (r Item) RenderLLM(w io.Writer, _ render.Options) error {
	tag := r.Status
	if tag == "" {
		tag = r.Type
	}
	fmt.Fprintf(w, "# %s (%s) %s\n\n", r.Kind, tag, r.ID)
	fmt.Fprintf(w, "**%s**\n\n%s\n", r.Title, r.Body)
	return nil
}

// RemoveResult is the unified result of remove.
type RemoveResult struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Removed bool   `json:"removed"`
}

func (RemoveResult) CommandName() string { return commandName }
func (r RemoveResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "removed %s %s\n", r.Kind, r.ID)
	return nil
}
func (r RemoveResult) RenderLLM(w io.Writer, o render.Options) error { return r.RenderText(w, o) }

// ListItem is one row in a unified list.
type ListItem struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Type   string `json:"type,omitempty"`
	Parent string `json:"parent,omitempty"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
}

// ListResult is the unified result of list for a concrete kind.
type ListResult struct {
	Kind   string         `json:"kind"`
	Type   string         `json:"type,omitempty"`
	Parent string         `json:"parent,omitempty"`
	Status string         `json:"status,omitempty"`
	Tag    string         `json:"tag,omitempty"`
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
	Items  []ListItem     `json:"items"`
}

func (ListResult) CommandName() string { return commandName }

func (r ListResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%d %s\n", r.Total, plural(r.Kind))
	for _, it := range r.Items {
		marker := it.Status
		if marker == "" {
			marker = it.Type
		}
		if marker != "" {
			fmt.Fprintf(w, "  %s [%s] %s", it.ID, marker, it.Title)
		} else {
			fmt.Fprintf(w, "  %s %s", it.ID, it.Title)
		}
		if it.Parent != "" {
			fmt.Fprintf(w, " (%s)", it.Parent)
		}
		fmt.Fprintln(w)
	}
	renderCounts(w, r.Counts)
	return nil
}

func (r ListResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%d)\n\n", titleFirst(plural(r.Kind)), r.Total)
	for _, it := range r.Items {
		marker := it.Status
		if marker == "" {
			marker = it.Type
		}
		fmt.Fprintf(w, "- **%s** [%s] %s\n", it.ID, marker, it.Title)
	}
	return nil
}

// TagCount is one tag and its usage count.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// TagListResult is the result of `memory list tag`.
type TagListResult struct {
	Total int        `json:"total"`
	Tags  []TagCount `json:"tags"`
}

func (TagListResult) CommandName() string { return commandName }

func (r TagListResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%d tags\n", r.Total)
	for _, t := range r.Tags {
		fmt.Fprintf(w, "  %s (%d)\n", t.Tag, t.Count)
	}
	return nil
}

func (r TagListResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Tags (%d)\n\n", r.Total)
	for _, t := range r.Tags {
		fmt.Fprintf(w, "- **%s** (%d)\n", t.Tag, t.Count)
	}
	return nil
}

// --- converters from the per-kind engine results into the unified Item ---

func itemFromEpic(r work.EpicResult) Item {
	return Item{
		Kind: "epic", ID: r.ID, Title: r.Title, Body: r.Body, Status: r.Status,
		Tags: r.Tags, Refs: refsFromWork(r.Refs),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Warnings: r.Warnings,
	}
}

func itemFromStory(r work.StoryResult) Item {
	return Item{
		Kind: "story", ID: r.ID, Parent: r.Epic, Title: r.Title, Body: r.Body, Status: r.Status,
		Tags: r.Tags, Refs: refsFromWork(r.Refs),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Warnings: r.Warnings,
	}
}

func itemFromTask(r work.TaskResult) Item {
	return Item{
		Kind: "task", ID: r.ID, Parent: r.Story, Title: r.Title, Body: r.Body, Status: r.Status,
		Tags: r.Tags, Refs: refsFromWork(r.Refs),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Warnings: r.Warnings,
	}
}

func itemFromMemory(r memory.MemoryResult) Item {
	it := Item{
		Kind: "context", ID: r.ID, Type: r.Kind, Title: r.Title, Body: r.Body,
		Tags: r.Tags, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Warnings: r.Warnings,
	}
	for _, e := range r.Evidence {
		it.Evidence = append(it.Evidence, Evidence{Path: e.Path, LineStart: e.LineStart, LineEnd: e.LineEnd})
	}
	for _, l := range r.Links {
		it.Refs = append(it.Refs, Ref{TargetType: l.TargetType, TargetRef: l.TargetRef})
	}
	return it
}

func refsFromWork(in []work.Ref) []Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]Ref, len(in))
	for i, r := range in {
		out[i] = Ref{TargetType: r.TargetType, TargetRef: r.TargetRef}
	}
	return out
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

// titleFirst upper-cases the first rune of s (ASCII headings only).
func titleFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func plural(kind string) string {
	switch kind {
	case "story":
		return "stories"
	case "context":
		return "context entries"
	default:
		return kind + "s"
	}
}

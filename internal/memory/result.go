package memory

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// MemoryRef is a memory summary.
type MemoryRef struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"`
	Title string   `json:"title"`
	Tags  []string `json:"tags,omitempty"`
}

// Evidence is a rendered evidence anchor.
type Evidence struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

// Link is a rendered link.
type Link struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
}

// MemoryResult is the typed result of add/edit/link (a single record).
type MemoryResult struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
	Links     []Link     `json:"links,omitempty"`
	CreatedAt string     `json:"created_at,omitempty"`
	UpdatedAt string     `json:"updated_at,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
}

func (MemoryResult) CommandName() string { return "memory" }

func (r MemoryResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s\n", r.ID, r.Kind, r.Title)
	if r.Body != "" {
		fmt.Fprintf(w, "%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(r.Tags, ", "))
	}
	for _, e := range r.Evidence {
		fmt.Fprintf(w, "evidence: %s:%d-%d\n", e.Path, e.LineStart, e.LineEnd)
	}
	for _, l := range r.Links {
		fmt.Fprintf(w, "link: %s:%s\n", l.TargetType, l.TargetRef)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	return nil
}

func (r MemoryResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%s)\n\n**%s**\n\n%s\n", r.ID, r.Kind, r.Title, r.Body)
	return nil
}

// RemoveResult is the typed result of remove.
type RemoveResult struct {
	ID      string `json:"id"`
	Removed bool   `json:"removed"`
}

func (RemoveResult) CommandName() string { return "memory" }
func (r RemoveResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "removed %s\n", r.ID)
	return nil
}
func (r RemoveResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "removed %s\n", r.ID)
	return nil
}

// ListResult is the typed result of list.
type ListResult struct {
	Kind     string         `json:"kind,omitempty"`
	Tag      string         `json:"tag,omitempty"`
	Total    int            `json:"total"`
	Counts   map[string]int `json:"counts"`
	Memories []MemoryRef    `json:"memories"`
}

func (ListResult) CommandName() string { return "memory" }

func (r ListResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%d memories\n", r.Total)
	for _, m := range r.Memories {
		fmt.Fprintf(w, "  %s [%s] %s\n", m.ID, m.Kind, m.Title)
	}
	if len(r.Counts) > 0 {
		var parts []string
		for _, k := range sortedKeys(r.Counts) {
			parts = append(parts, fmt.Sprintf("%s %d", k, r.Counts[k]))
		}
		fmt.Fprintf(w, "by kind: %s\n", strings.Join(parts, ", "))
	}
	return nil
}

func (r ListResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Memories (%d)\n\n", r.Total)
	for _, m := range r.Memories {
		fmt.Fprintf(w, "- **%s** [%s] %s\n", m.ID, m.Kind, m.Title)
	}
	return nil
}

// EvidenceStatus is one evidence anchor's validation outcome.
type EvidenceStatus struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Status    string `json:"status"` // ok | stale | broken
}

// LinkStatus is one link's validation outcome.
type LinkStatus struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
	Resolved   bool   `json:"resolved"`
}

// MemoryValidation is the validation report for one memory.
type MemoryValidation struct {
	ID       string           `json:"id"`
	Kind     string           `json:"kind"`
	Title    string           `json:"title"`
	Evidence []EvidenceStatus `json:"evidence,omitempty"`
	Links    []LinkStatus     `json:"links,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

// ValidateResult is the typed result of validate.
type ValidateResult struct {
	Total      int                `json:"total"`
	Drifted    int                `json:"drifted"`
	Unresolved int                `json:"unresolved"`
	Healthy    bool               `json:"healthy"`
	Memories   []MemoryValidation `json:"memories"`
}

func (ValidateResult) CommandName() string { return "memory" }

func (r ValidateResult) RenderText(w io.Writer, _ render.Options) error {
	for _, m := range r.Memories {
		marker := "ok"
		if len(m.Warnings) > 0 {
			marker = "drift"
		}
		fmt.Fprintf(w, "%s [%s] %s — %s\n", m.ID, m.Kind, m.Title, marker)
		for _, warn := range m.Warnings {
			fmt.Fprintf(w, "    %s\n", warn)
		}
	}
	fmt.Fprintf(w, "\n%d memories; %d with evidence drift, %d with unresolved links\n",
		r.Total, r.Drifted, r.Unresolved)
	return nil
}

func (r ValidateResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Memory validation\n\n- total: %d\n- drifted: %d\n- unresolved: %d\n- healthy: %t\n",
		r.Total, r.Drifted, r.Unresolved, r.Healthy)
	return nil
}

func (r ImportResult) RenderText(w io.Writer, _ render.Options) error {
	mode := "reassigned ids"
	if r.PreserveIDs {
		mode = "preserved ids"
	}
	fmt.Fprintf(w, "imported %d, skipped %d of %d (%s)\n", r.Imported, r.Skipped, r.Total, mode)
	return nil
}

func (r ImportResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Memory import\n\n- imported: %d\n- skipped: %d\n- total: %d\n- preserve_ids: %t\n",
		r.Imported, r.Skipped, r.Total, r.PreserveIDs)
	return nil
}

func resultFrom(m store.Memory) MemoryResult {
	out := MemoryResult{
		ID: FormatID(m.ID), Kind: m.Kind, Title: m.Title, Body: m.Body,
		Tags: m.Tags, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	for _, e := range m.Evidence {
		out.Evidence = append(out.Evidence, Evidence{Path: e.Path, LineStart: e.LineStart, LineEnd: e.LineEnd})
	}
	for _, l := range m.Links {
		out.Links = append(out.Links, Link{TargetType: l.TargetType, TargetRef: l.TargetRef})
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

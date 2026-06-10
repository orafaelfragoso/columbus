package search

import (
	"fmt"
	"io"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/render"
)

// GraphInfo is the 1-hop neighborhood of a result.
type GraphInfo struct {
	Imports    []string `json:"imports,omitempty"`
	ImportedBy []string `json:"imported_by,omitempty"`
	Tests      []string `json:"tests,omitempty"`
}

// MemoryRef is a memory linked to a result.
type MemoryRef struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// MemoryEvidence is a stored evidence anchor on a memory.
type MemoryEvidence struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

// MemoryLink is a stored link on a memory.
type MemoryLink struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
}

// MemoryDetail is one top-ranked memory expanded to its full record, so a
// single search returns complete memory context (no follow-up `show memory`).
type MemoryDetail struct {
	ID       string           `json:"id"`
	Kind     string           `json:"kind"`
	Title    string           `json:"title"`
	Body     string           `json:"body,omitempty"`
	Tags     []string         `json:"tags,omitempty"`
	Links    []MemoryLink     `json:"links,omitempty"`
	Evidence []MemoryEvidence `json:"evidence,omitempty"`
	Score    float64          `json:"score"`
	Why      string           `json:"why"`
}

// Hit is a single ranked search result.
type Hit struct {
	Grain      string      `json:"grain"` // "symbol" | "file"
	Name       string      `json:"name"`
	SymbolKind string      `json:"symbol_kind,omitempty"`
	Container  string      `json:"container,omitempty"`
	Signature  string      `json:"signature,omitempty"`
	Path       string      `json:"path,omitempty"`
	Package    string      `json:"package,omitempty"`
	Role       string      `json:"role,omitempty"`
	Exported   bool        `json:"exported,omitempty"`
	StartLine  int         `json:"start_line,omitempty"`
	EndLine    int         `json:"end_line,omitempty"`
	Snippet    string      `json:"snippet,omitempty"`
	Score      float64     `json:"score"`
	Why        string      `json:"why"`
	RiskLevel  string      `json:"risk_level"`
	Graph      GraphInfo   `json:"graph,omitempty"`
	Memories   []MemoryRef `json:"memories,omitempty"`
}

// SearchResult is the typed result of a search.
type SearchResult struct {
	Query    string         `json:"query"`
	Kind     string         `json:"kind"`
	Total    int            `json:"total"`
	Hits     []Hit          `json:"hits"`
	Memories []MemoryDetail `json:"memories,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (SearchResult) CommandName() string { return "search" }

func (r SearchResult) RenderText(w io.Writer, _ render.Options) error {
	if len(r.Hits) == 0 && len(r.Memories) == 0 {
		fmt.Fprintf(w, "no results for %q\n", r.Query)
		return nil
	}
	for i, h := range r.Hits {
		header := h.Name
		if h.Container != "" {
			header = h.Container + "." + h.Name
		}
		loc := h.Path
		if h.StartLine > 0 {
			loc = fmt.Sprintf("%s:%d", h.Path, h.StartLine)
		}
		fmt.Fprintf(w, "%d. %s  [%s]  (score %.2f, %s)\n", i+1, header, kindLabel(h), h.Score, h.Why)
		if loc != "" {
			fmt.Fprintf(w, "   %s\n", loc)
		}
		if h.Signature != "" {
			fmt.Fprintf(w, "   %s\n", h.Signature)
		}
		renderGraphText(w, h.Graph)
		for _, m := range h.Memories {
			fmt.Fprintf(w, "   memory %s [%s]: %s\n", m.ID, m.Kind, m.Title)
		}
	}
	if len(r.Memories) > 0 {
		fmt.Fprintf(w, "\nmemories:\n")
		for _, m := range r.Memories {
			fmt.Fprintf(w, "%s [%s] %s  (score %.2f, %s)\n", m.ID, m.Kind, m.Title, m.Score, m.Why)
			if m.Body != "" {
				fmt.Fprintln(w, indentMemory(m.Body))
			}
			if len(m.Tags) > 0 {
				fmt.Fprintf(w, "   tags: %s\n", strings.Join(m.Tags, ", "))
			}
			for _, l := range m.Links {
				fmt.Fprintf(w, "   link: %s:%s\n", l.TargetType, l.TargetRef)
			}
			for _, e := range m.Evidence {
				fmt.Fprintf(w, "   evidence: %s:%d-%d\n", e.Path, e.LineStart, e.LineEnd)
			}
		}
	}
	return nil
}

func indentMemory(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "   | " + l
	}
	return strings.Join(lines, "\n")
}

func (r SearchResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Search: %s\n\n%d result(s).\n\n", r.Query, r.Total)
	for _, h := range r.Hits {
		header := h.Name
		if h.Container != "" {
			header = h.Container + "." + h.Name
		}
		fmt.Fprintf(w, "## %s (%s)\n\n", header, kindLabel(h))
		if h.Path != "" {
			loc := h.Path
			if h.StartLine > 0 {
				loc = fmt.Sprintf("%s:%d-%d", h.Path, h.StartLine, h.EndLine)
			}
			fmt.Fprintf(w, "- location: `%s`\n", loc)
		}
		if h.Signature != "" {
			fmt.Fprintf(w, "- signature: `%s`\n", h.Signature)
		}
		fmt.Fprintf(w, "- score: %.2f (%s); risk: %s\n", h.Score, h.Why, h.RiskLevel)
		if len(h.Graph.ImportedBy) > 0 {
			fmt.Fprintf(w, "- imported by: %s\n", strings.Join(h.Graph.ImportedBy, ", "))
		}
		if len(h.Graph.Tests) > 0 {
			fmt.Fprintf(w, "- tests: %s\n", strings.Join(h.Graph.Tests, ", "))
		}
		if h.Snippet != "" {
			fmt.Fprintf(w, "\n```\n%s\n```\n", h.Snippet)
		}
		fmt.Fprintln(w)
	}
	if len(r.Memories) > 0 {
		fmt.Fprintf(w, "# Memories\n\n")
		for _, m := range r.Memories {
			fmt.Fprintf(w, "## %s (%s) — %s\n\n", m.ID, m.Kind, m.Title)
			fmt.Fprintf(w, "- score: %.2f (%s)\n", m.Score, m.Why)
			if len(m.Tags) > 0 {
				fmt.Fprintf(w, "- tags: %s\n", strings.Join(m.Tags, ", "))
			}
			for _, l := range m.Links {
				fmt.Fprintf(w, "- link: `%s:%s`\n", l.TargetType, l.TargetRef)
			}
			for _, e := range m.Evidence {
				fmt.Fprintf(w, "- evidence: `%s:%d-%d`\n", e.Path, e.LineStart, e.LineEnd)
			}
			if m.Body != "" {
				fmt.Fprintf(w, "\n%s\n", m.Body)
			}
			fmt.Fprintln(w)
		}
	}
	return nil
}

func renderGraphText(w io.Writer, g GraphInfo) {
	if len(g.ImportedBy) > 0 {
		fmt.Fprintf(w, "   imported by: %s\n", strings.Join(g.ImportedBy, ", "))
	}
	if len(g.Tests) > 0 {
		fmt.Fprintf(w, "   tests: %s\n", strings.Join(g.Tests, ", "))
	}
}

func kindLabel(h Hit) string {
	if h.Grain == "symbol" && h.SymbolKind != "" {
		return h.SymbolKind
	}
	return h.Grain
}

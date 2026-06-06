package search

import (
	"fmt"
	"io"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/render"
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

// Hit is a single ranked search result.
type Hit struct {
	Grain      string      `json:"grain"` // "symbol" | "file" | "memory" | "epic" | "task"
	ID         string      `json:"id,omitempty"`
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
	Query    string   `json:"query"`
	Kind     string   `json:"kind"`
	Total    int      `json:"total"`
	Hits     []Hit    `json:"hits"`
	Warnings []string `json:"warnings,omitempty"`
}

func (SearchResult) CommandName() string { return "search" }

func (r SearchResult) RenderText(w io.Writer, _ render.Options) error {
	if len(r.Hits) == 0 {
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
	return nil
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

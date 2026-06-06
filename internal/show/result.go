package show

import (
	"fmt"
	"io"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/render"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// MemoryRef is a linked memory summary.
type MemoryRef struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

func memID(id int64) string { return fmt.Sprintf("mem_%03d", id) }

// ---- show symbol ----

// SymbolBlock is one matching definition.
type SymbolBlock struct {
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Container string        `json:"container,omitempty"`
	Signature string        `json:"signature,omitempty"`
	Path      string        `json:"path"`
	Package   string        `json:"package,omitempty"`
	Role      string        `json:"role,omitempty"`
	Exported  bool          `json:"exported"`
	StartLine int           `json:"start_line,omitempty"`
	EndLine   int           `json:"end_line,omitempty"`
	Snippet   string        `json:"snippet,omitempty"`
	Tests     []string      `json:"tests,omitempty"`
	Memories  []MemoryRef   `json:"memories,omitempty"`
	Work      []WorkRefBack `json:"work,omitempty"`
}

// SymbolResult is the typed result of `show symbol`.
type SymbolResult struct {
	Query  string        `json:"query"`
	In     string        `json:"in,omitempty"`
	Total  int           `json:"total"`
	Capped bool          `json:"capped"`
	Blocks []SymbolBlock `json:"blocks"`
}

func (SymbolResult) CommandName() string { return "show" }

func (r SymbolResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%d definition(s) of %q", r.Total, r.Query)
	if r.In != "" {
		fmt.Fprintf(w, " in %q", r.In)
	}
	fmt.Fprintln(w)
	for i, b := range r.Blocks {
		header := b.Name
		if b.Container != "" {
			header = b.Container + "." + b.Name
		}
		loc := b.Path
		if b.StartLine > 0 {
			loc = fmt.Sprintf("%s:%d-%d", b.Path, b.StartLine, b.EndLine)
		}
		fmt.Fprintf(w, "\n%d. %s [%s]  %s\n", i+1, header, b.Kind, loc)
		if b.Snippet != "" {
			fmt.Fprintln(w, indent(b.Snippet))
		}
		if len(b.Tests) > 0 {
			fmt.Fprintf(w, "   tests: %s\n", strings.Join(b.Tests, ", "))
		}
		for _, m := range b.Memories {
			fmt.Fprintf(w, "   memory %s [%s]: %s\n", m.ID, m.Kind, m.Title)
		}
		for _, k := range b.Work {
			fmt.Fprintf(w, "   %s %s [%s]: %s\n", k.Kind, k.ID, k.Status, k.Title)
		}
	}
	if r.Capped {
		fmt.Fprintf(w, "\n(showing first %d of %d; narrow with --in <path>)\n", len(r.Blocks), r.Total)
	}
	return nil
}

func (r SymbolResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# Symbol: %s\n\n%d definition(s).\n", r.Query, r.Total)
	for _, b := range r.Blocks {
		header := b.Name
		if b.Container != "" {
			header = b.Container + "." + b.Name
		}
		fmt.Fprintf(w, "\n## %s (%s)\n\n- `%s:%d-%d`\n", header, b.Kind, b.Path, b.StartLine, b.EndLine)
		if b.Snippet != "" {
			fmt.Fprintf(w, "\n```\n%s\n```\n", b.Snippet)
		}
	}
	return nil
}

// ---- show file ----

// OutlineEntry is one symbol in a file outline.
type OutlineEntry struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Container string `json:"container,omitempty"`
	Signature string `json:"signature,omitempty"`
	Exported  bool   `json:"exported"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

// FileResult is the typed result of `show file`.
type FileResult struct {
	Path       string         `json:"path"`
	Language   string         `json:"language,omitempty"`
	Package    string         `json:"package,omitempty"`
	Role       string         `json:"role,omitempty"`
	Outline    []OutlineEntry `json:"outline"`
	Imports    []string       `json:"imports,omitempty"`
	ImportedBy []string       `json:"imported_by,omitempty"`
	Tests      []string       `json:"tests,omitempty"`
	Memories   []MemoryRef    `json:"memories,omitempty"`
	Work       []WorkRefBack  `json:"work,omitempty"`
}

func (FileResult) CommandName() string { return "show" }

func (r FileResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s  [%s, package %s, %s]\n", r.Path, r.Language, r.Package, r.Role)
	fmt.Fprintf(w, "\noutline (%d symbols):\n", len(r.Outline))
	for _, e := range r.Outline {
		name := e.Name
		if e.Container != "" {
			name = e.Container + "." + e.Name
		}
		fmt.Fprintf(w, "  %4d  %-9s %s\n", e.StartLine, e.Kind, name)
	}
	renderList(w, "imports", r.Imports)
	renderList(w, "imported by", r.ImportedBy)
	renderList(w, "tests", r.Tests)
	for _, m := range r.Memories {
		fmt.Fprintf(w, "memory %s [%s]: %s\n", m.ID, m.Kind, m.Title)
	}
	renderWork(w, r.Work)
	return nil
}

func (r FileResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# File: %s\n\n- language: %s\n- package: %s\n- role: %s\n\n## Outline\n\n", r.Path, r.Language, r.Package, r.Role)
	for _, e := range r.Outline {
		name := e.Name
		if e.Container != "" {
			name = e.Container + "." + e.Name
		}
		fmt.Fprintf(w, "- `%d` %s **%s**\n", e.StartLine, e.Kind, name)
	}
	return nil
}

// ---- show memory ----

// MemoryEvidence mirrors a stored evidence anchor.
type MemoryEvidence struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

// MemoryLink mirrors a stored link.
type MemoryLink struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
}

// MemoryResult is the typed result of `show memory`.
type MemoryResult struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	Title     string           `json:"title"`
	Body      string           `json:"body,omitempty"`
	Tags      []string         `json:"tags,omitempty"`
	Evidence  []MemoryEvidence `json:"evidence,omitempty"`
	Links     []MemoryLink     `json:"links,omitempty"`
	Work      []WorkRefBack    `json:"work,omitempty"`
	CreatedAt string           `json:"created_at,omitempty"`
	UpdatedAt string           `json:"updated_at,omitempty"`
}

func (MemoryResult) CommandName() string { return "show" }

func (r MemoryResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "%s [%s] %s\n", r.ID, r.Kind, r.Title)
	if r.Body != "" {
		fmt.Fprintf(w, "\n%s\n", r.Body)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "\ntags: %s\n", strings.Join(r.Tags, ", "))
	}
	for _, e := range r.Evidence {
		fmt.Fprintf(w, "evidence: %s:%d-%d\n", e.Path, e.LineStart, e.LineEnd)
	}
	for _, l := range r.Links {
		fmt.Fprintf(w, "link: %s:%s\n", l.TargetType, l.TargetRef)
	}
	renderWork(w, r.Work)
	return nil
}

func (r MemoryResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# %s (%s)\n\n%s\n\n%s\n", r.ID, r.Kind, r.Title, r.Body)
	return nil
}

func memoryResultFrom(m store.Memory) MemoryResult {
	out := MemoryResult{
		ID: memID(m.ID), Kind: m.Kind, Title: m.Title, Body: m.Body,
		Tags: m.Tags, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	for _, e := range m.Evidence {
		out.Evidence = append(out.Evidence, MemoryEvidence{Path: e.Path, LineStart: e.LineStart, LineEnd: e.LineEnd})
	}
	for _, l := range m.Links {
		out.Links = append(out.Links, MemoryLink{TargetType: l.TargetType, TargetRef: l.TargetRef})
	}
	return out
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "   | " + l
	}
	return strings.Join(lines, "\n")
}

func renderList(w io.Writer, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "%s: %s\n", label, strings.Join(items, ", "))
}

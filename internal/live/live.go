// Package live resolves exact line ranges and snippets by re-parsing the
// working tree with tree-sitter at query time. Stored line numbers are never
// trusted as truth (a core invariant); this package reconstructs them live.
package live

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/extract"
)

// MaxSnippetLines bounds snippet size so output stays manageable.
const MaxSnippetLines = 60

// Cache parses working-tree files on demand (once each).
type Cache struct {
	workDir string
	reg     *extract.Registry
	syms    map[string][]extract.Symbol
	lines   map[string][]string
}

// New returns a cache rooted at workDir using the given grammar registry.
func New(workDir string, reg *extract.Registry) *Cache {
	return &Cache{
		workDir: workDir,
		reg:     reg,
		syms:    map[string][]extract.Symbol{},
		lines:   map[string][]string{},
	}
}

func (c *Cache) read(rel string) ([]byte, bool) {
	b, err := os.ReadFile(filepath.Join(c.workDir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, false
	}
	return b, true
}

// Symbols returns the live symbols for a file (nil if unreadable or no grammar).
func (c *Cache) Symbols(rel string) []extract.Symbol {
	if v, ok := c.syms[rel]; ok {
		return v
	}
	var out []extract.Symbol
	if ex, ok := c.reg.ForPath(rel); ok {
		if content, ok := c.read(rel); ok {
			if res, err := ex.Extract(content); err == nil {
				out = res.Symbols
			}
		}
	}
	c.syms[rel] = out
	return out
}

// SourceLines returns the file split into lines (nil if unreadable).
func (c *Cache) SourceLines(rel string) []string {
	if v, ok := c.lines[rel]; ok {
		return v
	}
	var out []string
	if content, ok := c.read(rel); ok {
		out = strings.Split(string(content), "\n")
	}
	c.lines[rel] = out
	return out
}

// FindSymbol locates the symbol matching (name, container), preferring an exact
// kind match, in a freshly parsed file.
func (c *Cache) FindSymbol(rel, name, container, kind string) (extract.Symbol, bool) {
	var fallback extract.Symbol
	haveFallback := false
	for _, s := range c.Symbols(rel) {
		if s.Name != name || s.Container != container {
			continue
		}
		if string(s.Kind) == kind {
			return s, true
		}
		if !haveFallback {
			fallback, haveFallback = s, true
		}
	}
	return fallback, haveFallback
}

// Enclosing returns the innermost symbol whose live range contains line.
func Enclosing(syms []extract.Symbol, line int) (extract.Symbol, bool) {
	best := extract.Symbol{}
	found := false
	for _, s := range syms {
		if s.StartLine <= line && line <= s.EndLine {
			if !found || (s.EndLine-s.StartLine) < (best.EndLine-best.StartLine) {
				best, found = s, true
			}
		}
	}
	return best, found
}

// Snippet extracts lines [start-ctx, end+ctx] (1-based, clamped), capped.
func Snippet(lines []string, start, end, ctx int) string {
	if start <= 0 || len(lines) == 0 {
		return ""
	}
	from := start - ctx
	if from < 1 {
		from = 1
	}
	to := end + ctx
	if to > len(lines) {
		to = len(lines)
	}
	if to-from+1 > MaxSnippetLines {
		to = from + MaxSnippetLines - 1
	}
	return strings.Join(lines[from-1:to], "\n")
}

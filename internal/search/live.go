package search

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/extract"
)

// fileCache parses working-tree files on demand (once each) so live line-range
// and snippet resolution re-reads current content rather than trusting stored
// line numbers.
type fileCache struct {
	workDir string
	reg     *extract.Registry
	syms    map[string][]extract.Symbol
	lines   map[string][]string
}

func newFileCache(workDir string, reg *extract.Registry) *fileCache {
	return &fileCache{
		workDir: workDir,
		reg:     reg,
		syms:    map[string][]extract.Symbol{},
		lines:   map[string][]string{},
	}
}

func (c *fileCache) read(rel string) ([]byte, bool) {
	b, err := os.ReadFile(filepath.Join(c.workDir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, false
	}
	return b, true
}

func (c *fileCache) symbols(rel string) []extract.Symbol {
	if v, ok := c.syms[rel]; ok {
		return v
	}
	var out []extract.Symbol
	ex, ok := c.reg.ForPath(rel)
	if ok {
		if content, ok := c.read(rel); ok {
			if res, err := ex.Extract(content); err == nil {
				out = res.Symbols
			}
		}
	}
	c.syms[rel] = out
	return out
}

func (c *fileCache) sourceLines(rel string) []string {
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

// findSymbol locates the symbol matching (name, container) — preferring an
// exact kind match — in a freshly parsed file.
func (c *fileCache) findSymbol(rel, name, container, kind string) (extract.Symbol, bool) {
	var fallback extract.Symbol
	haveFallback := false
	for _, s := range c.symbols(rel) {
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

// enclosing returns the innermost symbol whose live range contains line.
func enclosing(syms []extract.Symbol, line int) (extract.Symbol, bool) {
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

// maxSnippetLines bounds snippet size so search output stays manageable.
const maxSnippetLines = 40

// snippet extracts lines [start-ctx, end+ctx] (1-based, clamped), capped.
func snippet(lines []string, start, end, ctx int) string {
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
	if to-from+1 > maxSnippetLines {
		to = from + maxSnippetLines - 1
	}
	return strings.Join(lines[from-1:to], "\n")
}

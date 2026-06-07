package live

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orafaelfragoso/columbus/internal/extract"
)

func lines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "L" + string(rune('a'+i%26))
	}
	return out
}

func TestSnippet(t *testing.T) {
	src := []string{"one", "two", "three", "four", "five"}
	cases := []struct {
		name            string
		lines           []string
		start, end, ctx int
		want            string
	}{
		{"no context", src, 2, 2, 0, "two"},
		{"with context", src, 2, 3, 1, "one\ntwo\nthree\nfour"},
		{"clamps low edge to first line", src, 1, 1, 3, "one\ntwo\nthree\nfour"},
		{"clamps high edge to last line", src, 5, 5, 3, "two\nthree\nfour\nfive"},
		{"range spanning multiple lines", src, 2, 4, 0, "two\nthree\nfour"},
		{"zero start returns empty", src, 0, 0, 1, ""},
		{"negative start returns empty", src, -1, 2, 1, ""},
		{"empty input returns empty", nil, 1, 1, 1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Snippet(c.lines, c.start, c.end, c.ctx); got != c.want {
				t.Fatalf("Snippet = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSnippetCapsAtMaxSnippetLines(t *testing.T) {
	src := lines(500)
	got := Snippet(src, 1, 400, 10)
	n := strings.Count(got, "\n") + 1
	if n != MaxSnippetLines {
		t.Fatalf("snippet has %d lines, want cap of %d", n, MaxSnippetLines)
	}
}

func TestEnclosing(t *testing.T) {
	outer := extract.Symbol{Name: "Outer", Kind: extract.KindClass, StartLine: 1, EndLine: 20}
	inner := extract.Symbol{Name: "inner", Kind: extract.KindMethod, StartLine: 5, EndLine: 10}

	t.Run("smallest enclosing range wins", func(t *testing.T) {
		got, ok := Enclosing([]extract.Symbol{outer, inner}, 7)
		if !ok || got.Name != "inner" {
			t.Fatalf("Enclosing(7) = %+v ok=%v, want inner", got, ok)
		}
	})

	t.Run("falls back to the only enclosing symbol", func(t *testing.T) {
		got, ok := Enclosing([]extract.Symbol{outer, inner}, 2)
		if !ok || got.Name != "Outer" {
			t.Fatalf("Enclosing(2) = %+v ok=%v, want Outer", got, ok)
		}
	})

	t.Run("boundaries are inclusive", func(t *testing.T) {
		if _, ok := Enclosing([]extract.Symbol{inner}, 5); !ok {
			t.Fatal("start line should be inclusive")
		}
		if _, ok := Enclosing([]extract.Symbol{inner}, 10); !ok {
			t.Fatal("end line should be inclusive")
		}
	})

	t.Run("no enclosing symbol", func(t *testing.T) {
		if _, ok := Enclosing([]extract.Symbol{inner}, 99); ok {
			t.Fatal("expected no match outside any range")
		}
		if _, ok := Enclosing(nil, 1); ok {
			t.Fatal("expected no match for empty symbols")
		}
	})
}

func newCache(t *testing.T, files map[string]string) *Cache {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := extract.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return New(dir, reg)
}

const goSrc = `package p

type Greeter struct{}

func (Greeter) Hello() string { return "hi" }

func Standalone() {}
`

func TestSourceLines(t *testing.T) {
	c := newCache(t, map[string]string{"a.go": "x\ny\nz"})
	got := c.SourceLines("a.go")
	if len(got) != 3 || got[0] != "x" || got[2] != "z" {
		t.Fatalf("SourceLines = %v", got)
	}
	if c.SourceLines("missing.go") != nil {
		t.Fatal("unreadable file should yield nil lines")
	}
}

func TestSymbols(t *testing.T) {
	c := newCache(t, map[string]string{"g.go": goSrc})
	syms := c.Symbols("g.go")
	if len(syms) == 0 {
		t.Fatal("expected symbols from a parseable Go file")
	}
	if c.Symbols("none.txt") != nil {
		t.Fatal("file without a grammar should yield nil")
	}
}

func TestFindSymbol(t *testing.T) {
	c := newCache(t, map[string]string{"g.go": goSrc})

	t.Run("locates a method by name and container", func(t *testing.T) {
		s, ok := c.FindSymbol("g.go", "Hello", "Greeter", string(extract.KindMethod))
		if !ok || s.Name != "Hello" {
			t.Fatalf("FindSymbol = %+v ok=%v", s, ok)
		}
	})

	t.Run("returns false when the name is absent", func(t *testing.T) {
		if _, ok := c.FindSymbol("g.go", "Nope", "", string(extract.KindFunction)); ok {
			t.Fatal("expected no match for unknown symbol")
		}
	})

	t.Run("falls back to a name/container match when kind differs", func(t *testing.T) {
		s, ok := c.FindSymbol("g.go", "Hello", "Greeter", string(extract.KindFunction))
		if !ok || s.Name != "Hello" {
			t.Fatalf("expected fallback to the Hello method, got %+v ok=%v", s, ok)
		}
	})
}

func TestCacheReusesParsedResults(t *testing.T) {
	c := newCache(t, map[string]string{"g.go": goSrc, "a.go": "x\ny"})
	if got, again := len(c.Symbols("g.go")), len(c.Symbols("g.go")); got != again {
		t.Fatalf("cached Symbols diverged: %d vs %d", got, again)
	}
	if got, again := len(c.SourceLines("a.go")), len(c.SourceLines("a.go")); got != again {
		t.Fatalf("cached SourceLines diverged: %d vs %d", got, again)
	}
}

package grep

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoGrepFindsTokensInAllowedFiles(t *testing.T) {
	work := t.TempDir()
	os.WriteFile(filepath.Join(work, "a.go"), []byte("package a\nfunc Foo() {}\n// note foo here\n"), 0o644)
	os.WriteFile(filepath.Join(work, "b.go"), []byte("package b\nfunc Bar() {}\n"), 0o644)
	os.WriteFile(filepath.Join(work, "ignored.go"), []byte("foo foo foo\n"), 0o644)

	allow := map[string]bool{"a.go": true, "b.go": true}
	hits, err := goGrep{}.Search(work, []string{"foo"}, allow, 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2 (case-insensitive Foo + foo in a.go), got %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.Path != "a.go" {
			t.Errorf("unexpected path %q (ignored.go must be excluded)", h.Path)
		}
	}
}

func TestGoGrepRespectsCap(t *testing.T) {
	work := t.TempDir()
	os.WriteFile(filepath.Join(work, "a.go"), []byte("x\nx\nx\nx\n"), 0o644)
	hits, _ := goGrep{}.Search(work, []string{"x"}, map[string]bool{"a.go": true}, 2)
	if len(hits) != 2 {
		t.Errorf("cap not respected: %d", len(hits))
	}
}

func TestNewReturnsASearcher(t *testing.T) {
	if New() == nil {
		t.Fatal("New returned nil")
	}
}

func TestParseRipgrepFiltersToAllow(t *testing.T) {
	out := []byte(`{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"func Foo()\n"},"line_number":2}}
{"type":"match","data":{"path":{"text":"vendor/x.go"},"lines":{"text":"Foo\n"},"line_number":5}}
{"type":"begin","data":{}}`)
	hits := parseRipgrep(out, map[string]bool{"a.go": true}, 100)
	if len(hits) != 1 || hits[0].Path != "a.go" || hits[0].Line != 2 {
		t.Errorf("parse = %+v, want one a.go:2 hit", hits)
	}
}

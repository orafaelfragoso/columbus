package index

import "testing"

func TestResolveRelativeImports(t *testing.T) {
	files := []FileMeta{
		{ID: 1, Path: "src/app.ts", Role: "impl"},
		{ID: 2, Path: "src/util.ts", Role: "impl"},
		{ID: 3, Path: "src/lib/index.ts", Role: "impl"},
	}
	imports := []ImportRow{
		{FileID: 1, Specifier: "./util"},    // -> src/util.ts
		{FileID: 1, Specifier: "./lib"},     // -> src/lib/index.ts
		{FileID: 1, Specifier: "react"},     // bare -> skipped
		{FileID: 1, Specifier: "./missing"}, // unresolved -> skipped
	}
	edges, _ := resolveGraph(files, imports)
	got := map[int64]bool{}
	for _, e := range edges {
		if e.From != 1 {
			t.Errorf("unexpected edge from %d", e.From)
		}
		got[e.To] = true
	}
	if !got[2] || !got[3] {
		t.Errorf("expected edges to util.ts(2) and lib/index.ts(3), got %v", got)
	}
	if len(edges) != 2 {
		t.Errorf("edge count = %d, want 2", len(edges))
	}
}

func TestResolveTestLinks(t *testing.T) {
	files := []FileMeta{
		{ID: 1, Path: "svc.go", Role: "impl"},
		{ID: 2, Path: "svc_test.go", Role: "test"},
		{ID: 3, Path: "src/app.ts", Role: "impl"},
		{ID: 4, Path: "src/app.test.ts", Role: "test"},
		{ID: 5, Path: "pkg/mod.py", Role: "impl"},
		{ID: 6, Path: "pkg/test_mod.py", Role: "test"},
	}
	_, links := resolveGraph(files, nil)
	pairs := map[[2]int64]bool{}
	for _, l := range links {
		pairs[[2]int64{l.Impl, l.Test}] = true
	}
	for _, want := range [][2]int64{{1, 2}, {3, 4}, {5, 6}} {
		if !pairs[want] {
			t.Errorf("missing test link %v in %v", want, pairs)
		}
	}
}

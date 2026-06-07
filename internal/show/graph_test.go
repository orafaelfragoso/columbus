package show

import (
	"testing"

	"github.com/orafaelfragoso/columbus/internal/store"
)

// seedGraph writes three files (impl a + b, test a_test) with a resolved import
// edge a->b, a test link a->a_test, and three unresolved imports on a covering
// an npm package, a relative miss and a Go-module specifier.
func seedGraph(t *testing.T, s *Shower) {
	t.Helper()
	err := s.DB.WithTx(func(tx *store.Tx) error {
		if _, e := tx.PutFile(
			store.FileRecord{Path: "a.go", Language: "go", Package: "a", Role: "impl", BlobOID: "o1"},
			nil, []string{"react", "./missing", "github.com/x/y/z"}, nil, nil,
		); e != nil {
			return e
		}
		if _, e := tx.PutFile(
			store.FileRecord{Path: "b.go", Language: "go", Package: "b", Role: "impl", BlobOID: "o2"},
			nil, nil, nil, nil,
		); e != nil {
			return e
		}
		if _, e := tx.PutFile(
			store.FileRecord{Path: "a_test.go", Language: "go", Package: "a", Role: "test", BlobOID: "o3"},
			nil, nil, nil, nil,
		); e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed files: %v", err)
	}
	a, _, _ := s.DB.FileByPath("a.go")
	b, _, _ := s.DB.FileByPath("b.go")
	at, _, _ := s.DB.FileByPath("a_test.go")
	if err := s.DB.WithTx(func(tx *store.Tx) error {
		return tx.ReplaceGraph([][2]int64{{a.ID, b.ID}}, [][2]int64{{a.ID, at.ID}})
	}); err != nil {
		t.Fatalf("ReplaceGraph: %v", err)
	}
}

func nodeByID(nodes []GraphNode, id string) (GraphNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return GraphNode{}, false
}

func countEdges(edges []GraphEdge, typ string) int {
	n := 0
	for _, e := range edges {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func TestGraphProjectsNodesEdgesAndExternals(t *testing.T) {
	s := buildShower(t, nil)
	seedGraph(t, s)

	g, err := s.Graph(GraphOptions{})
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if g.Total != 3 {
		t.Fatalf("total files = %d, want 3", g.Total)
	}
	// External package nodes: react and github.com/x/y; the relative miss is dropped.
	if _, ok := nodeByID(g.Nodes, "ext:react"); !ok {
		t.Fatalf("missing ext:react in %+v", g.Nodes)
	}
	if _, ok := nodeByID(g.Nodes, "ext:github.com/x/y"); !ok {
		t.Fatalf("missing collapsed Go module node in %+v", g.Nodes)
	}
	if _, ok := nodeByID(g.Nodes, "ext:./missing"); ok {
		t.Fatal("relative miss must not become an external node")
	}

	if got := countEdges(g.Edges, "import"); got != 1 {
		t.Fatalf("import edges = %d, want 1", got)
	}
	if got := countEdges(g.Edges, "test"); got != 1 {
		t.Fatalf("test edges = %d, want 1", got)
	}
	if got := countEdges(g.Edges, "external"); got != 2 {
		t.Fatalf("external edges = %d, want 2", got)
	}

	a, _ := nodeByID(g.Nodes, "a.go")
	if a.OutDegree != 3 || a.InDegree != 0 || !a.HasTests {
		t.Fatalf("a.go node = %+v (want out 3, in 0, has_tests)", a)
	}
	b, _ := nodeByID(g.Nodes, "b.go")
	if b.InDegree != 1 {
		t.Fatalf("b.go in_degree = %d, want 1", b.InDegree)
	}

	// Nodes sorted by id; edges sorted by (from,to,type).
	for i := 1; i < len(g.Nodes); i++ {
		if g.Nodes[i-1].ID > g.Nodes[i].ID {
			t.Fatalf("nodes not sorted by id: %s > %s", g.Nodes[i-1].ID, g.Nodes[i].ID)
		}
	}
}

func TestGraphRoleFilterInducesCleanSubgraph(t *testing.T) {
	s := buildShower(t, nil)
	seedGraph(t, s)

	g, err := s.Graph(GraphOptions{Role: "impl"})
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	// a_test.go is filtered out, so the test edge dangles and is dropped.
	if _, ok := nodeByID(g.Nodes, "a_test.go"); ok {
		t.Fatal("test file should be filtered out by role=impl")
	}
	if got := countEdges(g.Edges, "test"); got != 0 {
		t.Fatalf("dangling test edge survived filter: %d", got)
	}
	if got := countEdges(g.Edges, "import"); got != 1 {
		t.Fatalf("import edge a->b should survive: %d", got)
	}
}

func TestGraphCapByInDegree(t *testing.T) {
	s := buildShower(t, nil)
	seedGraph(t, s)

	g, err := s.Graph(GraphOptions{Max: 1})
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if !g.Capped || g.Total != 3 {
		t.Fatalf("capped=%v total=%d, want capped/3", g.Capped, g.Total)
	}
	files := 0
	for _, n := range g.Nodes {
		if n.Kind == "file" {
			files++
		}
	}
	if files != 1 {
		t.Fatalf("capped file nodes = %d, want 1", files)
	}
	// b.go has the highest in-degree (1) so it survives the cap.
	if _, ok := nodeByID(g.Nodes, "b.go"); !ok {
		t.Fatalf("highest in-degree node should survive cap: %+v", g.Nodes)
	}
}

func TestExternalPackageCollapse(t *testing.T) {
	cases := map[string]string{
		"react":              "react",
		"react/jsx-runtime":  "react",
		"@scope/pkg/sub":     "@scope/pkg",
		"os.path":            "os",
		"github.com/x/y/z/w": "github.com/x/y",
		"lodash":             "lodash",
	}
	for spec, want := range cases {
		got, ok := externalPackage(spec)
		if !ok || got != want {
			t.Errorf("externalPackage(%q) = %q,%v; want %q", spec, got, ok, want)
		}
	}
	for _, miss := range []string{"./rel", "../up", "/abs"} {
		if _, ok := externalPackage(miss); ok {
			t.Errorf("externalPackage(%q) should be dropped", miss)
		}
	}
}

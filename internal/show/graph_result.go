package show

import (
	"fmt"
	"io"
	"sort"

	"github.com/rafaelfragoso/columbus/internal/render"
)

// GraphNode is one node in the projected graph. File nodes carry classification
// and degree; external package nodes carry only id + kind.
type GraphNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // "file" | "external"
	Role      string `json:"role,omitempty"`
	Language  string `json:"language,omitempty"`
	Package   string `json:"package,omitempty"`
	InDegree  int    `json:"in_degree,omitempty"`
	OutDegree int    `json:"out_degree,omitempty"`
	HasTests  bool   `json:"has_tests,omitempty"`
}

// GraphEdge is one directed edge: import (importer->imported), test (impl->test)
// or external (file->ext:<pkg>).
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// GraphFreshness reports how current the projected (cached) graph is.
type GraphFreshness struct {
	IndexedHead   string `json:"indexed_head,omitempty"`
	Dirty         bool   `json:"dirty"`
	LastIndexedAt string `json:"last_indexed_at,omitempty"`
	Stale         bool   `json:"stale"`
}

// GraphResult is the typed result of `show graph`. The --json projection carries
// the full node/edge arrays (the canonical {nodes, edges} contract); text/llm
// are summaries.
type GraphResult struct {
	In        string         `json:"in,omitempty"`
	Role      string         `json:"role,omitempty"`
	Lang      string         `json:"lang,omitempty"`
	Total     int            `json:"total"`
	Capped    bool           `json:"capped"`
	Freshness GraphFreshness `json:"freshness"`
	Nodes     []GraphNode    `json:"nodes"`
	Edges     []GraphEdge    `json:"edges"`
}

func (GraphResult) CommandName() string { return "show" }

const graphTopN = 10

func (r GraphResult) RenderText(w io.Writer, _ render.Options) error {
	files, externals := r.nodeCounts()
	imports, tests, externalEdges := r.edgeCounts()
	fmt.Fprintf(w, "graph: %d files, %d external packages\n", files, externals)
	fmt.Fprintf(w, "edges: %d import, %d test, %d external\n", imports, tests, externalEdges)
	if r.Capped {
		fmt.Fprintf(w, "(showing %d of %d files; raise --max to see more)\n", files, r.Total)
	}

	if hubs := r.topHubs(graphTopN); len(hubs) > 0 {
		fmt.Fprintf(w, "\ntop hubs (most imported):\n")
		for _, h := range hubs {
			fmt.Fprintf(w, "  %3d  %s\n", h.InDegree, h.ID)
		}
	}
	if deps := r.topExternals(graphTopN); len(deps) > 0 {
		fmt.Fprintf(w, "\ntop external deps:\n")
		for _, d := range deps {
			fmt.Fprintf(w, "  %3d  %s\n", d.count, d.id)
		}
	}
	if missing := r.filesWithoutTests(); len(missing) > 0 {
		fmt.Fprintf(w, "\n%d file(s) without tests\n", len(missing))
	}
	fmt.Fprintf(w, "\n%s\n", r.freshnessLine())
	return nil
}

func (r GraphResult) RenderLLM(w io.Writer, _ render.Options) error {
	files, externals := r.nodeCounts()
	imports, tests, externalEdges := r.edgeCounts()
	fmt.Fprintf(w, "# Dependency graph\n\n")
	fmt.Fprintf(w, "- files: %d\n- external packages: %d\n", files, externals)
	fmt.Fprintf(w, "- edges: %d import, %d test, %d external\n", imports, tests, externalEdges)
	fmt.Fprintf(w, "- %s\n", r.freshnessLine())
	if r.Capped {
		fmt.Fprintf(w, "- capped: showing %d of %d files\n", files, r.Total)
	}
	if hubs := r.topHubs(graphTopN); len(hubs) > 0 {
		fmt.Fprintf(w, "\n## Top hubs\n\n")
		for _, h := range hubs {
			fmt.Fprintf(w, "- `%s` (imported by %d)\n", h.ID, h.InDegree)
		}
	}
	if deps := r.topExternals(graphTopN); len(deps) > 0 {
		fmt.Fprintf(w, "\n## Top external deps\n\n")
		for _, d := range deps {
			fmt.Fprintf(w, "- `%s` (%d)\n", d.id, d.count)
		}
	}
	return nil
}

func (r GraphResult) nodeCounts() (files, externals int) {
	for _, n := range r.Nodes {
		if n.Kind == "external" {
			externals++
		} else {
			files++
		}
	}
	return files, externals
}

func (r GraphResult) edgeCounts() (imports, tests, external int) {
	for _, e := range r.Edges {
		switch e.Type {
		case "import":
			imports++
		case "test":
			tests++
		case "external":
			external++
		}
	}
	return imports, tests, external
}

func (r GraphResult) topHubs(n int) []GraphNode {
	var files []GraphNode
	for _, node := range r.Nodes {
		if node.Kind == "file" && node.InDegree > 0 {
			files = append(files, node)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].InDegree != files[j].InDegree {
			return files[i].InDegree > files[j].InDegree
		}
		return files[i].ID < files[j].ID
	})
	if len(files) > n {
		files = files[:n]
	}
	return files
}

type externalCount struct {
	id    string
	count int
}

func (r GraphResult) topExternals(n int) []externalCount {
	counts := map[string]int{}
	for _, e := range r.Edges {
		if e.Type == "external" {
			counts[e.To]++
		}
	}
	out := make([]externalCount, 0, len(counts))
	for id, c := range counts {
		out = append(out, externalCount{id: id, count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].id < out[j].id
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (r GraphResult) filesWithoutTests() []string {
	var out []string
	for _, n := range r.Nodes {
		if n.Kind == "file" && n.Role != "test" && !n.HasTests {
			out = append(out, n.ID)
		}
	}
	return out
}

func (r GraphResult) freshnessLine() string {
	state := "fresh"
	if r.Freshness.Stale {
		state = "stale (reindex for current edges)"
	}
	at := r.Freshness.LastIndexedAt
	if at == "" {
		at = "never"
	}
	return fmt.Sprintf("index: %s, last indexed %s", state, at)
}

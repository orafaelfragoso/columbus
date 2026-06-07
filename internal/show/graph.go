package show

import (
	"sort"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/store"
)

// defaultGraphCap bounds how many file nodes `show graph` returns by default.
const defaultGraphCap = 200

// GraphOptions narrows and bounds the projected graph.
type GraphOptions struct {
	In   string // keep files whose path contains this substring
	Role string // keep files with this role
	Lang string // keep files with this language
	Max  int    // node cap (<=0 uses the default)
}

// Graph projects the already-indexed file dependency graph as a typed
// GraphResult: file nodes (id = path) and external package nodes (id =
// ext:<pkg>), connected by directed import/test/external edges. It is a pure
// read over the DB cache — no live re-resolution.
func (s *Shower) Graph(opts GraphOptions) (GraphResult, error) {
	meta, err := s.DB.Meta().Get()
	if err != nil {
		return GraphResult{}, err
	}

	files, err := s.DB.AllFiles()
	if err != nil {
		return GraphResult{}, err
	}
	selected := map[string]struct{}{}
	fileMeta := map[string]graphFileMeta{}
	for _, f := range files {
		if !graphKeep(f, opts) {
			continue
		}
		selected[f.Path] = struct{}{}
		fileMeta[f.Path] = graphFileMeta{role: f.Role, language: f.Language, pkg: f.Package}
	}

	depEdges, err := s.DB.AllDepEdges()
	if err != nil {
		return GraphResult{}, err
	}
	testLinks, err := s.DB.AllTestLinks()
	if err != nil {
		return GraphResult{}, err
	}
	unresolved, err := s.DB.UnresolvedImports()
	if err != nil {
		return GraphResult{}, err
	}

	// has_tests is a file property independent of the filtered subgraph.
	hasTests := map[string]bool{}
	for _, l := range testLinks {
		hasTests[l.From] = true
	}

	// Induced subgraph: keep an edge only when both endpoints survive filtering.
	var edges []GraphEdge
	for _, e := range depEdges {
		if inSet(selected, e.From) && inSet(selected, e.To) {
			edges = append(edges, GraphEdge{From: e.From, To: e.To, Type: "import"})
		}
	}
	for _, l := range testLinks {
		if inSet(selected, l.From) && inSet(selected, l.To) {
			edges = append(edges, GraphEdge{From: l.From, To: l.To, Type: "test"})
		}
	}
	externalNodes := map[string]struct{}{}
	seenExt := map[string]bool{}
	for _, u := range unresolved {
		if !inSet(selected, u.Path) {
			continue
		}
		pkg, ok := externalPackage(u.Specifier)
		if !ok {
			continue // relative miss: dropped, never an external node
		}
		extID := "ext:" + pkg
		key := u.Path + "\x00" + extID
		if seenExt[key] {
			continue
		}
		seenExt[key] = true
		externalNodes[extID] = struct{}{}
		edges = append(edges, GraphEdge{From: u.Path, To: extID, Type: "external"})
	}

	inDeg, outDeg := degrees(edges)

	// File nodes (pre-cap), then cap by (in_degree desc, path asc).
	fileNodes := make([]GraphNode, 0, len(selected))
	for path, m := range fileMeta {
		fileNodes = append(fileNodes, GraphNode{
			ID: path, Kind: "file", Role: m.role, Language: m.language, Package: m.pkg,
			InDegree: inDeg[path], OutDegree: outDeg[path], HasTests: hasTests[path],
		})
	}
	total := len(fileNodes)
	capN := opts.Max
	if capN <= 0 {
		capN = defaultGraphCap
	}
	capped := false
	if len(fileNodes) > capN {
		sort.Slice(fileNodes, func(i, j int) bool {
			if fileNodes[i].InDegree != fileNodes[j].InDegree {
				return fileNodes[i].InDegree > fileNodes[j].InDegree
			}
			return fileNodes[i].ID < fileNodes[j].ID
		})
		fileNodes = fileNodes[:capN]
		capped = true
	}

	survivors := map[string]struct{}{}
	for _, n := range fileNodes {
		survivors[n.ID] = struct{}{}
	}

	// Prune edges and external nodes to surviving endpoints (induced again).
	keptExt := map[string]struct{}{}
	pruned := edges[:0]
	for _, e := range edges {
		switch e.Type {
		case "external":
			if _, ok := survivors[e.From]; !ok {
				continue
			}
			keptExt[e.To] = struct{}{}
		default:
			if _, ok := survivors[e.From]; !ok {
				continue
			}
			if _, ok := survivors[e.To]; !ok {
				continue
			}
		}
		pruned = append(pruned, e)
	}

	nodes := fileNodes
	for extID := range externalNodes {
		if _, ok := keptExt[extID]; ok {
			nodes = append(nodes, GraphNode{ID: extID, Kind: "external"})
		}
	}

	// Report degrees over the visible (pruned) edges so the {nodes, edges}
	// contract stays self-consistent: a node's in_degree/out_degree always
	// equals the edges present in the result. (The cap above still ranks by the
	// pre-cap, full-filtered in-degree so the most-central nodes survive.)
	finalIn, finalOut := degrees(pruned)
	for i := range nodes {
		nodes[i].InDegree = finalIn[nodes[i].ID]
		nodes[i].OutDegree = finalOut[nodes[i].ID]
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(pruned, func(i, j int) bool {
		if pruned[i].From != pruned[j].From {
			return pruned[i].From < pruned[j].From
		}
		if pruned[i].To != pruned[j].To {
			return pruned[i].To < pruned[j].To
		}
		return pruned[i].Type < pruned[j].Type
	})

	return GraphResult{
		In: opts.In, Role: opts.Role, Lang: opts.Lang,
		Nodes: nodes, Edges: pruned, Total: total, Capped: capped,
		Freshness: GraphFreshness{
			IndexedHead:   meta.IndexedHead,
			Dirty:         meta.Dirty,
			LastIndexedAt: meta.LastIndexedAt,
			Stale:         meta.Dirty || meta.LastIndexedAt == "",
		},
	}, nil
}

type graphFileMeta struct {
	role, language, pkg string
}

func graphKeep(f store.FileRow, opts GraphOptions) bool {
	if opts.In != "" && !strings.Contains(f.Path, opts.In) {
		return false
	}
	if opts.Role != "" && f.Role != opts.Role {
		return false
	}
	if opts.Lang != "" && f.Language != opts.Lang {
		return false
	}
	return true
}

func inSet(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

func degrees(edges []GraphEdge) (in, out map[string]int) {
	in, out = map[string]int{}, map[string]int{}
	for _, e := range edges {
		if e.Type == "test" {
			continue // a test link is not an import: excluded from centrality
		}
		out[e.From]++
		in[e.To]++
	}
	return in, out
}

// externalPackage collapses a bare import specifier to its package root. A
// relative specifier (starts with "." or "/") that reached this point is an
// unresolved relative miss and returns ok=false (dropped, never external).
func externalPackage(spec string) (string, bool) {
	if spec == "" || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
		return "", false
	}
	// Python dotted module: a.b.c -> a.
	if !strings.Contains(spec, "/") && strings.Contains(spec, ".") {
		return spec[:strings.IndexByte(spec, '.')], true
	}
	parts := strings.Split(spec, "/")
	// npm scoped package: @scope/name.
	if strings.HasPrefix(spec, "@") {
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1], true
		}
		return parts[0], true
	}
	// Go module-ish: host.tld/org/repo -> first three segments.
	if strings.Contains(parts[0], ".") {
		n := len(parts)
		if n > 3 {
			n = 3
		}
		return strings.Join(parts[:n], "/"), true
	}
	// Plain package: first path segment.
	return parts[0], true
}

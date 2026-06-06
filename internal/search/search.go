// Package search runs the deterministic two-source search pipeline. On the
// metadata path it generates candidates from FTS5 (over metadata only), then
// scores them with a single deterministic feature function and enriches the
// top results with 1-hop graph edges and linked memories.
package search

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// Kind selects which sources to search.
type Kind int

const (
	KindAll Kind = iota
	KindCode
	KindMemory
)

// ParseKind maps a flag value to a Kind.
func ParseKind(s string) (Kind, error) {
	switch s {
	case "", "all":
		return KindAll, nil
	case "code":
		return KindCode, nil
	case "memory":
		return KindMemory, nil
	default:
		return KindAll, contract.Errorf(contract.CodeUsage, "unknown --kind %q (want code|memory|all)", s)
	}
}

func (k Kind) String() string {
	switch k {
	case KindCode:
		return "code"
	case KindMemory:
		return "memory"
	default:
		return "all"
	}
}

// Query parameters for a search.
type Query struct {
	Text         string
	Kind         Kind
	Limit        int
	ContextLines int
	Graph        bool
}

// Engine runs searches against a store.
type Engine struct {
	DB *store.DB
}

const candidateCap = 200

// Search executes the query and returns ranked, enriched results.
func (e *Engine) Search(q Query) (SearchResult, error) {
	if strings.TrimSpace(q.Text) == "" {
		return SearchResult{}, contract.Errorf(contract.CodeUsage, "search requires a query")
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}

	meta, err := e.DB.Meta().Get()
	if err != nil {
		return SearchResult{}, err
	}
	if meta.LastIndexedAt == "" {
		return SearchResult{}, &contract.Error{
			Code:    contract.CodeIndexMissing,
			Message: "no index found",
			Hint:    "run columbus index",
		}
	}

	tokens := tokenize(q.Text)
	match := buildFTSMatch(tokens)
	if match == "" {
		return SearchResult{}, contract.Errorf(contract.CodeUsage, "query has no searchable terms")
	}

	var hits []Hit
	if q.Kind == KindAll || q.Kind == KindCode {
		codeHits, err := e.codeHits(tokens, match)
		if err != nil {
			return SearchResult{}, err
		}
		hits = append(hits, codeHits...)
	}
	if q.Kind == KindAll || q.Kind == KindMemory {
		memHits, err := e.memoryHits(match)
		if err != nil {
			return SearchResult{}, err
		}
		hits = append(hits, memHits...)
	}

	sortHits(hits)
	if len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	if err := e.enrich(hits, q.Graph); err != nil {
		return SearchResult{}, err
	}

	return SearchResult{
		Query: q.Text,
		Kind:  q.Kind.String(),
		Total: len(hits),
		Hits:  hits,
	}, nil
}

// codeHits generates and scores code candidates (symbols + files).
func (e *Engine) codeHits(tokens []string, match string) ([]Hit, error) {
	candidates, err := e.DB.SearchCodeFTS(match, candidateCap)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var hits []Hit
	for _, c := range candidates {
		key := c.Grain + ":" + fmt.Sprint(c.RefID)
		if seen[key] {
			continue
		}
		seen[key] = true

		hit, ok, err := e.codeHit(tokens, c)
		if err != nil {
			return nil, err
		}
		if ok {
			hits = append(hits, hit)
		}
	}
	return hits, nil
}

func (e *Engine) codeHit(tokens []string, c store.CodeHit) (Hit, bool, error) {
	if c.Grain == "symbol" {
		sym, ok, err := e.DB.SymbolByID(c.RefID)
		if err != nil || !ok {
			return Hit{}, false, err
		}
		count, _ := e.DB.ImportedByCount(sym.FileID)
		tests, _ := e.DB.TestsOf(sym.FileID)
		mems, _ := e.DB.MemoriesForTarget("symbol", sym.Name)
		sig := signals{
			name: sym.Name, signature: sym.Signature, path: sym.Path, role: sym.Role,
			importedByCount: count, hasTests: len(tests) > 0,
			hasMemory: len(mems) > 0, hasFailureMemory: hasFailure(mems),
		}
		return Hit{
			Grain: "symbol", Name: sym.Name, SymbolKind: sym.Kind, Container: sym.Container,
			Signature: sym.Signature, Path: sym.Path, Package: sym.Package, Role: sym.Role,
			Exported: sym.Exported,
			Score:    round(score(tokens, sig)), Why: why(tokens, sig), RiskLevel: riskLevel(sig),
			Memories: toMemoryRefs(mems),
		}, true, nil
	}

	file, ok, err := e.DB.FileByID(c.RefID)
	if err != nil || !ok {
		return Hit{}, false, err
	}
	count, _ := e.DB.ImportedByCount(file.ID)
	tests, _ := e.DB.TestsOf(file.ID)
	mems, _ := e.DB.MemoriesForTarget("file", file.Path)
	sig := signals{
		name: baseName(file.Path), path: file.Path, role: file.Role,
		importedByCount: count, hasTests: len(tests) > 0,
		hasMemory: len(mems) > 0, hasFailureMemory: hasFailure(mems),
	}
	return Hit{
		Grain: "file", Name: baseName(file.Path), Path: file.Path, Package: file.Package, Role: file.Role,
		Score: round(score(tokens, sig)), Why: why(tokens, sig), RiskLevel: riskLevel(sig),
		Memories: toMemoryRefs(mems),
	}, true, nil
}

func (e *Engine) memoryHits(match string) ([]Hit, error) {
	ids, err := e.DB.SearchMemoryFTS(match, candidateCap)
	if err != nil {
		return nil, err
	}
	var hits []Hit
	for _, id := range ids {
		m, ok, err := e.DB.MemoryBriefByID(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		risk := "low"
		if m.Kind == "failure" {
			risk = "high"
		}
		hits = append(hits, Hit{
			Grain: "memory", Name: m.Title, SymbolKind: m.Kind,
			Score: 0.6, Why: "memory match", RiskLevel: risk,
			Memories: []MemoryRef{{ID: memID(m.ID), Kind: m.Kind, Title: m.Title}},
		})
	}
	return hits, nil
}

// enrich populates graph edges for the final result set.
func (e *Engine) enrich(hits []Hit, graph bool) error {
	for i := range hits {
		h := &hits[i]
		if h.Grain == "memory" || h.Path == "" {
			continue
		}
		file, ok, err := e.DB.FileByPath(h.Path)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		tests, _ := e.DB.TestsOf(file.ID)
		h.Graph.Tests = tests
		if graph {
			imports, _ := e.DB.ImportsOf(file.ID)
			importedBy, _ := e.DB.ImportedBy(file.ID)
			h.Graph.Imports = imports
			h.Graph.ImportedBy = importedBy
		}
	}
	return nil
}

func sortHits(hits []Hit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Name != hits[j].Name {
			return hits[i].Name < hits[j].Name
		}
		return hits[i].Path < hits[j].Path
	})
}

// buildFTSMatch builds a permissive FTS5 MATCH expression: each token is quoted
// and prefix-matched, joined with OR. Quoting neutralizes FTS operators in user
// input.
func buildFTSMatch(tokens []string) string {
	var parts []string
	for _, t := range tokens {
		parts = append(parts, `"`+t+`"*`)
	}
	return strings.Join(parts, " OR ")
}

func hasFailure(mems []store.MemoryBrief) bool {
	for _, m := range mems {
		if m.Kind == "failure" {
			return true
		}
	}
	return false
}

func toMemoryRefs(mems []store.MemoryBrief) []MemoryRef {
	if len(mems) == 0 {
		return nil
	}
	out := make([]MemoryRef, len(mems))
	for i, m := range mems {
		out[i] = MemoryRef{ID: memID(m.ID), Kind: m.Kind, Title: m.Title}
	}
	return out
}

func memID(id int64) string { return fmt.Sprintf("mem_%03d", id) }
func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func round(v float64) float64 {
	return float64(int(v*10000+0.5)) / 10000
}

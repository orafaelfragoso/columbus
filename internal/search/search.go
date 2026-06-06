// Package search runs the deterministic two-source search pipeline: FTS5 over
// metadata (in-DB) and live content matches over the working tree (ripgrep
// fast-path or pure-Go fallback). Both feed a single deterministic feature
// function; the top results are enriched with live line ranges/snippets, 1-hop
// graph edges and linked memories.
package search

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/extract"
	"github.com/rafaelfragoso/columbus/internal/grep"
	"github.com/rafaelfragoso/columbus/internal/live"
	"github.com/rafaelfragoso/columbus/internal/logging"
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

// Engine runs searches against a store. When WorkDir, Registry and Searcher are
// set the live content path is enabled; otherwise search is metadata-only.
type Engine struct {
	DB       *store.DB
	WorkDir  string
	Registry *extract.Registry
	Searcher grep.Searcher
	// Logger records best-effort enrichment read failures at debug. nil = none.
	Logger *slog.Logger
}

func (e *Engine) logErr(op string, err error) { logging.DebugErr(e.Logger, op, err) }

const (
	candidateCap = 200
	contentCap   = 500
)

// codeCand is an intermediate candidate accumulating signals from both sources
// before a single scoring pass.
type codeCand struct {
	grain     string
	name      string
	kind      string
	container string
	signature string
	path      string
	pkg       string
	role      string
	exported  bool
	fileID    int64

	importedByCount int
	hasTests        bool
	mems            []store.MemoryBrief
	contentDensity  float64
}

func candKey(grain, path, container, name string) string {
	return grain + "\x00" + path + "\x00" + container + "\x00" + name
}

// Search executes the query and returns ranked, enriched results.
func (e *Engine) Search(q Query) (SearchResult, error) {
	if strings.TrimSpace(q.Text) == "" {
		return SearchResult{}, contract.Errorf(contract.CodeUsage, "search requires a query")
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.ContextLines < 0 {
		q.ContextLines = 0
	}

	meta, err := e.DB.Meta().Get()
	if err != nil {
		return SearchResult{}, err
	}
	if meta.LastIndexedAt == "" {
		return SearchResult{}, &contract.Error{Code: contract.CodeIndexMissing, Message: "no index found", Hint: "run columbus index"}
	}

	tokens := tokenize(q.Text)
	match := buildFTSMatch(tokens)
	if match == "" {
		return SearchResult{}, contract.Errorf(contract.CodeUsage, "query has no searchable terms")
	}

	res := SearchResult{Query: q.Text, Kind: q.Kind.String()}

	if q.Kind == KindAll || q.Kind == KindCode {
		cands := map[string]*codeCand{}
		if err := e.metadataCandidates(match, cands); err != nil {
			return SearchResult{}, err
		}
		live := e.liveEnabled()
		if live {
			if err := e.contentCandidates(tokens, cands); err != nil {
				return SearchResult{}, err
			}
		} else {
			res.Warnings = append(res.Warnings, "live content search disabled (metadata-only)")
		}
		for _, c := range cands {
			res.Hits = append(res.Hits, c.toHit(tokens))
		}
	}

	if q.Kind == KindAll || q.Kind == KindMemory {
		memHits, err := e.memoryHits(match)
		if err != nil {
			return SearchResult{}, err
		}
		res.Hits = append(res.Hits, memHits...)
	}

	sortHits(res.Hits)
	if len(res.Hits) > q.Limit {
		res.Hits = res.Hits[:q.Limit]
	}

	if e.liveEnabled() {
		e.resolveLive(res.Hits, q.ContextLines)
	}
	if err := e.enrichGraph(res.Hits, q.Graph); err != nil {
		return SearchResult{}, err
	}

	res.Total = len(res.Hits)
	return res, nil
}

func (e *Engine) liveEnabled() bool {
	return e.WorkDir != "" && e.Registry != nil && e.Searcher != nil
}

// metadataCandidates generates candidates from FTS over metadata.
func (e *Engine) metadataCandidates(match string, cands map[string]*codeCand) error {
	hits, err := e.DB.SearchCodeFTS(match, candidateCap)
	if err != nil {
		return err
	}
	for _, h := range hits {
		if h.Grain == "symbol" {
			sym, ok, err := e.DB.SymbolByID(h.RefID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			c := e.symbolCand(sym)
			cands[candKey("symbol", c.path, c.container, c.name)] = c
		} else {
			file, ok, err := e.DB.FileByID(h.RefID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			c := e.fileCand(file)
			cands[candKey("file", c.path, "", c.name)] = c
		}
	}
	return nil
}

func (e *Engine) symbolCand(sym store.SymbolRow) *codeCand {
	count, err := e.DB.ImportedByCount(sym.FileID)
	e.logErr("ImportedByCount", err)
	tests, err := e.DB.TestsOf(sym.FileID)
	e.logErr("TestsOf", err)
	mems, err := e.DB.MemoriesForTarget("symbol", sym.Name)
	e.logErr("MemoriesForTarget", err)
	return &codeCand{
		grain: "symbol", name: sym.Name, kind: sym.Kind, container: sym.Container,
		signature: sym.Signature, path: sym.Path, pkg: sym.Package, role: sym.Role,
		exported: sym.Exported, fileID: sym.FileID,
		importedByCount: count, hasTests: len(tests) > 0, mems: mems,
	}
}

func (e *Engine) fileCand(file store.FileRow) *codeCand {
	count, err := e.DB.ImportedByCount(file.ID)
	e.logErr("ImportedByCount", err)
	tests, err := e.DB.TestsOf(file.ID)
	e.logErr("TestsOf", err)
	mems, err := e.DB.MemoriesForTarget("file", file.Path)
	e.logErr("MemoriesForTarget", err)
	return &codeCand{
		grain: "file", name: baseName(file.Path), path: file.Path, pkg: file.Package, role: file.Role,
		fileID: file.ID, importedByCount: count, hasTests: len(tests) > 0, mems: mems,
	}
}

// contentCandidates greps the working tree and folds content-match density into
// existing candidates (or creates new ones for matches inside symbols/files not
// surfaced by metadata).
func (e *Engine) contentCandidates(tokens []string, cands map[string]*codeCand) error {
	files, err := e.DB.AllFiles()
	if err != nil {
		return err
	}
	allow := make(map[string]bool, len(files))
	byPath := make(map[string]store.FileRow, len(files))
	for _, f := range files {
		allow[f.Path] = true
		byPath[f.Path] = f
	}

	hits, err := e.Searcher.Search(e.WorkDir, tokens, allow, contentCap)
	if err != nil {
		return err
	}

	cache := live.New(e.WorkDir, e.Registry)
	type agg struct {
		count int
		sym   extract.Symbol
		file  bool
	}
	counts := map[string]*agg{}
	for _, h := range hits {
		syms := cache.Symbols(h.Path)
		if s, ok := live.Enclosing(syms, h.Line); ok {
			key := candKey("symbol", h.Path, s.Container, s.Name)
			a := counts[key]
			if a == nil {
				a = &agg{sym: s}
				counts[key] = a
			}
			a.count++
		} else {
			key := candKey("file", h.Path, "", baseName(h.Path))
			a := counts[key]
			if a == nil {
				a = &agg{file: true}
				counts[key] = a
			}
			a.count++
		}
	}

	for key, a := range counts {
		density := clamp01(float64(a.count) / 3.0)
		if c, ok := cands[key]; ok {
			c.contentDensity = density
			continue
		}
		f, ok := byPath[pathOfKey(key)]
		if !ok {
			continue
		}
		if a.file {
			c := e.fileCand(f)
			c.contentDensity = density
			cands[key] = c
		} else {
			mems, err := e.DB.MemoriesForTarget("symbol", a.sym.Name)
			e.logErr("MemoriesForTarget", err)
			count, err := e.DB.ImportedByCount(f.ID)
			e.logErr("ImportedByCount", err)
			tests, err := e.DB.TestsOf(f.ID)
			e.logErr("TestsOf", err)
			cands[key] = &codeCand{
				grain: "symbol", name: a.sym.Name, kind: string(a.sym.Kind), container: a.sym.Container,
				signature: a.sym.Signature, path: f.Path, pkg: f.Package, role: f.Role,
				exported: a.sym.Exported, fileID: f.ID,
				importedByCount: count, hasTests: len(tests) > 0, mems: mems, contentDensity: density,
			}
		}
	}
	return nil
}

func (c *codeCand) toHit(tokens []string) Hit {
	sig := signals{
		name: c.name, signature: c.signature, path: c.path, role: c.role,
		importedByCount: c.importedByCount, hasTests: c.hasTests,
		hasMemory: len(c.mems) > 0, hasFailureMemory: hasFailure(c.mems),
		contentDensity: c.contentDensity,
	}
	if c.grain == "file" {
		sig.name = c.name
	}
	return Hit{
		Grain: c.grain, Name: c.name, SymbolKind: c.kind, Container: c.container,
		Signature: c.signature, Path: c.path, Package: c.pkg, Role: c.role, Exported: c.exported,
		Score: round(score(tokens, sig)), Why: why(tokens, sig), RiskLevel: riskLevel(sig),
		Memories: toMemoryRefs(c.mems),
	}
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

// resolveLive fills in current line ranges and snippets by re-parsing the
// working tree (the stored line numbers are never trusted as truth).
func (e *Engine) resolveLive(hits []Hit, ctx int) {
	cache := live.New(e.WorkDir, e.Registry)
	for i := range hits {
		h := &hits[i]
		if h.Grain != "symbol" || h.Path == "" {
			continue
		}
		sym, ok := cache.FindSymbol(h.Path, h.Name, h.Container, h.SymbolKind)
		if !ok {
			continue
		}
		h.StartLine = sym.StartLine
		h.EndLine = sym.EndLine
		h.Snippet = live.Snippet(cache.SourceLines(h.Path), sym.StartLine, sym.EndLine, ctx)
	}
}

// enrichGraph populates 1-hop graph edges for the final result set.
func (e *Engine) enrichGraph(hits []Hit, graph bool) error {
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
		tests, err := e.DB.TestsOf(file.ID)
		e.logErr("TestsOf", err)
		h.Graph.Tests = tests
		if graph {
			h.Graph.Imports, err = e.DB.ImportsOf(file.ID)
			e.logErr("ImportsOf", err)
			h.Graph.ImportedBy, err = e.DB.ImportedBy(file.ID)
			e.logErr("ImportedBy", err)
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

// buildFTSMatch builds a permissive prefix-OR MATCH expression. Quoting each
// token neutralizes FTS operators in user input.
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

// pathOfKey extracts the path component from a candidate key.
func pathOfKey(key string) string {
	parts := strings.Split(key, "\x00")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func round(v float64) float64 {
	return float64(int(v*10000+0.5)) / 10000
}

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

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/grep"
	"github.com/orafaelfragoso/columbus/internal/live"
	"github.com/orafaelfragoso/columbus/internal/logging"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// Kind selects which sources to search.
type Kind int

const (
	KindAll Kind = iota
	KindCode
	KindMemory
	KindEpic
	KindTask
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
	case "epic":
		return KindEpic, nil
	case "task":
		return KindTask, nil
	default:
		return KindAll, contract.Errorf(contract.CodeUsage, "unknown --kind %q (want code|memory|epic|task|all)", s)
	}
}

func (k Kind) String() string {
	switch k {
	case KindCode:
		return "code"
	case KindMemory:
		return "memory"
	case KindEpic:
		return "epic"
	case KindTask:
		return "task"
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
	// Snippets attaches code-body snippets to each hit. The default (false) is
	// locate-first: locations, signatures, scores and graph edges only, which is
	// far cheaper for agents. Bodies are pulled on demand via `show` or by
	// opting in here. Line ranges are always resolved regardless.
	Snippets bool
	// SnippetLines caps snippet length when Snippets is set (0 = default
	// MaxSnippetLines).
	SnippetLines int
}

// Embedder turns the NL query into a vector. Mirrors embed.Embedder's query
// path; search only needs EmbedQuery plus the model identity (to confirm the
// index was built with the same model). Kept local so the semantic seam can be
// nil (keyword fallback) and stubbed in tests without the ONNX runtime.
type Embedder interface {
	EmbedQuery(text string) ([]float32, error)
	Model() string
}

// Engine runs searches against a store. When WorkDir, Registry and Searcher are
// set the live content path is enabled; otherwise search is metadata-only.
type Engine struct {
	DB       *store.DB
	WorkDir  string
	Registry *extract.Registry
	Searcher grep.Searcher
	// Embedder, when non-nil and matching the index's model, enables vector
	// kNN-first search. Nil or a model mismatch falls back to FTS keyword search.
	Embedder Embedder
	// Logger records best-effort enrichment read failures at debug. nil = none.
	Logger *slog.Logger
}

func (e *Engine) logErr(op string, err error) { logging.DebugErr(e.Logger, op, err) }

const (
	candidateCap = 200
	contentCap   = 500
	// vecK is the kNN fan-out per owner type on the semantic path.
	vecK = 50
	// ftsRescueCap bounds the exact-identifier FTS union folded into semantic
	// candidates (NL embeddings miss literal tokens like getUserByIDv2).
	ftsRescueCap = 10
	// wVector / wHeuristic blend the primary vector signal with the deterministic
	// re-rank. They sum to 1.0.
	wVector    = 0.70
	wHeuristic = 0.30
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

	// vectorScore is 1 - cosine_distance in [0,1] from the kNN search; hasVector
	// distinguishes a kNN hit from an FTS-only rescue candidate (vectorScore 0).
	vectorScore float64
	hasVector   bool
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
		q.Limit = 15
	}
	if q.ContextLines < 0 {
		q.ContextLines = 0
	}

	meta, err := e.DB.Meta().Get()
	if err != nil {
		return SearchResult{}, err
	}
	if meta.LastIndexedAt == "" {
		return SearchResult{}, &contract.Error{Code: contract.CodeIndexMissing, Message: "no index found", Hint: "run columbus install"}
	}

	tokens := tokenize(q.Text)
	match := buildFTSMatch(tokens)
	if match == "" {
		return SearchResult{}, contract.Errorf(contract.CodeUsage, "query has no searchable terms")
	}

	res := SearchResult{Query: q.Text, Kind: q.Kind.String()}

	// Embed the query once; every kind (code, memory, work) shares the vector.
	semantic := e.semanticEnabled(meta)
	var qvec []float32
	if semantic {
		v, eerr := e.Embedder.EmbedQuery(q.Text)
		if eerr != nil {
			res.Warnings = append(res.Warnings, "semantic search failed; using keyword fallback")
			semantic = false
		} else {
			qvec = v
		}
	} else if e.Embedder != nil {
		res.Warnings = append(res.Warnings, "semantic search unavailable; using keyword fallback")
	}

	if q.Kind == KindAll || q.Kind == KindCode {
		cands := map[string]*codeCand{}
		if semantic {
			if err := e.semanticCandidates(qvec, match, cands); err != nil {
				return SearchResult{}, err
			}
		} else {
			if err := e.metadataCandidates(match, candidateCap, cands); err != nil {
				return SearchResult{}, err
			}
			if e.liveEnabled() {
				if err := e.contentCandidates(tokens, cands); err != nil {
					return SearchResult{}, err
				}
			} else {
				res.Warnings = append(res.Warnings, "live content search disabled (metadata-only)")
			}
		}
		foldFileHits(cands)
		for _, c := range cands {
			res.Hits = append(res.Hits, c.toHit(tokens, semantic))
		}
	}

	if q.Kind == KindAll || q.Kind == KindMemory {
		var memHits []Hit
		if semantic {
			memHits, err = e.semanticMemoryHits(qvec, match)
		} else {
			memHits, err = e.memoryHits(match)
		}
		if err != nil {
			return SearchResult{}, err
		}
		res.Hits = append(res.Hits, memHits...)
	}

	if q.Kind == KindAll || q.Kind == KindEpic || q.Kind == KindTask {
		var workHits []Hit
		if semantic {
			workHits, err = e.semanticWorkHits(qvec, match, q.Kind)
		} else {
			workHits, err = e.workHits(match, q.Kind)
		}
		if err != nil {
			return SearchResult{}, err
		}
		res.Hits = append(res.Hits, workHits...)
	}

	sortHits(res.Hits)
	if len(res.Hits) > q.Limit {
		res.Hits = res.Hits[:q.Limit]
	}

	if e.liveEnabled() {
		e.resolveLive(res.Hits, q.ContextLines, q.Snippets, q.SnippetLines)
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

// metadataCandidates generates candidates from FTS over metadata, capped at cap.
// Existing keys are never overwritten, so it can both seed the keyword path and
// fold an exact-identifier rescue set into semantic candidates without clobbering
// their vector scores.
func (e *Engine) metadataCandidates(match string, limit int, cands map[string]*codeCand) error {
	hits, err := e.DB.SearchCodeFTS(match, limit)
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
			key := candKey("symbol", sym.Path, sym.Container, sym.Name)
			if _, dup := cands[key]; dup {
				continue
			}
			cands[key] = e.symbolCand(sym)
		} else {
			file, ok, err := e.DB.FileByID(h.RefID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			key := candKey("file", file.Path, "", baseName(file.Path))
			if _, dup := cands[key]; dup {
				continue
			}
			cands[key] = e.fileCand(file)
		}
	}
	return nil
}

// semanticEnabled reports whether vector kNN search can run: an embedder is set
// and the index was built with the same model (vectors across models aren't
// comparable).
func (e *Engine) semanticEnabled(meta store.Meta) bool {
	return e.Embedder != nil && meta.EmbedModel != "" && meta.EmbedModel == e.Embedder.Model()
}

// semanticCandidates runs vector kNN over symbols and files, then folds in a
// small exact-identifier FTS rescue set. vectorScore = 1 - cosine_distance.
func (e *Engine) semanticCandidates(qvec []float32, match string, cands map[string]*codeCand) error {
	symHits, err := e.DB.SearchVectors(qvec, []string{"symbol"}, vecK)
	if err != nil {
		return err
	}
	for _, h := range symHits {
		sym, ok, err := e.DB.SymbolByID(h.OwnerID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		c := e.symbolCand(sym)
		c.vectorScore = clamp01(1 - h.Distance)
		c.hasVector = true
		cands[candKey("symbol", c.path, c.container, c.name)] = c
	}

	fileHits, err := e.DB.SearchVectors(qvec, []string{"file"}, vecK)
	if err != nil {
		return err
	}
	for _, h := range fileHits {
		file, ok, err := e.DB.FileByID(h.OwnerID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		c := e.fileCand(file)
		c.vectorScore = clamp01(1 - h.Distance)
		c.hasVector = true
		cands[candKey("file", c.path, "", c.name)] = c
	}

	// Exact-identifier rescue: union top FTS hits the embedding may have missed
	// (literal tokens). These carry no vector, so the blend ranks them on
	// heuristics alone.
	return e.metadataCandidates(match, ftsRescueCap, cands)
}

// foldFileHits drops standalone file candidates when a symbol of the same file
// already surfaced, so a file isn't listed twice. Files with no surfaced symbol
// (config/docs) are kept.
func foldFileHits(cands map[string]*codeCand) {
	symbolPaths := make(map[string]bool)
	for _, c := range cands {
		if c.grain == "symbol" {
			symbolPaths[c.path] = true
		}
	}
	for key, c := range cands {
		if c.grain == "file" && symbolPaths[c.path] {
			delete(cands, key)
		}
	}
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

func (c *codeCand) toHit(tokens []string, semantic bool) Hit {
	sig := signals{
		name: c.name, signature: c.signature, path: c.path, role: c.role,
		importedByCount: c.importedByCount, hasTests: c.hasTests,
		hasMemory: len(c.mems) > 0, hasFailureMemory: hasFailure(c.mems),
		contentDensity: c.contentDensity,
	}
	heur := score(tokens, sig)
	final := heur
	reason := why(tokens, sig)
	if semantic {
		// Vector recall leads; heuristics re-rank and annotate.
		final = wVector*c.vectorScore + wHeuristic*heur
		if wVector*c.vectorScore > wHeuristic*heur {
			reason = "semantic match"
		}
	}
	return Hit{
		Grain: c.grain, Name: c.name, SymbolKind: c.kind, Container: c.container,
		Signature: c.signature, Path: c.path, Package: c.pkg, Role: c.role, Exported: c.exported,
		Score: round(final), Why: reason, RiskLevel: riskLevel(sig),
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

// workHits returns ranked epic/task results from the work FTS index, optionally
// narrowed to a single noun.
func (e *Engine) workHits(match string, kind Kind) ([]Hit, error) {
	owners, err := e.DB.SearchWorkFTS(match, candidateCap)
	if err != nil {
		return nil, err
	}
	var hits []Hit
	for _, o := range owners {
		if kind == KindEpic && o.OwnerType != "epic" {
			continue
		}
		if kind == KindTask && o.OwnerType != "task" {
			continue
		}
		hits = append(hits, Hit{
			Grain: o.OwnerType, ID: workID(o.OwnerType, o.OwnerID), Name: o.Title, SymbolKind: o.Status,
			Score: 0.6, Why: o.OwnerType + " match", RiskLevel: "low",
		})
	}
	return hits, nil
}

func workID(ownerType string, id int64) string {
	return fmt.Sprintf("%s_%03d", ownerType, id)
}

// semanticMemoryHits returns the nearest memories to the query vector, then
// unions the FTS matches so memories added since the last index (not yet
// embedded) still surface.
func (e *Engine) semanticMemoryHits(qvec []float32, match string) ([]Hit, error) {
	vhits, err := e.DB.SearchVectors(qvec, []string{"memory"}, vecK)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var hits []Hit
	for _, vh := range vhits {
		m, ok, err := e.DB.MemoryBriefByID(vh.OwnerID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		seen[m.ID] = true
		hits = append(hits, memoryHit(m, round(clamp01(1-vh.Distance)), "semantic match"))
	}
	// FTS union for un-embedded memories.
	ids, err := e.DB.SearchMemoryFTS(match, candidateCap)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		m, ok, err := e.DB.MemoryBriefByID(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		seen[id] = true
		hits = append(hits, memoryHit(m, 0.6, "memory match"))
	}
	return hits, nil
}

// semanticWorkHits returns the nearest epics/stories/tasks to the query vector
// (narrowed to the kind's owner types), unioned with the FTS matches so work
// items added since the last index still surface.
func (e *Engine) semanticWorkHits(qvec []float32, match string, kind Kind) ([]Hit, error) {
	allow := map[string]bool{}
	var owners []string
	switch kind {
	case KindEpic:
		owners = []string{"epic"}
	case KindTask:
		owners = []string{"task"}
	default:
		owners = []string{"epic", "story", "task"}
	}
	for _, o := range owners {
		allow[o] = true
	}

	vhits, err := e.DB.SearchVectors(qvec, owners, vecK)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var hits []Hit
	for _, vh := range vhits {
		o, ok, err := e.DB.WorkOwner(vh.OwnerType, vh.OwnerID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		seen[workID(o.OwnerType, o.OwnerID)] = true
		hits = append(hits, workHit(o, round(clamp01(1-vh.Distance)), "semantic match"))
	}
	// FTS union for un-embedded work items.
	owned, err := e.DB.SearchWorkFTS(match, candidateCap)
	if err != nil {
		return nil, err
	}
	for _, o := range owned {
		if !allow[o.OwnerType] || seen[workID(o.OwnerType, o.OwnerID)] {
			continue
		}
		seen[workID(o.OwnerType, o.OwnerID)] = true
		hits = append(hits, workHit(o, 0.6, o.OwnerType+" match"))
	}
	return hits, nil
}

func memoryHit(m store.MemoryBrief, score float64, why string) Hit {
	risk := "low"
	if m.Kind == "failure" {
		risk = "high"
	}
	return Hit{
		Grain: "memory", Name: m.Title, SymbolKind: m.Kind,
		Score: score, Why: why, RiskLevel: risk,
		Memories: []MemoryRef{{ID: memID(m.ID), Kind: m.Kind, Title: m.Title}},
	}
}

func workHit(o store.WorkOwner, score float64, why string) Hit {
	return Hit{
		Grain: o.OwnerType, ID: workID(o.OwnerType, o.OwnerID), Name: o.Title, SymbolKind: o.Status,
		Score: score, Why: why, RiskLevel: "low",
	}
}

// resolveLive fills in current line ranges and snippets by re-parsing the
// working tree (the stored line numbers are never trusted as truth).
func (e *Engine) resolveLive(hits []Hit, ctx int, snippets bool, maxLines int) {
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
		// Line ranges are always resolved live (cheap, and the locator's primary
		// value); the code body is only attached when snippets are requested.
		h.StartLine = sym.StartLine
		h.EndLine = sym.EndLine
		if snippets {
			h.Snippet = live.SnippetN(cache.SourceLines(h.Path), sym.StartLine, sym.EndLine, ctx, maxLines)
		}
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

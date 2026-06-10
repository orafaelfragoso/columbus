package search

import (
	"crypto/sha256"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/grep"
	"github.com/orafaelfragoso/columbus/internal/index"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// topicEmbedder is a deterministic, runtime-free embedder used at BOTH index
// time (Embed) and query time (EmbedQuery) so semantic matches are reproducible
// without the real model. Texts mentioning a known topic collapse onto a shared
// one-hot basis vector; a NL query mentioning the same topic therefore matches
// the symbol whose BODY mentions it, even with zero name/token overlap.
type topicEmbedder struct{}

var topics = []string{"auth", "render", "database", "parse"}

func (topicEmbedder) Model() string { return "topic-v1" }
func (topicEmbedder) Dim() int      { return 256 }

func (topicEmbedder) Embed(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = topicVec(t)
	}
	return out, nil
}

func (topicEmbedder) EmbedQuery(text string) ([]float32, error) {
	return topicVec(text), nil
}

func topicVec(s string) []float32 {
	v := make([]float32, 256)
	low := strings.ToLower(s)
	matched := false
	for i, t := range topics {
		if strings.Contains(low, t) {
			v[i] = 1
			matched = true
		}
	}
	if !matched {
		// Unrelated text lands far from every topic basis vector.
		h := sha256.Sum256([]byte(s))
		v[4+int(h[0])%(256-4)] = 1
	}
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm > 0 {
		inv := float32(1 / math.Sqrt(norm))
		for i := range v {
			v[i] *= inv
		}
	}
	return v
}

// buildSemanticEngine indexes the fixture with the topic embedder and returns an
// engine wired for semantic (vector kNN) search.
func buildSemanticEngine(t *testing.T, files map[string]string) *Engine {
	t.Helper()
	work := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	for rel, content := range files {
		full := filepath.Join(work, rel)
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	add := exec.Command("git", "add", "-A")
	add.Dir = work
	add.Run()

	reg, _ := extract.NewRegistry()
	db, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ix := &index.Indexer{
		DB: db, Registry: reg, WorkDir: work,
		Clock:       clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)},
		MaxFileSize: config.DefaultMaxFileSize,
		Excludes:    config.Default().Indexing.Exclude,
		Embedder:    topicEmbedder{},
	}
	if _, err := ix.Run(index.ModeFull); err != nil {
		t.Fatalf("index: %v", err)
	}
	return &Engine{DB: db, WorkDir: work, Registry: reg, Searcher: grep.New(), Embedder: topicEmbedder{}}
}

// TestSemanticFindsSymbolWithoutTokenOverlap is the headline acceptance: a NL
// query finds the right symbol by meaning, not by any shared identifier.
func TestSemanticFindsSymbolWithoutTokenOverlap(t *testing.T) {
	e := buildSemanticEngine(t, map[string]string{
		"creds.go":  "package svc\n\nfunc checkCredentials() bool {\n\t// validate the auth token before access\n\treturn true\n}\n",
		"screen.go": "package svc\n\nfunc draw() {\n\t// render the screen pixels\n}\n",
	})
	res, err := e.Search(Query{Text: "where do we validate auth tokens", Kind: KindCode})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("no hits")
	}
	top := res.Hits[0]
	if top.Name != "checkCredentials" {
		t.Fatalf("top hit = %q, want checkCredentials (semantic)", top.Name)
	}
	if top.Why != "semantic match" {
		t.Errorf("why = %q, want %q", top.Why, "semantic match")
	}
}

// TestSemanticFoldsFileIntoSymbol: the file vector for creds.go also hits, but
// must not appear as a standalone result alongside its symbol.
func TestSemanticFoldsFileIntoSymbol(t *testing.T) {
	e := buildSemanticEngine(t, map[string]string{
		"creds.go": "package svc\n\nfunc checkCredentials() bool {\n\t// validate the auth token\n\treturn true\n}\n",
	})
	res, err := e.Search(Query{Text: "validate auth token", Kind: KindCode})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, h := range res.Hits {
		if h.Grain == "file" && h.Path == "creds.go" {
			t.Errorf("creds.go listed as standalone file despite a symbol hit; hits=%+v", res.Hits)
		}
	}
}

// TestSemanticReRankByHeuristic: two symbols with identical vector scores are
// ordered by the deterministic re-rank (exact name match wins).
func TestSemanticReRankByHeuristic(t *testing.T) {
	e := buildSemanticEngine(t, map[string]string{
		"a.go": "package svc\n\nfunc Authenticate() {\n\t// auth token check\n}\n",
		"b.go": "package svc\n\nfunc Login() {\n\t// auth token check\n}\n",
	})
	res, err := e.Search(Query{Text: "Authenticate auth", Kind: KindCode})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var authRank, loginRank = -1, -1
	for i, h := range res.Hits {
		switch h.Name {
		case "Authenticate":
			authRank = i
		case "Login":
			loginRank = i
		}
	}
	if authRank == -1 || loginRank == -1 {
		t.Fatalf("expected both symbols; hits=%+v", res.Hits)
	}
	if authRank > loginRank {
		t.Errorf("Authenticate (rank %d) should outrank Login (rank %d) via exact-name re-rank", authRank, loginRank)
	}
}

// TestSemanticDeterministic: same query + db + model -> identical ranked output.
func TestSemanticDeterministic(t *testing.T) {
	e := buildSemanticEngine(t, map[string]string{
		"creds.go":  "package svc\n\nfunc checkCredentials() bool {\n\t// validate the auth token\n\treturn true\n}\n",
		"screen.go": "package svc\n\nfunc draw() {\n\t// render the screen\n}\n",
	})
	q := Query{Text: "validate auth tokens", Kind: KindCode}
	first, err := e.Search(q)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.Search(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Hits) != len(second.Hits) {
		t.Fatalf("hit count differs: %d vs %d", len(first.Hits), len(second.Hits))
	}
	for i := range first.Hits {
		if first.Hits[i].Name != second.Hits[i].Name || first.Hits[i].Score != second.Hits[i].Score {
			t.Errorf("nondeterministic at %d: %+v vs %+v", i, first.Hits[i], second.Hits[i])
		}
	}
}

// TestSemanticFallbackWhenNoVectors: an embedder is set but the index has no
// vectors (built without one) -> keyword fallback with a warning.
func TestSemanticFallbackWhenNoVectors(t *testing.T) {
	// buildLiveEngine indexes WITHOUT an embedder, so meta.embed_model is empty.
	e := buildLiveEngine(t, map[string]string{
		"svc.go": "package svc\n\nfunc Handler() string {\n\treturn \"ok\"\n}\n",
	})
	e.Embedder = topicEmbedder{} // present, but index has no matching vectors

	res, err := e.Search(Query{Text: "Handler", Kind: KindCode})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !hasWarning(res.Warnings, "keyword fallback") {
		t.Errorf("expected keyword-fallback warning, got %v", res.Warnings)
	}
	// Keyword path still resolves the exact-name hit.
	found := false
	for _, h := range res.Hits {
		if h.Name == "Handler" {
			found = true
		}
	}
	if !found {
		t.Errorf("fallback keyword search lost Handler; hits=%+v", res.Hits)
	}
}

// TestLimitDefaultFifteen: an unset Limit defaults to 15.
func TestLimitDefaultFifteen(t *testing.T) {
	var b strings.Builder
	b.WriteString("package big\n\n")
	for i := 0; i < 25; i++ {
		// Each function body mentions auth so the query vector matches them all.
		b.WriteString("func F")
		b.WriteByte(byte('A' + i))
		b.WriteString("() {\n\t// auth token\n}\n\n")
	}
	e := buildSemanticEngine(t, map[string]string{"big.go": b.String()})
	res, err := e.Search(Query{Text: "auth", Kind: KindCode}) // Limit unset
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) > 15 {
		t.Errorf("default limit not applied: %d hits, want <= 15", len(res.Hits))
	}
}

// TestSemanticFindsMemoryWithFullBody: a memory embedded at index time
// surfaces in the search's memories section with its full body.
func TestSemanticFindsMemoryWithFullBody(t *testing.T) {
	e := buildSemanticEngine(t, map[string]string{"svc.go": "package svc\n\nfunc New() {}\n"})

	// Create a memory whose body is about auth.
	if err := e.DB.WithTx(func(tx *store.Tx) error {
		id, err := tx.NextMemSeq()
		if err != nil {
			return err
		}
		if err := tx.InsertMemory(id, "adr", "Token checks", "validate the auth token on each request", "t", "t"); err != nil {
			return err
		}
		return tx.ReindexMemoryFTS(id, "Token checks", "validate the auth token on each request", nil)
	}); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	// Re-index to embed the new memory (memories embed at index time).
	ix := &index.Indexer{
		DB: e.DB, Registry: e.Registry, WorkDir: e.WorkDir,
		Clock:       clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)},
		MaxFileSize: config.DefaultMaxFileSize,
		Excludes:    config.Default().Indexing.Exclude,
		Embedder:    topicEmbedder{},
	}
	if _, err := ix.Run(index.ModeIncremental); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	res, err := e.Search(Query{Text: "where do we validate auth tokens", Kind: KindAll})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, m := range res.Memories {
		if m.ID == "mem_001" {
			found = true
			if m.Body != "validate the auth token on each request" {
				t.Errorf("memory body = %q, want full body", m.Body)
			}
			if m.Kind != "adr" {
				t.Errorf("memory kind = %q, want adr", m.Kind)
			}
		}
	}
	if !found {
		t.Errorf("memory not found in memories section; memories=%+v", res.Memories)
	}
}

func TestFoldFileHitsUnit(t *testing.T) {
	cands := map[string]*codeCand{
		candKey("symbol", "a.go", "", "Foo"): {grain: "symbol", path: "a.go", name: "Foo"},
		candKey("file", "a.go", "", "a.go"):  {grain: "file", path: "a.go", name: "a.go"},
		candKey("file", "b.go", "", "b.go"):  {grain: "file", path: "b.go", name: "b.go"},
	}
	foldFileHits(cands)
	if _, ok := cands[candKey("file", "a.go", "", "a.go")]; ok {
		t.Error("a.go file cand should be folded into its symbol")
	}
	if _, ok := cands[candKey("file", "b.go", "", "b.go")]; !ok {
		t.Error("b.go has no symbol hit; its file cand must survive")
	}
}

func hasWarning(warnings []string, sub string) bool {
	for _, w := range warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

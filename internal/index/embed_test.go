package index

import (
	"crypto/sha256"
	"math"
	"strings"
	"testing"
)

// fakeEmbedder is a deterministic, runtime-free stand-in for embed.Embedder:
// it derives a normalized 256-d vector from each text and records call counts
// and the texts it saw, so tests can assert embed/skip behavior precisely.
type fakeEmbedder struct {
	calls int
	seen  []string
}

func (f *fakeEmbedder) Model() string { return "fake-v1" }
func (f *fakeEmbedder) Dim() int      { return 256 }

func (f *fakeEmbedder) Embed(texts []string) ([][]float32, error) {
	f.calls++
	f.seen = append(f.seen, texts...)
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = fakeVec(t)
	}
	return out, nil
}

func fakeVec(s string) []float32 {
	v := make([]float32, 256)
	sum := sha256.Sum256([]byte(s))
	var norm float64
	for i := range v {
		x := float32(sum[i%32]) - 128
		v[i] = x
		norm += float64(x) * float64(x)
	}
	if norm > 0 {
		inv := float32(1.0 / math.Sqrt(norm))
		for i := range v {
			v[i] *= inv
		}
	}
	return v
}

// vecCount returns how many vectors of an owner_type are stored.
func vecCount(t *testing.T, ix *Indexer, ownerType string) int {
	t.Helper()
	var n int
	if err := ix.DB.SQL().QueryRow(`SELECT COUNT(*) FROM chunk_meta WHERE owner_type = ?`, ownerType).Scan(&n); err != nil {
		t.Fatalf("count %s vectors: %v", ownerType, err)
	}
	return n
}

func TestEmbedPopulatesSymbolAndFileVectors(t *testing.T) {
	ix, _ := newIndexer(t)
	ix.Embedder = &fakeEmbedder{}

	res, err := ix.Run(ModeFull)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Embedded == 0 {
		t.Fatal("no chunks embedded")
	}
	// Every symbol and every file must have exactly one vector.
	if got := vecCount(t, ix, symbolOwner); got != res.Symbols {
		t.Errorf("symbol vectors = %d, want %d", got, res.Symbols)
	}
	if got := vecCount(t, ix, fileOwner); got != res.TotalFiles {
		t.Errorf("file vectors = %d, want %d (one per file)", got, res.TotalFiles)
	}
	// index_meta records the model + dim.
	m, _ := ix.DB.Meta().Get()
	if m.EmbedModel != "fake-v1" || m.EmbedDim != 256 {
		t.Errorf("embed meta = %q/%d, want fake-v1/256", m.EmbedModel, m.EmbedDim)
	}
}

func TestEmbedReindexNoChangeZeroEmbeds(t *testing.T) {
	ix, _ := newIndexer(t)
	fe := &fakeEmbedder{}
	ix.Embedder = fe
	if _, err := ix.Run(ModeIncremental); err != nil {
		t.Fatal(err)
	}
	before := vecCount(t, ix, symbolOwner)

	fe.calls = 0
	fe.seen = nil
	res, err := ix.Run(ModeIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if fe.calls != 0 || res.Embedded != 0 {
		t.Errorf("second run embedded: calls=%d embedded=%d, want 0", fe.calls, res.Embedded)
	}
	if after := vecCount(t, ix, symbolOwner); after != before {
		t.Errorf("symbol vectors changed: %d -> %d", before, after)
	}
}

func TestEmbedTouchOneFunctionReembedsOnlyIt(t *testing.T) {
	ix, work := newIndexer(t)
	fe := &fakeEmbedder{}
	ix.Embedder = fe
	if _, err := ix.Run(ModeIncremental); err != nil {
		t.Fatal(err)
	}

	// Change only New's body; Server is untouched (same source text).
	write(t, work, "svc.go", "package svc\n\nfunc New() int { return 2 }\n\ntype Server struct{}\n")
	fe.calls = 0
	fe.seen = nil
	res, err := ix.Run(ModeIncremental)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly one symbol re-embedded (New); Server carried forward.
	if res.Embedded != 1 {
		t.Errorf("embedded = %d, want 1 (only New)", res.Embedded)
	}
	if res.EmbedSkipped != 1 {
		t.Errorf("skipped = %d, want 1 (Server carried)", res.EmbedSkipped)
	}
	// Only New's chunk text should have reached the embedder.
	for _, s := range fe.seen {
		if strings.Contains(s, "Server") {
			t.Errorf("Server should not be re-embedded; saw %q", s)
		}
	}
}

func TestEmbedFileWithNoSymbolsUsesFallback(t *testing.T) {
	ix, work := newIndexer(t)
	fe := &fakeEmbedder{}
	ix.Embedder = fe
	// A plain-text file has no grammar and thus no extracted symbols.
	write(t, work, "notes.txt", "just some prose, no code symbols here\n")

	if _, err := ix.Run(ModeFull); err != nil {
		t.Fatal(err)
	}
	// Its fallback text (path/package/role) must have been embedded directly.
	found := false
	for _, s := range fe.seen {
		if strings.Contains(s, "notes.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("file-with-no-symbols fallback text was not embedded; seen=%v", fe.seen)
	}
}

func TestEmbedNilEmbedderNoVectors(t *testing.T) {
	ix, _ := newIndexer(t)
	ix.Embedder = nil // metadata-only

	res, err := ix.Run(ModeFull)
	if err != nil {
		t.Fatalf("run without embedder: %v", err)
	}
	if res.Embedded != 0 || res.EmbedSkipped != 0 {
		t.Errorf("embed stats nonzero without embedder: %+v", res)
	}
	if got := vecCount(t, ix, symbolOwner) + vecCount(t, ix, fileOwner); got != 0 {
		t.Errorf("stored %d vectors without embedder, want 0", got)
	}
}

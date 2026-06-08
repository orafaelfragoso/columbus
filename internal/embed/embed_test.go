package embed

import (
	"context"
	"math"
	"testing"
)

func newEmbedder(t *testing.T) Embedder {
	t.Helper()
	e, err := New(context.Background())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

// goldenSentence and goldenHead pin the first components of the Model2Vec,
// L2-normalized vector. They guard against model, tokenizer, or pooling drift.
const goldenSentence = "The quick brown fox jumps over the lazy dog."

var goldenHead = []float32{
	0.22601303, 0.039419904, 0.11196691, 0.16305041,
	-0.09970481, 0.052625258, 0.0472825, 0.20182556,
}

const tol = 1e-5

func TestDimAndModel(t *testing.T) {
	e := newEmbedder(t)
	if e.Dim() != Dim || e.Dim() != 256 {
		t.Errorf("Dim() = %d, want 256", e.Dim())
	}
	if e.Model() != Model || e.Model() != "minishlab/potion-code-16M" {
		t.Errorf("Model() = %q, want minishlab/potion-code-16M", e.Model())
	}
}

func TestDeterminism(t *testing.T) {
	e := newEmbedder(t)
	a, err := e.Embed([]string{goldenSentence})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Embed([]string{goldenSentence})
	if err != nil {
		t.Fatal(err)
	}
	if len(a[0]) != len(b[0]) {
		t.Fatalf("length mismatch: %d vs %d", len(a[0]), len(b[0]))
	}
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("non-deterministic at %d: %v != %v", i, a[0][i], b[0][i])
		}
	}
}

func TestGolden(t *testing.T) {
	e := newEmbedder(t)
	v, err := e.Embed([]string{goldenSentence})
	if err != nil {
		t.Fatal(err)
	}
	if len(v[0]) != Dim {
		t.Fatalf("Dim = %d, want %d", len(v[0]), Dim)
	}
	for i, want := range goldenHead {
		if d := math.Abs(float64(v[0][i] - want)); d > tol {
			t.Errorf("component %d = %v, want %v (|d|=%g > %g)", i, v[0][i], want, d, tol)
		}
	}
}

func TestNormalized(t *testing.T) {
	e := newEmbedder(t)
	vecs, err := e.Embed([]string{goldenSentence, "another unrelated sentence about gardening"})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range vecs {
		if n := norm(v); math.Abs(n-1.0) > tol {
			t.Errorf("row %d norm = %v, want 1.0", i, n)
		}
	}
}

func TestBatchEqualsSingle(t *testing.T) {
	e := newEmbedder(t)
	a, b := "database connection pooling reuses sockets", "the cat sat on the warm windowsill"

	batch, err := e.Embed([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	sa, err := e.Embed([]string{a})
	if err != nil {
		t.Fatal(err)
	}
	sb, err := e.Embed([]string{b})
	if err != nil {
		t.Fatal(err)
	}
	assertClose(t, "row a", batch[0], sa[0])
	assertClose(t, "row b", batch[1], sb[0])
}

func TestQueryUsesSameStaticSpace(t *testing.T) {
	e := newEmbedder(t)
	const x = "how do I configure connection pooling"
	q, err := e.EmbedQuery(x)
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.Embed([]string{x})
	if err != nil {
		t.Fatal(err)
	}
	assertClose(t, "query/document", q, d[0])
}

func TestSemanticSignal(t *testing.T) {
	e := newEmbedder(t)
	vecs, err := e.Embed([]string{
		"How do I open a file in Go?",
		"Reading a file from disk using Golang",
		"The recipe calls for two cups of flour",
	})
	if err != nil {
		t.Fatal(err)
	}
	related := cosine(vecs[0], vecs[1])
	unrelated := cosine(vecs[0], vecs[2])
	if related <= unrelated {
		t.Errorf("no semantic signal: related=%.4f should exceed unrelated=%.4f", related, unrelated)
	}
}

func TestEmptyInput(t *testing.T) {
	e := newEmbedder(t)
	v, err := e.Embed(nil)
	if err != nil || v != nil {
		t.Errorf("Embed(nil) = %v, %v; want nil, nil", v, err)
	}
}

func assertClose(t *testing.T, name string, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		if d := math.Abs(float64(got[i] - want[i])); d > tol {
			t.Fatalf("%s[%d] = %v, want %v (|d|=%g > %g)", name, i, got[i], want[i], d, tol)
		}
	}
}

func norm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

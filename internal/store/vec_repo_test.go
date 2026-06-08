package store

import (
	"math"
	"testing"
)

// unit returns an L2-normalized copy of v (vectors are stored pre-normalized so
// cosine distance is meaningful).
func unit(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	n := float32(math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

// vec384 builds a 384-d vector whose first few components are set, rest zero,
// then normalizes it.
func vec384(head ...float32) []float32 {
	v := make([]float32, 384)
	copy(v, head)
	return unit(v)
}

func TestVecUpsertAndSearchRoundTrip(t *testing.T) {
	db := openTemp(t)
	const model = "bge-small-en-v1.5"

	q := vec384(1, 0, 0)
	if err := db.UpsertVector("file", 10, model, "sha-a", q); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.UpsertVector("file", 11, model, "sha-b", vec384(0, 1, 0)); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	hits, err := db.SearchVectors(q, nil, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	// The identical vector must rank first at ~0 distance.
	if hits[0].OwnerType != "file" || hits[0].OwnerID != 10 {
		t.Errorf("nearest = %+v, want file/10", hits[0])
	}
	if hits[0].Distance > 1e-5 {
		t.Errorf("self distance = %v, want ~0", hits[0].Distance)
	}
	if !(hits[0].Distance < hits[1].Distance) {
		t.Errorf("distances not ordered: %v then %v", hits[0].Distance, hits[1].Distance)
	}
}

func TestVecOwnerTypeFilter(t *testing.T) {
	db := openTemp(t)
	const model = "m"
	q := vec384(1, 0, 0)
	if err := db.UpsertVector("file", 1, model, "s1", q); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertVector("symbol", 2, model, "s2", q); err != nil {
		t.Fatal(err)
	}

	hits, err := db.SearchVectors(q, []string{"symbol"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].OwnerType != "symbol" || hits[0].OwnerID != 2 {
		t.Fatalf("filter failed: %+v", hits)
	}
}

func TestVecChunkSHASkipPath(t *testing.T) {
	db := openTemp(t)
	const model = "m"
	if _, ok, err := db.ChunkSHA("memory", 7, model); err != nil || ok {
		t.Fatalf("ChunkSHA before insert = ok %v, err %v; want false, nil", ok, err)
	}
	if err := db.UpsertVector("memory", 7, model, "abc123", vec384(1)); err != nil {
		t.Fatal(err)
	}
	sha, ok, err := db.ChunkSHA("memory", 7, model)
	if err != nil || !ok || sha != "abc123" {
		t.Fatalf("ChunkSHA = %q, %v, %v; want abc123, true, nil", sha, ok, err)
	}
}

func TestVecUpsertReplacesInPlace(t *testing.T) {
	db := openTemp(t)
	const model = "m"
	if err := db.UpsertVector("file", 1, model, "old", vec384(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	// Re-embed same owner+model with a new vector and sha.
	if err := db.UpsertVector("file", 1, model, "new", vec384(0, 1, 0)); err != nil {
		t.Fatal(err)
	}
	sha, _, _ := db.ChunkSHA("file", 1, model)
	if sha != "new" {
		t.Errorf("sha = %q, want new", sha)
	}
	// Exactly one row should remain, and nearest to (0,1,0) is itself at ~0.
	hits, err := db.SearchVectors(vec384(0, 1, 0), nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d rows after replace, want 1", len(hits))
	}
	if hits[0].Distance > 1e-5 {
		t.Errorf("replaced vector distance = %v, want ~0", hits[0].Distance)
	}
}

func TestVecDelete(t *testing.T) {
	db := openTemp(t)
	const model = "m"
	for _, id := range []int64{1, 2, 3} {
		if err := db.UpsertVector("file", id, model, "s", vec384(1, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DeleteVectors("file", []int64{1, 3}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	hits, err := db.SearchVectors(vec384(1, 0, 0), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].OwnerID != 2 {
		t.Fatalf("after delete got %+v, want only file/2", hits)
	}
	if _, ok, _ := db.ChunkSHA("file", 1, model); ok {
		t.Error("chunk_meta row for file/1 not removed")
	}
}

func TestEmbedInfoRoundTrip(t *testing.T) {
	db := openTemp(t)
	m, err := db.Meta().Get()
	if err != nil {
		t.Fatal(err)
	}
	if m.EmbedModel != "" || m.EmbedDim != 0 {
		t.Errorf("fresh embed info = %q/%d, want empty", m.EmbedModel, m.EmbedDim)
	}
	if err := db.Meta().SetEmbedInfo("bge-small-en-v1.5", 384); err != nil {
		t.Fatal(err)
	}
	m, _ = db.Meta().Get()
	if m.EmbedModel != "bge-small-en-v1.5" || m.EmbedDim != 384 {
		t.Errorf("embed info = %q/%d, want bge-small-en-v1.5/384", m.EmbedModel, m.EmbedDim)
	}
}

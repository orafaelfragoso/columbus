package embed

import (
	"context"

	model2vec "github.com/townsendmerino/aikit/embed"
)

// Model/space constants. Dim and Model are exported so the vector store can
// pin the index to the exact model that produced its vectors.
const (
	// Dim is the embedding dimensionality of minishlab/potion-code-16M.
	Dim = 256
	// Model is the model identifier embedded vectors are tagged with.
	Model = "minishlab/potion-code-16M"
)

// Embedder turns text into L2-normalized 256-d vectors on-device.
type Embedder interface {
	// Embed encodes documents verbatim, batched, returning one unit vector per
	// input in order.
	Embed(texts []string) ([][]float32, error)
	// EmbedQuery encodes a search string, returning a single unit vector in the
	// same static Model2Vec space as indexed documents.
	EmbedQuery(text string) ([]float32, error)
	// Dim reports the vector dimensionality (256).
	Dim() int
	// Model reports the model identifier ("minishlab/potion-code-16M").
	Model() string
	// Close releases resources held by the embedder.
	Close() error
}

type staticEmbedder struct {
	model *model2vec.StaticModel
}

// New loads the embedded Model2Vec assets and returns a ready Embedder.
func New(ctx context.Context) (Embedder, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	model, err := loadModel()
	if err != nil {
		return nil, err
	}
	if model.Dim() != Dim {
		return nil, embedFailure("load model: dimension %d, want %d", model.Dim(), Dim)
	}
	return &staticEmbedder{model: model}, nil
}

func (e *staticEmbedder) Dim() int      { return Dim }
func (e *staticEmbedder) Model() string { return Model }

func (e *staticEmbedder) Embed(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	vecs := make([][]float32, len(texts))
	for i, text := range texts {
		vecs[i] = e.model.Encode(text)
	}
	return vecs, nil
}

func (e *staticEmbedder) EmbedQuery(text string) ([]float32, error) {
	vecs, err := e.Embed([]string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *staticEmbedder) Close() error {
	e.model = nil
	return nil
}

package embed

import (
	"context"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// Model/space constants. Dim and Model are exported so the vector store can
// pin the index to the exact model that produced its vectors.
const (
	// Dim is the embedding dimensionality of bge-small-en-v1.5.
	Dim = 384
	// Model is the model identifier embedded vectors are tagged with.
	Model = "bge-small-en-v1.5"

	// queryInstruction is the bge retrieval prefix. Prepending it to a search
	// string is what makes EmbedQuery land in the same space as the passages
	// indexed by Embed. Documents get no prefix.
	queryInstruction = "Represent this sentence for searching relevant passages: "
)

// Embedder turns text into L2-normalized 384-d vectors on-device.
type Embedder interface {
	// Embed encodes documents verbatim, batched, returning one unit vector per
	// input in order.
	Embed(texts []string) ([][]float32, error)
	// EmbedQuery encodes a search string with the bge retrieval instruction
	// prefix applied, returning a single unit vector.
	EmbedQuery(text string) ([]float32, error)
	// Dim reports the vector dimensionality (384).
	Dim() int
	// Model reports the model identifier ("bge-small-en-v1.5").
	Model() string
	// Close releases the ONNX session and tokenizer.
	Close() error
}

// ortEmbedder is the onnxruntime-backed Embedder. Its session is guarded by mu
// because onnxruntime sessions are not reentrant; batching keeps the lock cheap.
type ortEmbedder struct {
	tok  *tokenizer
	sess *ort.DynamicAdvancedSession
	mu   sync.Mutex
}

// New loads the embedded model and tokenizer and returns a ready Embedder. It
// fails with contract.CodeRuntimeMissing (hinting at columbus doctor) when the
// onnxruntime shared library cannot be loaded.
func New(ctx context.Context) (Embedder, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tok, err := newTokenizer(tokenizerJSON)
	if err != nil {
		return nil, err
	}
	sess, err := newSession()
	if err != nil {
		tok.Close()
		return nil, err
	}
	return &ortEmbedder{tok: tok, sess: sess}, nil
}

func (e *ortEmbedder) Dim() int      { return Dim }
func (e *ortEmbedder) Model() string { return Model }

func (e *ortEmbedder) Embed(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	b, err := e.tok.encodeBatch(texts)
	if err != nil {
		return nil, err
	}
	return e.run(b)
}

func (e *ortEmbedder) EmbedQuery(text string) ([]float32, error) {
	vecs, err := e.Embed([]string{queryInstruction + text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *ortEmbedder) Close() error {
	var err error
	if e.sess != nil {
		if derr := e.sess.Destroy(); derr != nil {
			err = derr
		}
		e.sess = nil
	}
	if e.tok != nil {
		e.tok.Close()
		e.tok = nil
	}
	return err
}

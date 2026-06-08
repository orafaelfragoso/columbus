package embed

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

// Model assets are compiled into the binary. The build fails loudly if
// assets/model.onnx is absent (it is fetched, not committed: see `make
// fetch-model`); tokenizer.json is small enough to commit.
//
//go:embed assets/model.onnx
var modelONNX []byte

//go:embed assets/tokenizer.json
var tokenizerJSON []byte

// BERT input/output tensor names, confirmed against the bge-small-en-v1.5 ONNX
// graph: three int64 [batch, seq] inputs, one float [batch, seq, 384] output.
var (
	inputNames  = []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames = []string{"last_hidden_state"}
)

// The onnxruntime environment is a process-global singleton initialized once.
var (
	ortInitOnce sync.Once
	ortInitErr  error
)

// initRuntime resolves and loads the onnxruntime shared library, then
// initializes the global ORT environment, exactly once for the process.
func initRuntime() error {
	ortInitOnce.Do(func() {
		if path := resolveORTLib(); path != "" {
			ort.SetSharedLibraryPath(path)
		}
		if err := ort.InitializeEnvironment(); err != nil {
			ortInitErr = runtimeMissing(err)
		}
	})
	return ortInitErr
}

// resolveORTLib locates the onnxruntime shared library, in order: the
// COLUMBUS_ORT_LIB env var, then a library sitting next to the executable,
// then "" to let onnxruntime_go fall back to the OS default search. (Release
// bundling is spec 7; here we only resolve and fail clearly.)
func resolveORTLib() string {
	if p := os.Getenv("COLUMBUS_ORT_LIB"); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, name := range ortLibNames() {
			cand := filepath.Join(dir, name)
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
		}
	}
	return ""
}

func ortLibNames() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"onnxruntime.dll"}
	case "darwin":
		return []string{"libonnxruntime.dylib"}
	default:
		return []string{"libonnxruntime.so"}
	}
}

// newSession initializes the runtime (once) and opens a dynamic session over
// the embedded model, leaving batch and sequence dimensions free.
func newSession() (*ort.DynamicAdvancedSession, error) {
	if err := initRuntime(); err != nil {
		return nil, err
	}
	sess, err := ort.NewDynamicAdvancedSessionWithONNXData(modelONNX, inputNames, outputNames, nil)
	if err != nil {
		return nil, embedFailure("create onnx session: %v", err)
	}
	return sess, nil
}

// run encodes one batch through the session under the mutex (sessions are not
// reentrant), then CLS-pools and L2-normalizes each row.
func (e *ortEmbedder) run(b *batch) ([][]float32, error) {
	shape := ort.NewShape(int64(b.rows), int64(b.seqLen))

	ids, err := ort.NewTensor(shape, b.ids)
	if err != nil {
		return nil, embedFailure("build input_ids tensor: %v", err)
	}
	defer func() { _ = ids.Destroy() }()
	mask, err := ort.NewTensor(shape, b.mask)
	if err != nil {
		return nil, embedFailure("build attention_mask tensor: %v", err)
	}
	defer func() { _ = mask.Destroy() }()
	types, err := ort.NewTensor(shape, b.types)
	if err != nil {
		return nil, embedFailure("build token_type_ids tensor: %v", err)
	}
	defer func() { _ = types.Destroy() }()

	out, err := ort.NewEmptyTensor[float32](ort.NewShape(int64(b.rows), int64(b.seqLen), int64(Dim)))
	if err != nil {
		return nil, embedFailure("allocate output tensor: %v", err)
	}
	defer func() { _ = out.Destroy() }()

	e.mu.Lock()
	err = e.sess.Run([]ort.Value{ids, mask, types}, []ort.Value{out})
	e.mu.Unlock()
	if err != nil {
		return nil, embedFailure("onnx session run: %v", err)
	}

	hidden := out.GetData() // flattened [rows, seq, Dim]
	vecs := make([][]float32, b.rows)
	for r := 0; r < b.rows; r++ {
		v := clsVector(hidden, r, b.seqLen, Dim)
		l2normalize(v)
		vecs[r] = v
	}
	return vecs, nil
}

func runtimeMissing(err error) error {
	return contract.Errorf(contract.CodeRuntimeMissing,
		"onnxruntime shared library not loadable: %v", err).
		WithHint("install onnxruntime or set COLUMBUS_ORT_LIB to its path, then run 'columbus doctor'")
}

func embedFailure(format string, args ...any) error {
	return contract.Errorf(contract.CodeEmbedFailure, "%s", fmt.Sprintf(format, args...))
}

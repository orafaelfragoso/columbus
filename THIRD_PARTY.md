# Third-party components

Columbus bundles or links the following third-party components. Their licenses
are reproduced here as required; each remains under its own terms.

## Model weights

- **bge-small-en-v1.5** (`BAAI/bge-small-en-v1.5`) — sentence-embedding model,
  384-dimensional, used for local semantic search. License: **MIT**.
  Source: https://huggingface.co/BAAI/bge-small-en-v1.5
  The ONNX weights are fetched at build time (`make fetch-model`) and embedded
  in the binary; they are not redistributed in this source tree.

## Native runtime libraries

- **ONNX Runtime** (`microsoft/onnxruntime`) — inference engine, loaded at
  runtime as a shared library and shipped inside release archives. License:
  **MIT**. Source: https://github.com/microsoft/onnxruntime
- **sqlite-vec** (`asg017/sqlite-vec`) — `vec0` virtual table for vector search,
  statically linked. License: **Apache-2.0 OR MIT**.
  Source: https://github.com/asg017/sqlite-vec
- **tokenizers** (`daulet/tokenizers`, HuggingFace `tokenizers`) —
  `libtokenizers.a`, linked statically at build time. License: **Apache-2.0**.
  Source: https://github.com/daulet/tokenizers

## Go modules

Key Go dependencies and their licenses:

- `github.com/yalue/onnxruntime_go` — MIT (Go bindings for ONNX Runtime)
- `github.com/asg017/sqlite-vec-go-bindings` — Apache-2.0 OR MIT
- `github.com/daulet/tokenizers` — Apache-2.0
- `github.com/mattn/go-sqlite3` — MIT
- `github.com/spf13/cobra` — Apache-2.0

See `go.mod` for the full dependency set and pinned versions.

// Package embed turns text into dense vectors entirely on-device, with no
// network access. It runs the bge-small-en-v1.5 sentence model (ONNX, 384
// dimensions, CPU) via onnxruntime, tokenizing with the model's own Hugging
// Face tokenizer for parity.
//
// embed is a pure leaf: it depends on neither store, index, nor cli, so the
// downstream vector store, index embedding, and semantic search layers can all
// build on the Embedder interface in isolation.
//
// Two outputs differ only by a retrieval instruction prefix: Embed encodes
// documents verbatim, while EmbedQuery prepends the bge query instruction so a
// search string lands in the same space as the passages it should match. Both
// pool the [CLS] token (position 0) of the final hidden state and L2-normalize,
// so a dot product is a cosine similarity.
//
// Two native libraries back this package: onnxruntime (loaded at runtime via a
// shared library) and the tokenizers static library (linked at build time).
// See the Makefile for fetching the model and wiring CGO_LDFLAGS.
package embed

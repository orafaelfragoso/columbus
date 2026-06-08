// Package embed turns text into dense vectors entirely on-device, with no
// network access. It runs the minishlab/potion-code-16M Model2Vec model (256
// dimensions, CPU) with a pure-Go tokenizer and safetensors reader.
//
// embed is a pure leaf: it depends on neither store, index, nor cli, so the
// downstream vector store, index embedding, and semantic search layers can all
// build on the Embedder interface in isolation.
//
// Embed and EmbedQuery both encode text in the same static Model2Vec space.
// Outputs are L2-normalized, so a dot product is a cosine similarity. The model
// assets are fetched by the Makefile and embedded into the binary at build time.
package embed

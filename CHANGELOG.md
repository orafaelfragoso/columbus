# Changelog

All notable changes to Columbus will be documented in this file.

This project follows [Conventional Commits](https://www.conventionalcommits.org/),
and released changes are grouped by GoReleaser from commit history.

## [Unreleased]

No unreleased changes yet.

## [0.2.1] - 2026-06-08

The dashboard polish release. Columbus now has a two-tab TUI with a metrics-led
main view, a dedicated kanban work view, semantic-search result drilldown, and
safer quit handling.

### Features

- **tui:** split the dashboard into Main and Work tabs with highlighted header tabs
- **tui:** add embeddings, memories, epics, stories, and tasks metric cards
- **tui:** replace the old work summary cards with a full-height memory table
- **tui:** add a kanban view for epics, stories, and tasks
- **search:** show ranked semantic-search results with snippets in result detail
- **tui:** add a quit confirmation modal for `q`

### Fixes

- **tui:** make selected memory rows and kanban cards use the active panel border color
- **tui:** move kanban selection up and down within the active column
- **tui:** move kanban selection left and right between status columns
- **tui:** show only kanban card titles and wrap long titles instead of truncating them

## [0.2.0] - 2026-06-08

The semantic-search release. Columbus now retrieves with **on-device embeddings**
(bge-small-en-v1.5 via ONNX) and a vector store, then re-ranks with the existing
deterministic heuristics — no cloud, no LLM calls. The command surface was
reshaped around the new lifecycle.

### ⚠ BREAKING CHANGES

- The CLI was redesigned around the semantic lifecycle. `init` → `install`,
  `index` → `reindex`, `ui` → `view`; epics/stories/tasks/context/tags are now
  unified under `memory <add|update|remove|list> <kind>`; `import`/`export` are
  top-level; `search --graph` is replaced by the `graphs` command. Global
  `--limit`/`--depth` flags replace per-command equivalents.

### Features

- **embed:** on-device ONNX embedding engine (`Embedder`: Embed/EmbedQuery/Dim/Model)
- **store:** sqlite-vec (vec0) vector store with cosine distance + migration 0003
- **index:** chunk and embed symbols and files during indexing
- **search:** vector kNN-first retrieval with keyword fallback and deterministic re-rank
- **search:** locate-first results with opt-in code bodies
- **work:** story tier (epic → story → task) and an embedded durable-knowledge layer
- **cli:** redesigned command surface around the semantic lifecycle

### Build

- bundle onnxruntime per target and verify the native chain (goreleaser, CI smoke job)

### Docs

- update README for semantic search and the redesigned CLI

[0.2.1]: https://github.com/orafaelfragoso/columbus/releases/tag/v0.2.1
[0.2.0]: https://github.com/orafaelfragoso/columbus/releases/tag/v0.2.0
[0.1.0]: https://github.com/orafaelfragoso/columbus/releases/tag/v0.1.0

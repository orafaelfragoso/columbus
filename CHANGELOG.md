# Changelog

All notable changes to Columbus will be documented in this file.

This project follows [Conventional Commits](https://www.conventionalcommits.org/),
and released changes are grouped by GoReleaser from commit history.

## [0.2.3](https://github.com/orafaelfragoso/columbus/compare/v0.2.2...v0.2.3) (2026-06-09)


### Features

* **index:** embed non-code file content for search ([2af24c7](https://github.com/orafaelfragoso/columbus/commit/2af24c7829a8e371fb13644340d23e75f1452d69))
* **memory:** add note and reminder memory kinds ([8ac1956](https://github.com/orafaelfragoso/columbus/commit/8ac1956f2f320a2200597edb6a7b005207a67ba9))
* **tui:** show tags column in memories table ([df0b901](https://github.com/orafaelfragoso/columbus/commit/df0b9019818190b4e3b032909f6680f34671a779))


### Bug Fixes

* **tui:** show all memories in the table, not just the first 12 ([9be2766](https://github.com/orafaelfragoso/columbus/commit/9be2766132367c241c173f3de059e1e4774711f8))

## [0.2.2](https://github.com/orafaelfragoso/columbus/compare/v0.2.1...v0.2.2) (2026-06-09)


### Bug Fixes

* remove unused TUI helpers ([a6eb9f7](https://github.com/orafaelfragoso/columbus/commit/a6eb9f7f2dcbf20011d5781a63dac793d4048464))

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

# Columbus

[![CI](https://github.com/rafaelfragoso/columbus/actions/workflows/ci.yml/badge.svg)](https://github.com/rafaelfragoso/columbus/actions/workflows/ci.yml)
[![Release](https://github.com/rafaelfragoso/columbus/actions/workflows/release.yml/badge.svg)](https://github.com/rafaelfragoso/columbus/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rafaelfragoso/columbus.svg)](https://pkg.go.dev/github.com/rafaelfragoso/columbus)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A **local-only, deterministic code-context server** that a code agent calls as
a tool. Columbus does exactly three things:

1. **Index** the codebase (embedded tree-sitter).
2. **Search** — return LLM-ready context to the caller.
3. **Memory** — own the project's durable memory, including structured memory
   (epics & tasks).

Everything else — orchestration, hooks, guardrails, verification — lives in the
agent/plugin, not here. There are no LLM calls: all ranking, "why relevant"
text, and risk hints come from deterministic heuristics.

## Design invariants

- **Data always reflects current project state.** The database stores
  **metadata + git anchors only** — never code lines or file bodies. Snippets
  and exact line ranges are reconstructed **live** at query time by re-parsing
  the working tree with tree-sitter.
- **The DB is a metadata + graph cache**, not a content store.
- **`git` is the only hard runtime dependency.** `ripgrep` is the recommended
  search fast-path; a pure-Go fallback covers the rest. `ast-grep` is optional.
- **`--json` is a versioned API contract** with a canonical error envelope.

## Install

```sh
brew install rafaelfragoso/columbus/columbus   # pulls in ripgrep
```

Or build from source (Go 1.26+, a C compiler for cgo):

```sh
make build      # -> dist/columbus  (always built with -tags fts5, CGO_ENABLED=1)
```

## Quick start

```sh
columbus init                       # mint project_id, write .columbus.json (git-excluded)
columbus index                      # incremental index (also --full/--changed/--clean/--status)
columbus search "parse config"      # ranked, LLM-ready results
columbus search NewServer --graph   # include 1-hop imports/imported-by
columbus show symbol Engine --in internal/search
columbus show file internal/store/store.go
columbus doctor                     # environment + project health
```

### Memory

```sh
columbus memory add --kind decision --title "Use WAL" --body "readers never block writers" \
  --evidence internal/store/store.go:30-40 --link symbol:Open --tag db
columbus memory list
columbus memory search journal
columbus memory validate            # evidence drift + link resolution (warnings, never fatal)
columbus memory export --out knowledge.json   # unified doc: memories + epics + tasks
columbus memory import knowledge.json
```

### Epics & tasks

Structured memory: a passive, durable record of work (status, history, comments,
references) alongside memories. Columbus *stores and retrieves* — it never
drives, gates, or enforces transitions. `status` is just a recorded field from a
fixed vocabulary: `todo`, `in_progress`, `blocked`, `done`, `cancelled` (any →
any; no order enforced).

```sh
columbus epic add --title "Ship search" --tag infra
columbus task add --epic epic_001 --title "Index FTS"
columbus task status task_001 --to in_progress --comment "started"
columbus task comment task_001 --text "blocked on tokenizer choice"
columbus epic ref epic_001 --file internal/search/search.go --memory mem_003
columbus epic list --status in_progress
columbus task list --epic epic_001
columbus show epic epic_001          # fields, refs (inline drift), history, child tasks
columbus show task task_001
columbus epic validate               # reference drift scan (warnings, never fatal)
columbus search "search" --kind epic # epics/tasks are searchable (also in --kind all)
columbus epic delete epic_001 --force   # destructive; cascades child tasks; id retired
```

Each epic/task carries an append-only event log (every status change and
comment), a denormalized current status for fast `list --status`, and
drift-checked references to indexed `file`/`dir`/`memory`/`symbol` targets.
`show file|symbol|memory` lists in reverse the epics and tasks that reference
that entity ("what work touches this?").

## Output modes

`text` (default, human; color only on a TTY), `--json` (machine contract), and
`--llm` (markdown) are pure projections of the same typed result — they can
never silently diverge. Color follows `--no-color`, then `NO_COLOR`,
`FORCE_COLOR`, `TERM=dumb`, and `CI`, in that order, before falling back to TTY
detection.

### Exit codes

| code | meaning |
|---|---|
| 0 | success (incl. "usable with warnings") |
| 1 | runtime error |
| 2 | usage error |
| 3 | not initialized / index missing |
| 4 | transient / retryable (e.g. index writer locked) |

## Languages (V1)

TypeScript + TSX, JavaScript + JSX, Python, Go, Markdown. Adding a language is
a grammar + `.scm` queries + an extension mapping — no core changes.

## Development

```sh
make test       # go test -tags fts5 ./...
make vet
golangci-lint run ./...
make cover
```

See [CHANGELOG.md](CHANGELOG.md) for release history.

## License

MIT — see [LICENSE](LICENSE).

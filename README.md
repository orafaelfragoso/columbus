<div align="center">

<img src=".github/columbus.jpg" alt="Columbus — local-only context server for LLM agents" width="100%" />

<br />

**The navigator your coding agent has been missing.**

[![CI](https://github.com/orafaelfragoso/columbus/actions/workflows/ci.yml/badge.svg)](https://github.com/orafaelfragoso/columbus/actions/workflows/ci.yml)
[![Release](https://github.com/orafaelfragoso/columbus/actions/workflows/release.yml/badge.svg)](https://github.com/orafaelfragoso/columbus/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/orafaelfragoso/columbus.svg)](https://pkg.go.dev/github.com/orafaelfragoso/columbus)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

📖 **[Documentation & Guides](https://github.com/orafaelfragoso/columbus/wiki)** · [Quick Start](https://github.com/orafaelfragoso/columbus/wiki/Quick-Start) · [Using with your agent](https://github.com/orafaelfragoso/columbus/wiki/Using-Columbus-with-Your-Agent)

</div>

Your coding agent is brilliant at reasoning and bad at locating and remembering.
So it greps in the dark, reads whole files to use ten lines, and leans on stale
`.md` context that quietly lies.

**Columbus** is a **local-only, deterministic code-context server** your agent
calls as a tool. Ask it where something is and get ranked, LLM-ready context
with **exact line ranges** — reconstructed live from your working tree, so it's
**never stale**. It also owns your project's durable memory, so the agent stops
re-discovering the codebase every session.

_No embeddings. No cloud. No LLM calls. Same query, same answer, every time._

> [!IMPORTANT]
> **It cannot go stale, because it never stores your code.** The database holds
> **metadata and git anchors only** — every snippet and exact line range is
> rebuilt live at query time by re-parsing the working tree. The answer always
> matches the code as it is *right now*.

Columbus does exactly three things:

1. **Index** — chart the codebase with embedded tree-sitter (metadata + git
   anchors only, never your code).
2. **Search** — return ranked, LLM-ready context with exact line ranges,
   optionally with the 1-hop dependency graph.
3. **Memory** — own the project's durable record: decisions, plus structured
   epics & tasks with history, references, and drift checks.

There are no LLM calls: all ranking, "why relevant" text, and risk hints come from deterministic heuristics.

## Why Columbus

The agent is great at the thinking. It's the *finding* and *remembering* that
bleed tokens and go wrong. Columbus takes that off its plate.

| Without Columbus | With Columbus |
|---|---|
| Greps and reads whole files to find ten relevant lines | One call returns ranked context with **exact line ranges** |
| Different exploration path every run | **Deterministic** — same query, same answer |
| Context drifts; stale `.md` files confidently lie | Snippets rebuilt **live** from the working tree — never stale |
| Re-discovers the codebase every session | **Durable memory** — decisions, epics & tasks persist |
| Repo cluttered with `.cursorrules` / scattered context files | Memory is queryable and **git-excluded**, not committed noise |
| Embeddings shipped to a cloud, per-query cost | **Local-only**, zero LLM calls, nothing leaves your machine |

> [!NOTE]
> **Proof, measured.** A with/without study (tokens to first correct location,
> total session tokens, tool calls, run-to-run variance) is in progress — real
> numbers will land here. _Placeholder; not yet published._

## Install

Requires Go 1.26+ and a C compiler (cgo). Columbus is always built with
`-tags fts5` and `CGO_ENABLED=1`.

### `go install`

```sh
CGO_ENABLED=1 go install -tags fts5 github.com/orafaelfragoso/columbus/cmd/columbus@latest
```

Installs `columbus` into `$(go env GOPATH)/bin` — make sure that's on your `PATH`.

### Build from source

```sh
brew install zig goreleaser ripgrep ast-grep
make install      # -> dist/columbus  (always built with -tags fts5, CGO_ENABLED=1)
```

`git` is the only hard runtime dependency. `ripgrep` is the recommended search
fast-path (a pure-Go fallback covers the rest); `ast-grep` is optional.

## Quick start

```sh
columbus init                    # mint project_id, write .columbus.json (git-excluded)
columbus index                   # chart the codebase (incremental)
columbus search "parse config"   # ranked, LLM-ready context with exact line ranges
columbus ui                      # full-screen dashboard (index, memory, epics, tasks, graph)
```

Then point your agent at Columbus as a tool (see
[Use it with your agent](#use-it-with-your-agent)). The agent stops grepping and
starts asking; `columbus ui` gives *you* the live view of what it's doing.

## Documentation

Full guides and reference live in the **[Columbus Wiki](https://github.com/orafaelfragoso/columbus/wiki)**:

- [Your First Index & Search](https://github.com/orafaelfragoso/columbus/wiki/Your-First-Index-and-Search) — guided tour with real output
- [Using Columbus with Your Agent](https://github.com/orafaelfragoso/columbus/wiki/Using-Columbus-with-Your-Agent) — the skills model and `--json`/`--llm` contract
- [Never Stale: Live Reconstruction](https://github.com/orafaelfragoso/columbus/wiki/Never-Stale-Live-Reconstruction) — why answers always match current code
- [Command Reference](https://github.com/orafaelfragoso/columbus/wiki/Command-search) · [Configuration](https://github.com/orafaelfragoso/columbus/wiki/Configuration) · [FAQ](https://github.com/orafaelfragoso/columbus/wiki/FAQ)

## How it works

The **chart** (the index) tells Columbus *where* things are. The **working
tree** tells it *what they currently say*. Because Columbus never caches the
"what," it can never lie about it.

```
your working tree ──(tree-sitter)──▶ index: metadata + git anchors   (the chart)
        │                                          │
        │   agent: columbus search "parse config"  │
        ▼                                          ▼
   re-parse live ◀───────────────────────  rank deterministically
        │
        ▼
   ranked results + EXACT line ranges + "why relevant"  ──▶  LLM-ready
```

Indexing is incremental and cheap, but even a stale index can't produce a stale
*answer*: the snippet and line range you get back are reconstructed from the
file as it exists at query time.

## Use it with your agent

Columbus is a tool, not an autopilot. Your agent learns *when* and *how* to call
it through **skills** — small instruction files that teach the agent the CLI
(when to `search` before editing, how to record a decision, how to read the
graph). The skills live in the **agent/plugin layer**, never in this binary,
which keeps Columbus a small, deterministic context server with no opinion on
your workflow.

The contract the agent consumes:

- **`--json`** — a versioned, machine-readable contract with a canonical error
  envelope. Stable to parse, safe to depend on.
- **`--llm`** — markdown shaped for a context window: ranked results, exact line
  ranges, and a short "why relevant" per hit.

Both are pure projections of the same typed result as the human `text` output —
they can never silently diverge.

```sh
columbus search "where do we parse config" --llm    # drop straight into context
columbus search NewServer --graph --json            # ranked hits + 1-hop imports, machine-readable
```

> [!NOTE]
> **Columbus skills** (for Claude Code and other agents) are published separately
> as plugin assets. _Link coming soon._

## Commands

### Search & navigate

```sh
columbus search "parse config"      # ranked, LLM-ready results
columbus search NewServer --graph   # include 1-hop imports/imported-by
columbus show symbol Engine --in internal/search
columbus show file internal/store/store.go
columbus show graph --json          # whole dependency graph as {nodes, edges}
columbus show graph --role impl --in internal/store   # narrow + induce subgraph
columbus index                      # also --full/--changed/--clean/--status
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

### Dashboard

`columbus ui` opens a full-screen, read-mostly terminal dashboard over the
indexed project: index freshness and counts, durable memory, epics & tasks
(with task roll-up progress), and the dependency-graph hubs. It auto-refreshes,
so external `columbus index`/agent edits appear on their own.

Keys: `tab` switch pane · `↑/↓` (or `j/k`) navigate · `enter` detail (full body,
refs, history) · `/` search across code, memory, epics & tasks · `esc` back ·
`r` refresh · `R` reindex (in-process) · `?` help · `q` quit. It is a projection
of the same data the JSON/LLM commands expose; only `R` writes (it runs the
indexer) — epics/tasks/memory are read-only.

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

## Design invariants

- **Data always reflects current project state.** The database stores
  **metadata + git anchors only** — never code lines or file bodies. Snippets
  and exact line ranges are reconstructed **live** at query time by re-parsing
  the working tree with tree-sitter.
- **The DB is a metadata + graph cache**, not a content store.
- **`git` is the only hard runtime dependency.** `ripgrep` is the recommended
  search fast-path; a pure-Go fallback covers the rest. `ast-grep` is optional.
- **`--json` is a versioned API contract** with a canonical error envelope.

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

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md). For security reports, see
[SECURITY.md](SECURITY.md).

## License

MIT — see [LICENSE](LICENSE).

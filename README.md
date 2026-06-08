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

**Columbus** is a **local-only, semantic code-context server** your agent calls
as a tool. Ask it — in plain language — where something is and get ranked,
LLM-ready context with **exact line ranges**, reconstructed live from your
working tree so it's **never stale**. It also owns your project's durable
memory, so the agent stops re-discovering the codebase every session.

_Local embeddings, no cloud, no LLM calls — natural-language search that stays on
your machine._

> [!IMPORTANT]
> **It cannot go stale, because it never stores your code.** The database holds
> **metadata, git anchors and embedding vectors only** — every snippet and exact
> line range is rebuilt live at query time by re-parsing the working tree. The
> vectors are a derived index, not your source; the answer always matches the
> code as it is *right now*.

Columbus does exactly three things:

1. **Index** — chart the codebase with embedded tree-sitter and embed each
   symbol/file on-device (metadata + git anchors + vectors, never your code).
2. **Search** — natural-language semantic search: vector retrieval re-ranked by
   deterministic heuristics, returning ranked context with exact line ranges.
3. **Memory** — own the project's durable record: decisions, plus structured
   epics → stories → tasks with history, references, and drift checks.

Embeddings run **on-device** with bundled Model2Vec assets
(`minishlab/potion-code-16M`); there are no LLM calls and nothing leaves your
machine. Ranking, "why relevant" text, and risk hints are deterministic.

## Why Columbus

The agent is great at the thinking. It's the *finding* and *remembering* that
bleed tokens and go wrong. Columbus takes that off its plate.

| Without Columbus | With Columbus |
|---|---|
| Greps for the exact word and misses the concept | **Natural-language** semantic search finds it by meaning |
| Reads whole files to find ten relevant lines | One call returns ranked context with **exact line ranges** |
| Context drifts; stale `.md` files confidently lie | Snippets rebuilt **live** from the working tree — never stale |
| Re-discovers the codebase every session | **Durable memory** — decisions, epics, stories & tasks persist |
| Repo cluttered with `.cursorrules` / scattered context files | Memory is queryable and **git-excluded**, not committed noise |
| Embeddings shipped to a cloud, per-query cost | **On-device** embeddings, zero LLM calls, nothing leaves your machine |

> [!NOTE]
> **Proof, measured.** A with/without study (tokens to first correct location,
> total session tokens, tool calls, run-to-run variance) is in progress — real
> numbers will land here. _Placeholder; not yet published._

## Install

Requires Go 1.26+ and a C compiler for the embedded tree-sitter grammars.
Columbus is built with `-tags fts5`. The SQLite, vector, and embedding stack is
pure Go, so no ONNX, tokenizer, or SQLite native libraries are required.

### Release archives

Download the archive for your platform from
[Releases](https://github.com/orafaelfragoso/columbus/releases). Each archive
ships a single `columbus` binary with the model assets embedded, so local
natural-language (vector) search works out of the box with no network at
runtime.

### Build from source

```sh
brew install ripgrep ast-grep
make setup        # fetch Model2Vec assets into internal/embed/assets
make install      # built with -tags fts5 and CGO_ENABLED=1 by default
```

`git` is the only hard runtime dependency. `ripgrep` is the recommended search
fast-path (a pure-Go fallback covers the rest); `ast-grep` is optional. The
SQLite metadata store and `vec0` vector search use the pure-Go modernc driver;
the embedding engine uses a pure-Go Model2Vec runtime and embedded safetensors
weights.

## Quick start

```sh
columbus install                 # onboard: write .columbus.json, create db, first index + embed
columbus search "parse config"   # ranked, LLM-ready context with exact line ranges
columbus reindex                 # re-chunk + re-embed only what changed
columbus view                    # full-screen dashboard (main memory view + work Kanban)
columbus doctor                  # verify git, vec0, model runtime + index health
```

Then point your agent at Columbus as a tool (see
[Use it with your agent](#use-it-with-your-agent)). The agent stops grepping and
starts asking; `columbus view` gives *you* the live view of what it's doing.

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

Search is **locate-first**: by default it returns ranked locations, signatures,
scores and graph edges — the cheap "where to look" map — and omits code bodies.
Add `--snippets` to attach bodies inline (capped with `--snippet-lines N`), or
pull a specific body on demand with `columbus show symbol`. Exact line ranges are
always present, so an agent can read first and drill down only where it matters.

```sh
columbus search "where do we parse config" --llm             # ranked locations, no bodies (cheap)
columbus search "where do we parse config" --llm --snippets  # same, with code bodies inline
columbus graphs --in internal/server --json                  # dependency graph as {nodes, edges}, machine-readable
```

> [!NOTE]
> **Columbus skills** (for Claude Code and other agents) are published separately
> as plugin assets. _Link coming soon._

## Commands

### Lifecycle

```sh
columbus install                    # onboard: write config, create db, first index + embed
columbus reindex                    # re-chunk + re-embed changes (also --full/--changed/--clean/--status)
columbus doctor                     # environment + project health (git, vec0, runtime, model, index)
columbus uninstall                  # remove config + delete the db (confirm; --yes when non-TTY)
columbus purge                      # clear all records + reset config to defaults (confirm; --yes)
```

### Search & navigate

```sh
columbus search "where do we parse config"    # natural-language, ranked, LLM-ready results
columbus search "auth token check" --kind all # code + memory + epics/stories/tasks
columbus show symbol Engine --in internal/search
columbus show file internal/store/store.go
columbus graphs --json                        # whole dependency graph as {nodes, edges}
columbus graphs --role impl --in internal/store   # narrow + induce subgraph
```

Search is semantic: the query is embedded on-device and matched by vector
similarity, then re-ranked by deterministic heuristics. With no runtime library
present it degrades to keyword (FTS) ranking — `columbus doctor` shows which.

### Memory (durable knowledge)

One surface over every durable-knowledge kind —
`epic`, `story`, `task`, `context` (free-form decisions/patterns/…), and `tag`
(read-only). All of it is embedded and shows up in semantic `search`.

```sh
columbus memory add context --type decision --title "Use WAL" --body "readers never block writers" \
  --evidence internal/store/store.go:30-40 --ref symbol:Open --tag db
columbus memory add epic  --title "Ship search" --tag infra
columbus memory add story --parent epic_001 --title "Indexing"
columbus memory add task  --parent story_001 --title "Index FTS"
columbus memory update task task_001 --status in_progress --comment "started"
columbus memory update epic epic_001 --add-ref file:internal/search/search.go
columbus memory list epic --status in_progress
columbus memory list task --parent story_001
columbus memory list tag                        # distinct tags + counts
columbus memory remove epic epic_001 --force    # destructive; cascades stories+tasks; id retired
```

The work hierarchy is **epic → story → task**: a passive, durable record (status,
append-only history, comments, references). Columbus *stores and retrieves* — it
never drives, gates, or enforces transitions. `status` is a recorded field from a
fixed vocabulary: `todo`, `in_progress`, `blocked`, `done`, `cancelled` (any →
any). References are drift-checked against indexed
`file`/`dir`/`memory`/`symbol` targets; `show file|symbol|memory` lists in
reverse the work that references that entity ("what touches this?").

```sh
columbus show epic epic_001          # fields, refs (inline drift), history, child stories/tasks
columbus show task task_001
columbus search "search" --kind epic # work items are searchable (also in --kind all)
```

### Import / export

```sh
columbus export --out knowledge.json   # full knowledge doc: memories + epics + stories + tasks
columbus import knowledge.json         # vectors are not exported — reindex rebuilds them
```

### Dashboard

`columbus view` opens a full-screen, read-mostly terminal dashboard over the
indexed project. The Main tab shows index freshness, file/symbol/embedding
counts, memory counts, epic/story/task counts, and a full-width memory table. The
Work tab is a Kanban board for epics, stories, and tasks, grouped by status. It
auto-refreshes, so external `columbus reindex`/agent edits appear on their own.

Keys: `tab` switch Main/Work · `←/→` cycle epics/stories/tasks in Work · `↑/↓`
(or `j/k`) navigate · `enter` detail (full body, refs, history) · `/` semantic
search across code, memory and work (ranked results, snippets in detail) ·
`esc` back · `r` refresh · `R` reindex (in-process) · `?` help · `q` quit
confirmation. It is a projection of the same data the JSON/LLM commands expose;
only `R` writes (it runs the indexer) — work/memory are read-only.

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

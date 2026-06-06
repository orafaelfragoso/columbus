# Columbus CLI — Implementation Plan (v1)

> Status: design locked via grilling session (2026-06-06). This document is the source of truth for the V1 build. Source of the user-facing surface: `GRILL.md`.

---

## 1. Purpose & scope boundary

Columbus is a **local-only, deterministic context server** that a code agent (the Claude Code plugin) calls as a tool. It has exactly three jobs:

1. **Index** the codebase (embedded tree-sitter).
2. **Search** — return LLM-ready context to the caller.
3. **Memory** — own and control the project's durable memory.

Everything else — orchestration, hooks, commands, skills, agents, guardrails, verification, isolation — lives **plugin-side**, not in this CLI. Do **not** add `run/worker/worktree/verify/preflight/guardrail` logic to this binary.

The database is an **implementation detail**, never part of the product surface. There are no `db` commands.

---

## 2. Core invariants (the non-negotiables that fell out of the design)

1. **Data is always trusted; it always reflects current project state.** The DB stores **metadata + git anchors only** — never code lines, never file bodies, never line numbers as truth. Snippets and exact line ranges are reconstructed **live** at query time by re-parsing the working tree with tree-sitter.
2. **The DB is a metadata + graph cache**, not a content store. It exists for speed, relationships, and staleness detection.
3. **Single hard runtime dependency: `git`.** `rg` is recommended (fast-path); `ast-grep` is optional (opt-in enhancer). The CLI is fully functional with only git present (pure-Go fallbacks cover the rest).
4. **Deterministic, no LLM calls.** All ranking, "why relevant" text, and `risk_level` are produced by deterministic heuristics.
5. **Typed result → projections.** Each command produces one typed Go value; `text` / `--json` / `--llm` are pure projections of it and can never silently diverge.
6. **The `--json` output is an API contract** consumed by the plugin: versioned (`schema_version`), with a canonical error envelope.

---

## 3. Tech stack

| Concern | Choice | Notes |
|---|---|---|
| Language | Go 1.26 | cgo enabled (`CGO_ENABLED=1` always) |
| CLI framework | `spf13/cobra` | nested subcommands, help, completions; **no viper** |
| Parsing | embedded **tree-sitter** (cgo) | sole required structural engine |
| SQLite driver | `mattn/go-sqlite3` + `-tags fts5` | FTS5 BM25 over metadata |
| Search index | SQLite **FTS5** (metadata only) | code bodies are NOT indexed |
| Content search | `rg` fast-path + **pure-Go gitignore-aware fallback** | git is the only hard dep |
| Structural (opt-in) | `ast-grep` | optional; extra language coverage / custom patterns |
| Log rotation | `gopkg.in/natefinch/lumberjack` | size-capped |
| Cross-platform dirs | `adrg/xdg` or tiny hand-rolled resolver | + `COLUMBUS_DATA_DIR` override |
| Release | `goreleaser` + **`zig cc`** | single-pipeline cgo cross-compile |

### Dependency surface (runtime)
- **Required:** `git`.
- **Recommended:** `ripgrep` (`rg`) — declared as a Homebrew formula dep; runtime-optional everywhere else.
- **Optional:** `ast-grep` — opt-in only.
- **Embedded (no install):** tree-sitter + bundled grammars, SQLite.

---

## 4. Project layout

```
columbus/
  cmd/columbus/main.go        # thin entry: build root cmd, Execute, map error->exit code
  internal/
    cli/                      # cobra commands (thin: parse flags -> domain -> render)
    config/                   # .columbus.json load/validate, precedence, data-dir resolver
    project/                  # project_id, git anchoring, file-set selection
    store/                    # sqlite open, PRAGMAs, migrations, repositories
    index/                    # walk, change-detection, parse orchestration, extractors
    extract/                  # per-language LanguageExtractor + .scm queries + shared IR
    search/                   # candidate generation, ranking, result assembly
    memory/                   # memory domain: records, evidence, links, validation, export/import
    render/                   # text / json / llm projections of typed results
    logging/                  # slog JSON handler + lumberjack
  testdata/                   # fixture sources; git repos built at test setup (no committed .git)
```

- **No `pkg/`.** Columbus is a binary, not a Go library. `internal/` enforces this.
- **Thin command handlers.** No business logic in `internal/cli`; commands call a domain function and hand the typed result to `internal/render`. This keeps everything testable without invoking cobra.

---

## 5. Data model (SQLite — metadata + graph + git anchors only)

Normalized; paths stored once in `files` and referenced by id everywhere.

- **`index_meta`** — singleton-ish: `schema_version` (also via `PRAGMA user_version`), `project_id`, `indexed_head`, `dirty`, stats, `mem_seq` counter.
- **`files`** — `id`, `path` (unique), `language`, `package`, `role` (impl/test/doc/other), `blob_oid` (tracked) / `content_sha256` (untracked), `grain_eligible`.
- **`symbols`** — `id`, `file_id`, `name`, `kind` (shared enum), `container`, `signature`, `exported`, plus a **structural identity** (path + name + kind + container) used for stable reference. (Line ranges are **not** authoritative; resolved live.)
- **`imports`** / **`exports`** — raw specifiers tied to `file_id` (best-effort resolution).
- **`dep_edges`** — resolved file→file edges (best-effort).
- **`test_links`** — impl_file_id ↔ test_file_id.
- **`todos`** — `file_id`, location reference, text (reference, re-validated live).
- **`memories`** — `id` (`mem_NNN`), `kind` (enum), `title`, `body`, timestamps.
- **`memory_tags`** — `memory_id`, `tag`.
- **`memory_evidence`** — `memory_id`, `path`, `line_start`, `line_end`, `blob_oid_at_creation`.
- **`memory_links`** — `memory_id`, `target_type` (`file`|`symbol`), `target_ref` (path, or name+qualifier).
- **`code_fts`** (FTS5) — metadata only: `name`, `signature`, `path`, `package`; `grain` = `symbol`|`file`.
- **`memory_fts`** (FTS5) — `title`, `body`, `tags`.

### Shared symbol-kind enum
`function`, `method`, `class`, `interface`, `type`, `const`, `var`, `enum`, … normalized across languages.

### Memory-kind enum (fixed)
`decision`, `pattern`, `failure`, `command`, `glossary`. `add`/`edit` reject unknown kinds (`INVALID_KIND`, exit 2).

---

## 6. I/O contract

### Streams
- **stdout = payload only.** In `--json`, stdout is *pure JSON* (directly `JSON.parse`-able).
- **stderr = diagnostics** (progress, human warnings, logs).
- In machine modes, warnings travel **in-band** in the JSON (`warnings: [...]`), never only on stderr.

### Output modes
- **text** (default, human) — color/styling **only when stdout is a TTY**; honor `NO_COLOR` + `--no-color`.
- **`--json`** — versioned machine contract.
- **`--llm`** — markdown projection of the same typed result.
- **Format is never auto-switched by TTY.** Text unless `--json`/`--llm` is passed explicitly.

### JSON envelope
```jsonc
// success
{ "ok": true, "schema_version": 1, "command": "search", /* ...payload... */ }
// error
{ "ok": false, "schema_version": 1, "command": "search",
  "error": { "code": "INDEX_MISSING", "message": "...", "hint": "run columbus index" } }
```
- Struct tags drive JSON; never hand-build JSON strings. `json.Encoder` with `SetEscapeHTML(false)`.

### Exit codes
| code | meaning |
|---|---|
| 0 | success (incl. "usable with warnings") |
| 1 | runtime error |
| 2 | usage error |
| 3 | not initialized / index missing (recoverable state the plugin branches on) |

### `error.code` → exit table
| code | exit | when |
|---|---|---|
| `USAGE` | 2 | bad flags/args |
| `CONFIG_INVALID` | 2 | `.columbus.json` invalid |
| `INVALID_KIND` | 2 | memory kind not in enum |
| `NOT_INITIALIZED` | 3 | no `.columbus.json` |
| `INDEX_MISSING` | 3 | operation needs an index, none exists |
| `INDEX_LOCKED` | 1 | writer-lock contention |
| `SCHEMA_TOO_NEW` | 1 | DB newer than binary |
| `NOT_FOUND` | 1 | file/symbol/memory not found |
| `STORE_ERROR` | 1 | SQLite/IO failure |
| `DEPENDENCY_MISSING` | 1 | required dep (git) absent |

---

## 7. Project identity & storage

- **`project_id`** generated once at `init` (`proj_` + 8 random hex), stored in `.columbus.json` (the source of truth). Decoupled from path and git remote.
- **`.columbus.json` is local-only.** `init` adds it to **`.git/info/exclude`** (idempotent; no-op outside a git repo; never touches tracked `.gitignore`). Output notes `.columbus.json (git-excluded locally)`.
- **Data directory** (OS-specific), holding `projects/<project_id>/columbus.sqlite`, `logs.jsonl`, `exports/`:
  - Linux: `$XDG_DATA_HOME` or `~/.local/share/columbus`
  - macOS: `~/Library/Application Support/columbus`
  - Windows: `%LocalAppData%\columbus`
  - Override: **`COLUMBUS_DATA_DIR`** (essential for tests).
- Two clones of one repo each `init` their own id (local file) → no collision.
- Whole-DB backup/dump/import is **deferred** (post-V1).

---

## 8. `.columbus.json` config & precedence

Minimal file: `schema_version`, `project_id`, and an **indexing config** block (`include`/`exclude` globs, `max_file_size`, per-language enable/disable). **Not** stored: ranking weights (compiled constants in V1), detection results (recomputed each run).

**Precedence (low → high):** built-in defaults → `.columbus.json` → environment (`COLUMBUS_*`, `NO_COLOR`) → command-line flags.

Validation on load: unknown keys → warning (forward-compat); invalid values → error (`CONFIG_INVALID`, exit 2).

---

## 9. Indexing

### Languages (V1 bundled grammars)
TypeScript + TSX, JavaScript + JSX, Python, Go, Markdown.

### Extractor architecture
- A `LanguageExtractor` per language, all producing one **shared IR** (symbols, imports/exports, todos).
- Driven by per-language tree-sitter **`.scm` query files**; a registry maps **extension → grammar + query set**.
- **Adding a language = grammar + queries + extension mapping; zero core changes.**
- Language-agnostic post-processing derives `role` (path heuristics: `*.test.*`, `__tests__/`, `_test.go`) and `package` (nearest `package.json`/`go.mod`/`pyproject.toml`).
- **Import resolution is best-effort:** resolve relative + same-package imports; record unresolved specifiers as-is (no edge). Surface resolution coverage in `index` stats. (tsconfig-paths / node_modules resolution deferred.)

### Modes
- **`index` (default)** = incremental: diff current working-tree state vs indexed state; (re)index added/changed, drop deleted. Covers committed + uncommitted.
- **`--full`** = reindex everything from scratch (memories preserved).
- **`--changed`** = fast path: only files dirty in the working tree.
- **`--clean`** = drop all index data; preserve `.columbus.json` + memories + memory links.
- **`--status`** = report only, no writes.

### Change detection
- **Per-file content hash** is the change key: git **blob OID** (tracked) / **SHA-256** (untracked). Reindex iff hash differs.
- **indexed HEAD** + **dirty** flag stored for `--status`/`doctor` staleness reporting (not the primary signal).

### File set
- In git: `git ls-files` ∪ `git ls-files --others --exclude-standard` (`.gitignore` honored for free; new untracked files index).
- Outside git: filesystem walk honoring `.gitignore`-style rules + Columbus excludes.
- Skip binaries (content sniff) and oversized files (configurable, default ~1–2 MB); counted in skipped stats.

---

## 10. Search

Two-source pipeline, unified by a deterministic scorer:

1. **Metadata match (in-DB, fast).** FTS5 BM25 over metadata (symbol names, signatures, paths, packages, todos, memory title/body/tags).
2. **Live content match (working tree).** `rg` (fast-path) or pure-Go fallback greps actual files now → `file:line` hits; tree-sitter parses those files live to map each hit to its enclosing symbol + **current** line range + snippet text.
3. **Merge + enrich.** Dedupe to one result per symbol/file; pull graph edges (imports / imported_by / tests) and relevant memories from the DB; rank; render.

- `memory search` / `--kind memory` = pure FTS5. `--kind code` = both sources. Default = all.
- `--graph` (V1) = **1-hop** expansion over stored edges (direct neighbors). Multi-hop / `--depth` deferred.

### Ranking (deterministic; candidate-generation ≠ scoring)
- FTS5 + rg are **candidate generators only**; their internal scores never reach the final number.
- Each candidate deduped to **one result object** carrying all signals.
- **Single feature function → `score ∈ [0,1]`** with named, tunable, compiled-constant weights:
  - symbol-name match (exact > prefix > substring) — highest
  - signature / identifier match
  - path / filename match
  - content-match density (rg/Go hits within range)
  - graph centrality (`imported_by` count, has-tests)
  - memory-linkage boost
  - role weighting: **implementation > test by default** (tests still surface in `Tests:`; boosted when query hits test identifiers)
- **`score` documented as relative ranking, not absolute confidence.**
- **"why relevant"** = templated from the dominant feature(s) — no prose generation.
- **`risk_level`** = crude heuristic (elevate when highly central or has a linked `failure` memory); documented as a hint.
- Embeddings (`--embedding`) deferred: a future semantic-similarity feature plugs into the **same** function.

### `show` semantics
- `show symbol <name>`: **show all matches**, each as its own block (capped, with a note); `--in <path>` narrows.
- `show file <path>`: exact path; not-found → `NOT_FOUND` with did-you-mean suggestions from the index.
- `show memory <id>`: by id.
- `--context-lines N` (search + show): pad N lines around the matched range; **default 3**, `0` = exact range.

---

## 11. Memory subsystem

- **IDs:** monotonic per-project integer, `mem_%03d` (grows to `mem_1042`); counter in `index_meta` (safe under the writer lock). **Never reused** — deleted IDs retired.
- **Kinds:** fixed enum (`decision`/`pattern`/`failure`/`command`/`glossary`); unknown → `INVALID_KIND`.
- **Evidence:** git-anchored reference (`path` + line range + `blob_oid_at_creation`). `validate`/`doctor` compare to current tree: unchanged → ✓; changed → **stale** (warning); missing → **broken** (warning).
- **Links:** `file:` (resolves to indexed file) / `symbol:` (name + optional path/container qualifier). **Resolved at read time** against the live index — no cascade on reindex. Unresolvable → stored **with a warning** (warnings-not-errors), so you can link ahead.
- Memories are **timeless** (not tied to a commit); only evidence/links carry git anchors for drift.
- **Drift is always a warning, never hard-invalid** — a memory stays usable and listed even if evidence drifted.

### Commands
- **`add`** — `--kind --title --body --evidence path:start-end --link file:… --link symbol:… --tag …`.
- **`edit <id>`** — partial via flags (`--title --body --kind --add-tag/--remove-tag --add-evidence/--remove-evidence --add-link/--remove-link`); ≥1 change required; re-index FTS row.
- **`remove <id>`** — hard delete (cascade evidence/links/tags/FTS); ID retired; **no interactive prompt** (agent-friendly).
- **`link <id>`** — add links to existing memory (same validation/warning model).
- **`list`** — `--kind`/`--tag` filters; summary counts by kind.
- **`search`** — pure FTS5 over `memory_fts`.
- **`validate`** — records/IDs/evidence/links; warnings for drift.
- **`export`** — schema-versioned JSON (`{schema_version, memories:[record+evidence+links+tags]}`); **stdout by default**, `--out <path>` (suggested dir: data-dir `exports/`); `--kind`/`--tag` filters.
- **`import`** — reads path or stdin. **Default merge = reassign new local IDs + content-hash dedupe** (safe, idempotent, never collides). **`--preserve-ids`** for same-project backup/restore into an empty store (errors on collision).

---

## 12. Concurrency model

Each `columbus …` is a **separate OS process** → primary concern is cross-process access to one SQLite file.

- **WAL mode** — readers (`search`/`show`) never block during an `index` write; they see the last committed snapshot.
- PRAGMAs: `synchronous=NORMAL`, `foreign_keys=ON`, `busy_timeout` (~5s).
- **`index` is one atomic transaction** — all-or-nothing; a crash rolls back; never a half-built index. `search` during indexing sees the previous committed snapshot (correct semantics, not staleness).
- **Writer exclusivity:** writes open with **`BEGIN IMMEDIATE`**; on contention, wait up to `busy_timeout` then fail with **`INDEX_LOCKED`** (exit 1) rather than hang.
- **In-process parallelism:** worker pool parses files concurrently (one tree-sitter parser per worker — not reentrant), feeding a **single serialized DB writer** batching into the one transaction.

---

## 13. Migrations

- **Hand-rolled, embedded, ordered** migrations keyed to `PRAGMA user_version`; SQL embedded via `embed.FS`; applied on open inside a transaction.
- **Auto-migrate on open** (safe: local, single-user, disposable cache).
- DB `user_version` newer than binary → **`SCHEMA_TOO_NEW`** error (no guessing).
- No `golang-migrate`/`goose`.

---

## 14. Logging

- Per-project **`logs.jsonl`** in the data dir, via `slog` JSON handler; distinct from stderr (stderr = per-invocation human diagnostics).
- Entry: timestamp (injected clock), level, `command`, `project_id`, message, fields.
- **Default (info):** writes + significant ops — `index` runs (mode/counts/duration/result), memory mutations, migrations, **all errors + warnings**.
- **Reads (`search`/`show`/`list`) excluded by default** (agent loops are high-frequency); logged only at `debug` (`COLUMBUS_LOG_LEVEL=debug`).
- **Rotation:** `lumberjack`, size-capped (~10 MB), keep 1–2 files.

---

## 15. Testing strategy (TDD)

- **DI for determinism:** inject **clock**, **ID generator** (deterministic `mem_NNN`/`proj_`), and **data-dir resolver**. Domain code never reaches `time.Now()`/random directly.
- **Pyramid:**
  - **Pure-logic unit tests** (bulk): ranking function, change-detection/hash, extractor IR, memory validation, the three render projections.
  - **Store integration:** real SQLite + FTS5 against `t.TempDir()` via `COLUMBUS_DATA_DIR`.
  - **Index integration:** real tree-sitter over fixture sources; **fixture repos built at test setup** (`git init` + write + commit; no committed `.git`); also exercise the non-git path.
  - **E2E:** **in-process primary** (call root cmd `Execute` with args, capture buffers, assert JSON + exit code) **+ a thin binary-smoke set** (catches `main()` wiring and `-tags fts5`).
- **Golden files** for big render outputs (text/llm/json) with `-update`, **normalizing** non-deterministic fields (`project_id`, abs paths, durations) before compare. Logic uses explicit assertions.
- **Mutation testing (`gremlins`) deferred to post-V1**, then targeted at ranking / change-detection / memory-validation.

---

## 16. Distribution

- **Targets (V1):** `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`. **Windows deferred.**
- `CGO_ENABLED=1` always.
- **Cross-compile via `zig cc`** (single Linux pipeline; compiles sqlite + tree-sitter C across all four targets).
- **`goreleaser`:** archives, checksums, version injection via `-ldflags` (`version`/`commit`/`date` → `columbus --version` + `doctor`), Homebrew tap publishing.
- **Homebrew tap** with `depends_on "ripgrep"`; `ast-grep` left optional; git assumed present.

---

## 17. V1 scope

**In V1:**
- `init`, `doctor`
- `index` (`--full` / `--changed` / `--clean` / `--status`)
- `search` (text / `--json` / `--llm`, `--limit`, `--context-lines`, `--kind code|memory`, **`--graph` 1-hop**)
- `show file` / `show symbol` / `show memory`
- `memory add` / `edit` / `remove` / `list` / `search` / `link` / `validate`
- **`memory export` / `import`** (record portability)

**Deferred (post-V1):**
- `search --embedding` (semantic)
- `search --graph` multi-hop / `--depth N`
- Whole-DB backup/dump/import
- ast-grep deep integration (beyond opt-in)
- Pure-Go rg fallback may ship after the rg path
- Windows target
- Mutation testing
- tsconfig-paths / node_modules import resolution

---

## 18. Build sequence (vertical slices, PR-sized)

Each slice ends in a working, tested state. TDD throughout.

1. **Skeleton + I/O contract.** cobra root, `cmd/columbus`, render layer (typed-result → text/json/llm with `schema_version` + error envelope), exit-code mapping, TTY/color, DI for clock/id/datadir. E2E harness (in-process) + binary smoke. *(No domain logic yet — `--version` + a stub command prove the contract.)*
2. **Store + migrations.** SQLite open, PRAGMAs (WAL etc.), embedded `user_version` migrations, base schema, repositories. Store-integration tests on temp DB.
3. **`init` + config + project identity.** `.columbus.json` create/load/validate, `project_id`, `.git/info/exclude`, data-dir resolver, precedence chain.
4. **Extractors + shared IR.** `LanguageExtractor` registry + `.scm` queries for the 5 grammars; symbol/import/export/todo extraction to IR. Pure parse tests over fixtures.
5. **`index` core.** File-set selection (git ∪ untracked − ignored, non-git fallback), content-hash change detection, parallel-parse/serial-write, atomic transaction, modes (default/full/changed/clean/status), stats. `index` integration tests.
6. **`doctor`.** All checks; usable-vs-warning semantics; exit codes.
7. **Search — metadata path.** FTS5 over metadata, ranking feature function, result assembly + graph enrichment (1-hop), three render modes. `--kind`, `--limit`, `--context-lines`.
8. **Search — live content path.** rg fast-path + pure-Go fallback, tree-sitter live line-range/snippet resolution, dedupe/merge into the scorer.
9. **`show file` / `symbol` / `memory`.** Disambiguation (show-all + `--in`), did-you-mean, context-lines.
10. **Memory subsystem.** Tables, IDs, kinds, evidence/link reference model, `add/edit/remove/list/search/link/validate`, memory FTS, drift warnings.
11. **Memory export / import.** JSON format, reassign+dedupe default, `--preserve-ids`.
12. **Logging.** `logs.jsonl` + lumberjack; wire info/debug levels across commands.
13. **Distribution.** goreleaser + zig-cc pipeline, ldflags version, Homebrew tap.

---

## 19. Notes / things to revisit

- `risk_level` is the fuzziest field — crude in V1, expect iteration.
- Ranking weights are compiled constants in V1; expose in `.columbus.json` only once stable.
- Pure-Go content-search fallback can lag the rg path in sequencing if needed, but is required before claiming "git-only" operation.
- `ast-grep` opt-in role (extra languages vs custom structural patterns) to be specified when implemented; it must never become required.

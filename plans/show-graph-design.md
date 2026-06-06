# `columbus show graph` — resolved design

Outcome of a grill-me session. This is the shared understanding to plan against,
not an implementation plan. Source of truth for the decisions below.

## One-line

`columbus show graph` projects the **already-indexed** file dependency graph as a
typed `GraphResult`, rendered through the existing `--json` / `text` / `--llm`
lanes. Primary consumer: the agent (canonical `{nodes, edges}` JSON).

## Decisions (10)

1. **Consumer** — agent context. `--json` `{nodes, edges}` is the canonical
   contract; `text`/`--llm` are summaries.
2. **Granularity** — file-level only. Nodes = indexed files. Symbol-level call
   graph is explicitly out of scope (no call edges exist in schema/extractor).
3. **Edges** — everything: `import` (from `dep_edges`), `test` (from
   `test_links`), and external edges (from `imports WHERE resolved_file_id IS NULL`).
4. **Externals** — collapse specifier to package root (npm scope-aware, Go
   module-ish, Python top-level), reusing `internal/index/resolve.go` heuristics.
   Companion rule: a specifier starting with `.` or `/` that didn't resolve is a
   relative miss → **dropped**, not an external. Only bare specifiers become
   `ext:` nodes.
5. **Scope/scale** — whole graph by default; `--in` / `--role` / `--lang`
   filters; node cap with `Total` + `Capped` truncation signal (mirrors
   `show symbol`). (`--root`/`--depth` = possible fast-follow, not v1.)
6. **Freshness** — read edges from DB cache (like `show file`); emit
   `indexed_head` / `dirty` / `last_indexed_at` (or derived `stale`) in the
   contract. No live whole-tree re-resolve.
7. **Subgraph semantics** — drop dangling edges (clean induced subgraph). Cap by
   **in-degree descending, path tiebreak** (deterministic; reuses the
   `imported_by` centrality notion in `rank.go`).
8. **Contract shape** — string ids: file id = path, external id = `ext:<pkg>`
   (namespaced so kinds never collide). Medium-rich file nodes:
   `{id, kind, role, language, package, in_degree, out_degree, has_tests}`.
   External nodes: `{id, kind}`. Edges: `{from, to, type}`, directed
   (import: importer→imported; test: impl→test), deduped.
9. **Projections** — `text` and `--llm` are summary projections: counts by
   kind/type, top-N hubs (in-degree), top-N external deps, files-without-tests,
   freshness line. `--json` carries full node/edge arrays. Mermaid/DOT = possible
   later `--format`, not v1.
10. **Scope boundary** — in scope. Pure read-projection over indexed metadata,
    same category as `show file`'s graph section and `search --graph`. No
    orchestration/LLM/guardrails — that line stays uncrossed.

## Determinism

`nodes` sorted by `id`; `edges` sorted by `(from, to, type)`. Cap survivors by
`(in_degree desc, path asc)`. No nondeterministic ordering anywhere (honors the
"deterministic" core promise).

## Build surface

- **store**: new read(s) next to `graph_repo.go` — resolved edges (`dep_edges`),
  test links (`test_links`), and unresolved imports with specifiers; plus degree
  counts. `AllFiles()` already exists.
- **graph/show**: a `Shower.Graph(opts)` (or small `internal/graph` builder)
  producing `GraphResult` — collapse externals, apply filters, build induced
  subgraph, cap, compute degrees, attach freshness.
- **result type**: `GraphResult` + node/edge types with the three `Render*`
  projections, alongside `SymbolResult`/`FileResult` in `internal/show`.
- **cli**: `newShowGraphCmd` registered in `show.go` with `--in/--role/--lang`
  and a cap flag; reuses `withShower`.

## Out of scope (v1)

Symbol-level call graph · `--root`/`--depth` rooted subgraphs · Mermaid/DOT
`--format` · live whole-tree re-resolution · external nodes for relative misses.

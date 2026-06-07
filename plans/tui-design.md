# Columbus TUI — design decision

A local, fullscreen terminal UI (`columbus ui`) for humans who prefer a visual
view of project state. This is a **secondary-audience** feature: Columbus's
primary consumer is an agent, and this view is for human inspection, onboarding,
and demos — not on the agent's critical path. Decided consciously.

## Decision (resolved via grill-me)

| Fork | Choice | Why |
| --- | --- | --- |
| TUI vs web | **TUI (Bubble Tea)** | Fits the one-shot, single-binary, local-only, no-network identity. Dies with the session; ships in the same binary. |
| Graph view | **Dropped** | The only feature that would have forced a browser (TUI can't do interactive node-link layout). Removing it removes the web pressure entirely. |
| Deployment | **Local-only** | No auth/multi-tenant/persistence. Behind a `columbus ui` subcommand, never the default. |
| Liveness | **Both**: auto-refresh dashboard + live progress on triggered index | Auto-refresh is pure polling (zero backend change). Triggered-index progress needs a new callback in `internal/index`. |
| Write surface | **Index-trigger only** (`g`) | Epics/tasks are view-only; edits stay in the CLI/agent. Smallest write surface, cleanest concurrency story, no scope-boundary tension. |
| Index execution | **In-process** | `columbus ui` imports `internal/index` and calls it; progress arrives via callback → `tea.Cmd` → `tea.Msg`. Avoids a subprocess + machine-readable progress format; better cancellation. |

## Views (v1)

1. **Epics & tasks** — list + detail, status, child tasks, refs/drift, history.
   Read-only. Data already exists (`work_repo`: `ListEpics`/`ListTasks`,
   `EpicFull`/`TaskFull`, `WorkEvents`).
2. **Index dashboard** — freshness panel (indexed head, dirty/clean,
   last-indexed-at, file/symbol counts) from `store.Meta`. `g` triggers an
   in-process index and streams `file N of M` as a progress bar.

No graph view.

## Architecture principles

- **Pure `Model`, dumb `View`.** All logic lives in `Update(Msg) (Model, Cmd)`
  and plain data functions — both unit-testable without rendering. `View()` is a
  thin renderer with no logic. This is how the TUI stays compatible with the
  repo's hard-TDD discipline: test the model transitions, not the pixels.
- **Auto-refresh via `tea.Tick`** (~1s poll re-reading meta + work rows). No
  fsnotify in v1 — polling is simpler and the data is cheap to re-read.
- **Progress callback is in-memory.** `index.Run` gains an `onProgress
  func(Progress)` that reports a counter held in memory. It must NOT re-read the
  DB inside the index `WithTx` — that deadlocks the single connection
  (`SetMaxOpenConns(1)`, see store memory).
- **Concurrency is already handled** by the store: WAL (readers never block the
  writer cross-process), `busy_timeout=5000` (the `g`-index waits rather than
  erroring if an agent holds the write lock).
- **New deps** (build-only, for an optional subcommand): `charmbracelet/bubbletea`,
  `lipgloss`, `bubbles`. First UI-framework dependency in the repo; acceptable
  because it's optional and off the agent path, but note the deviation from
  "git is the only hard dependency."

## Implemented architecture (`internal/tui` + `columbus ui`)

The feature shipped as a feature-focused dashboard over real data. The charm
stack is used per component:

- **Bubble Tea** — Elm-style root `Model` (`model.go`): `Init`/`Update`/`View`,
  all logic in `Update` + pure helpers so it's testable without a terminal.
- **Lip Gloss** — central `theme.go` (palette, `panel`/`card`/`bar`, width
  helpers); the header, cards, memory & graph panels, and modals.
- **Bubbles** — `table` (epics + tasks, focus/scroll/selection; the tasks table
  re-syncs to the selected epic), `viewport` (scrollable detail), `help` + `key`
  (footer generated from the same `keyMap` that handles input), `spinner`
  (async refresh).
- **Huh** — the `/` filter form, embedded as a `tea.Model` (nil unless active).
- **Glamour** — renders the selected epic/task as markdown in the detail pane.

**Data port (the seam):** `data.go` defines an immutable `Snapshot` +
`Source` interface; `source.go`'s `StoreSource` adapts `*store.DB` +
`*memory.Manager` (reads `Meta`, `ListEpics`, `ListTasks`, `AllDepEdges`/
`AllTestLinks` for hubs/edges, `memory.List`). Epic progress is **derived** from
the task roll-up (`done/total`) — epics have no percentage field. Tests use a
fake `Source` (model) and a seeded real store (adapter).

### Done

- ✅ `Source`/`Snapshot` port + `StoreSource` adapter (tested against a seeded store)
- ✅ Bubble Tea model: tab focus, table nav, epic→tasks sync, `/` filter (huh),
  `enter` detail (viewport + glamour), `r` refresh (spinner), `?` help, `q` quit
- ✅ Full-height/width responsive layout; `View` tested across sizes
- ✅ `columbus ui` command (+ `COLUMBUS_UI_PRINT` headless frame); README
- ✅ **`esc`** robustly backs out of any mode (cancel search / close detail /
  clear filter) — keyed on `tea.KeyEsc` directly, ahead of huh's own handling
- ✅ Header **branch** via `gitrepo.Info.Branch()` (best-effort, cosmetic)
- ✅ Live **auto-refresh** via `tea.Tick` (silent 2s reload; skipped while
  loading/reindexing/searching)
- ✅ **`R` in-process reindex** via `tea.Cmd` (`WithReindex` option → cli wires an
  `index.Indexer`); spinner + "reindexing…" header, reloads on completion
- ✅ Richer **detail** via the `DetailSource` port (`EpicFull`/`TaskFull` + tags,
  refs, history), with the Snapshot summary as fallback
- ✅ **`/` global search** across code, memory, epics & tasks (`WithSearch` →
  `search.Engine` with `KindAll`), results in a scrollable modal with
  kind-colored tags and locations (replaces the old title-only filter)
- ✅ Panel **spacing**: blank line between each title and its content, and blank
  rows between stacked sections

### Remaining (smaller follow-ups)

1. `feat(index)` — per-file **progress** (`onProgress` callback through the index
   pipeline) so `R` shows `N/M` rather than an indeterminate spinner.
2. Memory **detail** (currently epics/tasks only) via `memory.Get`.

## Effort / impact

- **Effort:** the feature is built and tested end-to-end. The only genuinely new
  backend piece left is the index progress callback (deferred — indeterminate
  spinner works today).
- **Impact:** real but for humans (inspection, onboarding, demo), not the agent
  loop. Eyes-open secondary-audience investment.

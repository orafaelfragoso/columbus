# Dashboard: migrate to Charm v2 + idiomatic components

Goal: move `internal/tui` from Charm v1 to v2 and fix the layout/interaction
issues that the hand-rolled v1 layer caused.

## Dependency swap (module path is `charm.land/*`, not github)

- `charm.land/bubbletea/v2 v2.0.7`
- `charm.land/lipgloss/v2 v2.0.3`   (adds `Canvas`/`Layer` compositing)
- `charm.land/bubbles/v2 v2.1.0`
- `charm.land/glamour/v2 v2.0.0`

## v2 API deltas that touch us

- `View() string` → `View() tea.View` (`tea.NewView(s)`, set `AltScreen`).
- `tea.KeyMsg` → `tea.KeyPressMsg` (`{Code: tea.KeyEnter}`, `.String()=="enter"`,
  shift+tab via `{Code: tea.KeyTab, Mod: tea.ModShift}`).
- `tea.NewProgram(m)` — no `WithAltScreen`; alt screen is a `View` field.
- `lipgloss.Color` is now a func returning `image/color.Color`; palette vars and
  helper signatures change from `lipgloss.Color` to `color.Color`.
- `table`/`viewport`/`textinput`/`spinner`/`help`/`list` move to `/v2` paths;
  `spinner.Tick` is a method value (still usable as a `tea.Cmd`).

## Fixes delivered with the migration

1. **Overlay modals** — render the dashboard as a background layer and
   `Compose` the search/results/detail box as a centered `Layer` on top via
   `lipgloss.Canvas`, instead of replacing the whole screen with `lipgloss.Place`.
2. **Navigable results** — replace the scroll-only results viewport with a
   `bubbles/v2/list`; ↑/↓ select, enter opens the selected hit's detail.
3. **Table selection feedback** — explicit `Selected` style (bg + bright fg) and
   keep the focused pane's table `Focus()`d.
4. **Border/width clipping** — clamp every panel body line to the panel inner
   width so a too-wide table can never eat the right border or the count column.
5. **Refresh tick no longer wipes modals** — skip the background reload while a
   search/results/detail modal is open.

## Verify

`go test -tags fts5 ./...`, `-race`, `go vet`, `gofmt`, `golangci-lint`, and a
headless `COLUMBUS_UI_PRINT` render at narrow + wide widths.

# Contributing to Columbus

Thanks for your interest in improving Columbus! This document covers everything
you need to get a change merged.

## Code of Conduct

This project adheres to a [Code of Conduct](CODE_OF_CONDUCT.md). By
participating you agree to uphold it.

## Project scope

Columbus does exactly three things: **index**, **search**, and **memory**.
Orchestration, hooks, guardrails, and verification belong in the agent/plugin
layer, not here. There are **no LLM calls** — all ranking, "why relevant" text,
and risk hints are deterministic heuristics. Please keep proposals within this
boundary; see [README.md](README.md) for the design invariants.

## Development setup

You need **Go 1.26+** and a **C compiler** (cgo is always on — tree-sitter and
SQLite are C). `ripgrep` is recommended (the search fast-path); `git` is the
only hard runtime dependency.

```sh
git clone https://github.com/rafaelfragoso/columbus
cd columbus
make build       # -> dist/columbus
make install     # -> ~/.local/bin/columbus (override with PREFIX=/usr/local/bin)
```

`cgo` and the `fts5` build tag are mandatory everywhere. The `Makefile` pins
both, so always go through it (or remember `-tags fts5 CGO_ENABLED=1`).

## The workflow

This project follows **Test-Driven Development**. Every change to production
code should be driven by a failing test first.

1. **RED** — write a failing test that describes the desired behavior.
2. **GREEN** — write the minimum code to make it pass.
3. **REFACTOR** — improve the design while tests stay green.

Test behavior through the public API, not implementation details.

Before opening a PR, the following must pass locally:

```sh
make test        # go test -tags fts5 ./...
make vet         # go vet -tags fts5 ./...
golangci-lint run ./...
gofmt -l .       # must print nothing
```

CI runs all of these on every pull request.

## Commit messages — Conventional Commits

Commits **must** follow the [Conventional Commits](https://www.conventionalcommits.org/)
spec. The PR title is linted against it, and release notes are generated from
commit history.

```
<type>(<optional scope>): <description>

[optional body]

[optional footer(s)]
```

Allowed types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`,
`ci`, `chore`, `revert`.

Examples:

```
feat(search): add --graph 1-hop enrichment
fix(store): avoid deadlock when reading inside WithTx
docs: document the I/O contract exit codes
```

A `feat:` triggers a minor bump, `fix:`/`perf:` a patch bump, and a
`BREAKING CHANGE:` footer (or `!` after the type) a major bump.

## Pull requests

1. Fork and branch from `main`.
2. Keep changes small and focused — one logical change per PR.
3. Add or update tests for any behavior change.
4. Fill out the PR template.
5. Ensure CI is green.

## Releasing (maintainers)

Releases are cut by pushing a semver tag. CI (goreleaser) builds the
cross-compiled binaries, checksums, GitHub release notes, and the Homebrew tap
bump automatically.

```sh
git tag v1.2.0
git push origin v1.2.0
```

## License

By contributing, you agree that your contributions are licensed under the
project's [MIT License](LICENSE).

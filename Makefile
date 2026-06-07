# Columbus build/test conventions. cgo + the fts5 build tag are always on.

TAGS := fts5
CGO_ENABLED := 1
export CGO_ENABLED

# Where `make install` drops the binary. Override with `make install PREFIX=/usr/local/bin`.
PREFIX ?= $(HOME)/.local/bin

# Version metadata stamped into the binary via -ldflags (mirrors goreleaser).
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build install uninstall test test-short vet fmt lint cover clean release-check release-test

build:
	go build -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o dist/columbus ./cmd/columbus

install:
	@mkdir -p '$(PREFIX)'
	go build -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o '$(PREFIX)/columbus' ./cmd/columbus
	@echo "installed columbus $(VERSION) -> $(PREFIX)/columbus"
	@command -v columbus >/dev/null 2>&1 || echo "warning: $(PREFIX) is not on your PATH"

uninstall:
	rm -f '$(PREFIX)/columbus'

test:
	go test -tags '$(TAGS)' ./...

test-short:
	go test -tags '$(TAGS)' -short ./...

vet:
	go vet -tags '$(TAGS)' ./...

fmt:
	gofmt -w .

cover:
	go test -tags '$(TAGS)' -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

clean:
	rm -rf dist coverage.out coverage.html

# Validate the goreleaser config without building anything.
release-check:
	goreleaser check

# Dry-run the full cross-compile release locally: same builds, archives, and
# checksums CI produces, but no tag/publish required. Reproduces the CI zig
# cross-compile (artifacts land in dist/). Needs goreleaser + zig on PATH.
release-test:
	goreleaser release --snapshot --clean

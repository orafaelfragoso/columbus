# Columbus build/test conventions. Tree-sitter still requires cgo; the embedding
# and SQLite/vector stack no longer need native libraries.

TAGS := fts5
CGO_ENABLED ?= 1
export CGO_ENABLED

# Where `make install` drops the binary. Override with `make install PREFIX=/usr/local/bin`.
PREFIX ?= $(HOME)/.local/bin

# Version metadata stamped into the binary via -ldflags (mirrors goreleaser).
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# --- Embedding model assets ---
#
# The embed package uses a pure-Go Model2Vec runtime. These Hugging Face files
# are embedded into the binary via //go:embed. CI, release, and local setup all
# run fetch-model.
MODEL_DIR   := internal/embed/assets
MODEL_REPO  := minishlab/potion-code-16M
MODEL_BASE  := https://huggingface.co/$(MODEL_REPO)/resolve/main
MODEL_FILES := config.json model.safetensors tokenizer.json

.PHONY: build install uninstall test test-short vet fmt lint cover clean release-check release-test fetch-model setup

fetch-model:
	@mkdir -p '$(MODEL_DIR)'
	@for f in $(MODEL_FILES); do \
		echo "fetching $(MODEL_REPO)/$$f -> $(MODEL_DIR)/$$f"; \
		curl -fSL --retry 3 "$(MODEL_BASE)/$$f" -o "$(MODEL_DIR)/$$f"; \
	done
	@du -h $(MODEL_FILES:%=$(MODEL_DIR)/%) | sed 's/^/done /'

setup: fetch-model
	@echo "ready: model assets are staged in $(MODEL_DIR)"

# build fails loudly with a fetch hint when the embedded model is missing,
# rather than emitting an opaque //go:embed error.
build:
	@for f in $(MODEL_FILES); do test -f "$(MODEL_DIR)/$$f" || { echo "error: $(MODEL_DIR)/$$f missing -> run 'make fetch-model'"; exit 1; }; done
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
# checksums CI produces, but no tag/publish required. Needs goreleaser + zig.
release-test:
	goreleaser release --snapshot --clean

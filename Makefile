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

# --- Embedding engine (internal/embed) native dependencies ---
#
# The embed package needs two native libs that are NOT vendored here (bundling
# is spec 7):
#
#   1. tokenizers static lib (libtokenizers.a), linked at BUILD time. Point the
#      linker at its directory via CGO_LDFLAGS, e.g.:
#          export CGO_LDFLAGS="-L$$HOME/.columbus/libs"
#      Prebuilt libs: https://github.com/daulet/tokenizers/releases/latest
#
#   2. onnxruntime shared lib, loaded at RUN time. Set COLUMBUS_ORT_LIB to its
#      path (or drop it next to the binary), e.g.:
#          export COLUMBUS_ORT_LIB="$$HOME/.columbus/libs/libonnxruntime.dylib"
#      Releases: https://github.com/microsoft/onnxruntime/releases
#
# The model weights are fetched separately with `make fetch-model`.
MODEL_DIR  := internal/embed/assets
MODEL_FILE := $(MODEL_DIR)/model.onnx
MODEL_URL  := https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main/onnx/model.onnx

.PHONY: build install uninstall test test-short vet fmt lint cover clean release-check release-test fetch-model

# fetch-model downloads the bge-small-en-v1.5 ONNX weights into assets/. The
# weights are git-ignored (too large to commit); tokenizer.json sits beside
# them and is committed.
fetch-model:
	@mkdir -p '$(MODEL_DIR)'
	@echo "fetching model.onnx -> $(MODEL_FILE)"
	@curl -fSL --retry 3 '$(MODEL_URL)' -o '$(MODEL_FILE)'
	@echo "done ($$(du -h '$(MODEL_FILE)' | cut -f1))"

# build fails loudly with a fetch hint when the embedded model is missing,
# rather than emitting an opaque //go:embed error.
build:
	@test -f '$(MODEL_FILE)' || { echo "error: $(MODEL_FILE) missing -> run 'make fetch-model'"; exit 1; }
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

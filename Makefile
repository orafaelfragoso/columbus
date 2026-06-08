# Columbus build/test conventions. cgo + the fts5 build tag are always on.

TAGS := fts5
CGO_ENABLED := 1
export CGO_ENABLED

# Point the linker at the local native libs (libtokenizers.a). Override to use a
# system location. Exported so plain `go build`/`go test` invocations link too.
CGO_LDFLAGS ?= -L$(HOME)/.columbus/libs
export CGO_LDFLAGS

# The sqlite-vec cgo binding compiles sqlite-vec.c with -DSQLITE_CORE, whose
# header does `#include "sqlite3.h"`. mattn/go-sqlite3 statically compiles SQLite
# but names its header sqlite3-binding.h, so the bare sqlite3.h is only found via
# a system header — which is absent under zig cross-compiles (build fails) and is
# Apple's deprecated copy on macOS (noisy). Ship the matching official header and
# put it on the include path so every toolchain resolves it identically.
CGO_CFLAGS := -I$(CURDIR)/third_party/sqlite $(CGO_CFLAGS)
export CGO_CFLAGS

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

# Where the native libs land for local dev. libtokenizers.a is linked at build
# time (CGO_LDFLAGS=-L$(LIBS_DIR)); the onnxruntime shared lib is loaded at run
# time (COLUMBUS_ORT_LIB=$(LIBS_DIR)/<lib>). `make setup` populates both.
LIBS_DIR        ?= $(HOME)/.columbus/libs
# ORT 1.26.0 is the first release exposing the API version the Go binding needs.
ORT_VERSION     ?= 1.26.0
TOKENIZERS_TAG  ?= v1.27.0

# Host os/arch -> go slugs and upstream asset names. Override on cross targets.
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_S),Darwin)
  GO_OS    := darwin
  ORT_LINK := libonnxruntime.dylib
  ORT_LIB  := libonnxruntime.$(ORT_VERSION).dylib
  ifeq ($(UNAME_M),arm64)
    GO_ARCH := arm64
    TOK_SLUG := darwin-aarch64
  else
    GO_ARCH := amd64
    TOK_SLUG := darwin-x86_64
  endif
else
  GO_OS    := linux
  ORT_LINK := libonnxruntime.so
  ORT_LIB  := libonnxruntime.so.$(ORT_VERSION)
  ifeq ($(UNAME_M),aarch64)
    GO_ARCH := arm64
    TOK_SLUG := linux-aarch64
  else
    GO_ARCH := amd64
    TOK_SLUG := linux-x86_64
  endif
endif

TOK_URL := https://github.com/daulet/tokenizers/releases/download/$(TOKENIZERS_TAG)/libtokenizers.$(TOK_SLUG).tar.gz

.PHONY: build install uninstall test test-short vet fmt lint cover clean release-check release-test fetch-model fetch-ort fetch-tokenizers setup

# fetch-model downloads the bge-small-en-v1.5 ONNX weights into assets/. The
# weights are git-ignored (too large to commit); tokenizer.json sits beside
# them and is committed.
fetch-model:
	@mkdir -p '$(MODEL_DIR)'
	@echo "fetching model.onnx -> $(MODEL_FILE)"
	@curl -fSL --retry 3 '$(MODEL_URL)' -o '$(MODEL_FILE)'
	@echo "done ($$(du -h '$(MODEL_FILE)' | cut -f1))"

# fetch-tokenizers downloads the prebuilt libtokenizers.a for the host into
# $(LIBS_DIR). It is linked statically at build time via CGO_LDFLAGS.
fetch-tokenizers:
	@mkdir -p '$(LIBS_DIR)'
	@echo "fetching libtokenizers ($(TOK_SLUG) $(TOKENIZERS_TAG)) -> $(LIBS_DIR)"
	@curl -fSL --retry 3 '$(TOK_URL)' | tar -xz -C '$(LIBS_DIR)' libtokenizers.a
	@echo "done -> $(LIBS_DIR)/libtokenizers.a"

# fetch-ort downloads the onnxruntime shared lib for the host into $(LIBS_DIR)
# and symlinks the unversioned name the loader resolves. It is dlopen'd at run
# time (set COLUMBUS_ORT_LIB or drop it next to the binary).
fetch-ort:
	@echo "fetching onnxruntime ($(ORT_SLUG) $(ORT_VERSION)) -> $(LIBS_DIR)"
	@bash scripts/fetch-native.sh $(GO_OS) $(GO_ARCH) '$(LIBS_DIR)' >/dev/null
	@echo "done -> $(LIBS_DIR)/$(ORT_LINK)"
	@echo "export COLUMBUS_ORT_LIB=$(LIBS_DIR)/$(ORT_LINK)"

# setup fetches everything the embedding engine needs for a local semantic build
# (fetch-ort stages both libtokenizers.a and the onnxruntime shared lib).
setup: fetch-model fetch-ort
	@echo "ready: build with CGO_LDFLAGS=-L$(LIBS_DIR), run with COLUMBUS_ORT_LIB=$(LIBS_DIR)/$(ORT_LINK)"

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

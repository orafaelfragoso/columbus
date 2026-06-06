# Columbus build/test conventions. cgo + the fts5 build tag are always on.

TAGS := fts5
CGO_ENABLED := 1
export CGO_ENABLED

.PHONY: build test test-short vet fmt lint cover clean

build:
	go build -tags '$(TAGS)' -o dist/columbus ./cmd/columbus

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

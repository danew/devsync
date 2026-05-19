BINARY := devsync
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/danew/devsync/internal/buildinfo.Version=$(VERSION) -X github.com/danew/devsync/internal/buildinfo.Commit=$(COMMIT) -X github.com/danew/devsync/internal/buildinfo.Date=$(DATE)

.PHONY: build test install clean release-snapshot checksums

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/devsync

test:
	go test ./...

install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/devsync

clean:
	rm -rf bin dist checksums.txt

release-snapshot:
	goreleaser release --snapshot --clean

checksums: build
	shasum -a 256 bin/$(BINARY) > checksums.txt

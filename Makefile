.PHONY: build test check clean

VERSION ?= dev
REVISION ?= unknown
BUILD_TIME ?= unknown
GOCACHE ?= /tmp/sma-go-cache
LDFLAGS := -s -w -X sma/internal/buildinfo.Version=$(VERSION) -X sma/internal/buildinfo.Revision=$(REVISION) -X sma/internal/buildinfo.BuildTime=$(BUILD_TIME)

build:
	mkdir -p dist
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sma_linux_amd64 ./cmd/sma

test:
	GOCACHE=$(GOCACHE) go test ./...

check:
	test -z "$$(gofmt -l cmd internal)"
	GOCACHE=$(GOCACHE) go vet ./...
	GOCACHE=$(GOCACHE) go test -race ./...
	GOCACHE=$(GOCACHE) GOOS=linux GOARCH=amd64 go build ./...
	GOCACHE=$(GOCACHE) GOOS=linux GOARCH=arm64 go build ./...

clean:
	rm -rf dist

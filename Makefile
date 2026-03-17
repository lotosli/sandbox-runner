GO ?= go
BIN ?= sandbox-run
VERSION ?= dev
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -s -w -X 'github.com/lotosli/sandbox-runner/internal/cli.versionInfo.Version=$(VERSION)' -X 'github.com/lotosli/sandbox-runner/internal/cli.versionInfo.GitSHA=$(GIT_SHA)' -X 'github.com/lotosli/sandbox-runner/internal/cli.versionInfo.BuildTime=$(BUILD_TIME)'

.PHONY: build
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/sandbox-run

.PHONY: test
test:
	$(GO) test ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: dist
dist:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/sandbox-run-darwin-arm64 ./cmd/sandbox-run
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/sandbox-run-darwin-amd64 ./cmd/sandbox-run
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/sandbox-run-linux-amd64 ./cmd/sandbox-run
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/sandbox-run-linux-arm64 ./cmd/sandbox-run

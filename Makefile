.PHONY: build-cli test clean all

CLI_BINARY_NAME=photo-ai-cli.exe
GO=C:/Program\ Files/Go/bin/go.exe
GIT_COMMIT=$(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE=$(shell powershell -NoProfile -Command "Get-Date -Format 'yyyy-MM-dd_HH:mm:ss'")

# Simple build
all: build-cli

build-cli:
	@mkdir -p ./bin
	cd cmd/photo-ai-cli && $(GO) build -ldflags "-X main.gitCommit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)" -o ../../bin/$(CLI_BINARY_NAME)

test:
	$(GO) test ./...

clean:
	rm -rf ./bin

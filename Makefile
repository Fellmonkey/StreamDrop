# StreamDrop build entry points. The SPA must be built before go build so the
# embedded dist/ is current.

SHELL := /bin/bash
WEB := web
BIN := bin/streamdrop
BIN_MCP := bin/streamdrop-mcp
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_NO_V := $(VERSION:v%=%)

.PHONY: all web deps build build-mcp build-windows test test-web test-e2e vet lint depcheck run clean package
all: web build

web:
	cd $(WEB) && npm ci && npm run build

deps:
	go mod download

build: web
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o $(BIN) -ldflags "-X streamdrop/internal/config.Version=$(VERSION_NO_V)" ./cmd/streamdrop

build-mcp:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o $(BIN_MCP) ./cmd/streamdrop-mcp

# Reference platform: Windows. streamdrop.exe is a GUI app (no console); the
# MCP mode is a subcommand (streamdrop mcp) and works over inherited stdio
# pipes. build-mcp produces the standalone console variant for development.
build-windows: web
	mkdir -p bin
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
		-ldflags "-s -w -H=windowsgui -X streamdrop/internal/config.Version=$(VERSION_NO_V)" -o bin/streamdrop.exe ./cmd/streamdrop

test:
	go test ./...

test-web:
	cd $(WEB) && npm test

test-e2e:
	go test ./internal/e2e/ -run MailRu -v

vet:
	go vet ./...

depcheck:
	go run ./cmd/depcheck

run: build
	STREAMDROP_CONSOLE_LOG=1 ./$(BIN) --no-tray --no-browser

# Portable layout: the single streamdrop.exe (components auto-download on
# first run). Release CI uploads the executable itself per platform.
package: build-windows
	mkdir -p dist/StreamDrop
	cp bin/streamdrop.exe dist/StreamDrop/streamdrop.exe

clean:
	rm -rf bin dist
	rm -rf $(WEB)/dist
	mkdir -p $(WEB)/dist && touch $(WEB)/dist/.gitkeep

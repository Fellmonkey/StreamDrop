# StreamDrop build entry points. The SPA must be built before go build so the
# embedded dist/ is current.

SHELL := /bin/bash
WEB := web
BIN := bin/streamdrop
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all web deps build build-windows test test-web test-e2e vet lint depcheck run clean package

all: web build

web:
	cd $(WEB) && npm ci && npm run build

deps:
	go mod download

build: web
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o $(BIN) ./cmd/streamdrop

# Reference platform: Windows, no console window.
build-windows: web
	mkdir -p bin
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
		-ldflags "-s -w -H=windowsgui" -o bin/streamdrop.exe ./cmd/streamdrop

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

# Portable archive layout: binary, docs, and downloaded components.
package: build-windows
	mkdir -p dist/StreamDrop
	cp bin/streamdrop.exe dist/StreamDrop/
	cp README.md dist/StreamDrop/
	cp README.ru.md dist/StreamDrop/
	cp -r deps dist/StreamDrop/deps 2>/dev/null || true
	cd dist && rm -f StreamDrop-v1-windows-amd64.zip && \
		(command -v zip >/dev/null && zip -qr StreamDrop-v1-windows-amd64.zip StreamDrop || \
		 powershell -NoProfile -Command "Compress-Archive -Path StreamDrop -DestinationPath StreamDrop-v1-windows-amd64.zip -Force")

clean:
	rm -rf bin dist
	rm -rf $(WEB)/dist
	mkdir -p $(WEB)/dist && touch $(WEB)/dist/.gitkeep

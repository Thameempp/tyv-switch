BINARY    := tyv
MODULE    := github.com/thameem/tyv
VERSION   := 0.1.0
LDFLAGS   := -s -w -X main.version=$(VERSION)
INSTALL   := $(HOME)/.local/bin

.PHONY: build install uninstall test vet fmt clean cross-build checksums help

## build: compile the binary for the current platform
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/tyv/

## install: build and install to $(INSTALL) (no sudo required)
install: build
	@mkdir -p $(INSTALL)
	@cp $(BINARY) $(INSTALL)/$(BINARY)
	@chmod 755 $(INSTALL)/$(BINARY)
	@echo "Installed: $(INSTALL)/$(BINARY)"
	@echo "Run: tyv --help"

## uninstall: remove the binary from $(INSTALL)
uninstall:
	@rm -f $(INSTALL)/$(BINARY)
	@echo "Removed: $(INSTALL)/$(BINARY)"

## test: run tests with race detector
test:
	go test -race -v ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: check formatting
fmt:
	@if [ "$$(gofmt -l . | wc -l)" -gt 0 ]; then \
		echo "Files need formatting:"; gofmt -l .; exit 1; \
	else echo "gofmt: OK"; fi

## clean: remove built binaries
clean:
	@rm -f $(BINARY)
	@rm -rf dist/

## cross-build: build for all supported platforms
cross-build: clean
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/tyv-darwin-arm64  ./cmd/tyv/
	GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/tyv-darwin-amd64  ./cmd/tyv/
	GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/tyv-linux-amd64   ./cmd/tyv/
	GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/tyv-linux-arm64   ./cmd/tyv/
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/tyv-windows-amd64.exe ./cmd/tyv/
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/tyv-windows-arm64.exe ./cmd/tyv/
	@ls -lh dist/

## checksums: generate SHA-256 checksums for dist binaries
checksums:
	@cd dist && sha256sum * > SHA256SUMS && cat SHA256SUMS

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'

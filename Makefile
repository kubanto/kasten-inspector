BINARY  := kasten-inspector
VERSION := 1.2.0
LDFLAGS := -s -w -X main.version=$(VERSION)

# Generates go.sum and downloads all modules.
# GONOSUMDB=* skips checksum DB (avoids gopamath lookup).
# GOFLAGS=-mod=mod lets go update go.sum on the fly.
.PHONY: deps
deps:
	GONOSUMDB=* GOFLAGS=-mod=mod go mod download
	GONOSUMDB=* GOFLAGS=-mod=mod go mod tidy -e 2>/dev/null || true

# Build for current platform
.PHONY: build
build:
	GONOSUMDB=* GOFLAGS=-mod=mod CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/

# All platforms
.PHONY: all
all: dist linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64
	@echo ""
	@echo "All binaries in ./dist/"
	@ls -lh dist/

dist:
	mkdir -p dist

.PHONY: linux-amd64
linux-amd64: dist
	GONOSUMDB=* GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64    ./cmd/
	@echo "  ✓ dist/$(BINARY)-linux-amd64"

.PHONY: linux-arm64
linux-arm64: dist
	GONOSUMDB=* GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64    ./cmd/
	@echo "  ✓ dist/$(BINARY)-linux-arm64"

.PHONY: darwin-amd64
darwin-amd64: dist
	GONOSUMDB=* GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64   ./cmd/
	@echo "  ✓ dist/$(BINARY)-darwin-amd64"

.PHONY: darwin-arm64
darwin-arm64: dist
	GONOSUMDB=* GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64   ./cmd/
	@echo "  ✓ dist/$(BINARY)-darwin-arm64"

.PHONY: windows-amd64
windows-amd64: dist
	GONOSUMDB=* GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/
	@echo "  ✓ dist/$(BINARY)-windows-amd64.exe"

.PHONY: clean
clean:
	rm -rf dist/ $(BINARY) $(BINARY).exe go.sum

.PHONY: vet
vet:
	GONOSUMDB=* GOFLAGS=-mod=mod go vet ./...

BIN := bin/gh-tui
# Build as the release does (cli/gh-extension-precompile): without cgo,
# which gh-tui doesn't need, without local paths, and without the symbol
# table and DWARF, a quarter of the binary. Only the race detector needs cgo.
GOBUILD := CGO_ENABLED=0 go build
RELEASE_FLAGS := -trimpath -ldflags='-s -w'
# gopls runs analyzers that golangci-lint lacks, such as writestring and
# scannererr; CI runs this version.
GOPLS := go run golang.org/x/tools/gopls@v0.23.0

.PHONY: help build build-debug run test bench lint gopls fmt

help: ## list the targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build bin/gh-tui with the release flags
	$(GOBUILD) $(RELEASE_FLAGS) -o $(BIN) ./cmd/gh-tui

build-debug: ## build bin/gh-tui with symbols and no optimizations, for dlv
	$(GOBUILD) -gcflags='all=-N -l' -o $(BIN) ./cmd/gh-tui

run: build ## build and run gh-tui; pass flags with ARGS, e.g. ARGS=--debug
	./$(BIN) $(ARGS)

test: ## run the tests with the race detector
	go test -race ./...

bench: ## run every benchmark in the module
	go test -run '^$$' -bench . -benchmem ./...

lint: ## run golangci-lint
	golangci-lint run ./...

gopls: ## report every gopls diagnostic, hints too, and fail on any
	@out=$$($(GOPLS) check -severity=hint $$(git ls-files '*.go')) || exit 1; \
		[ -z "$$out" ] || { echo "$$out"; exit 1; }

fmt: ## format the code with golangci-lint
	golangci-lint fmt ./...

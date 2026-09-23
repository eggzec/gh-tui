.PHONY: build test bench lint fmt

build:
	go build -o bin/gh-tui ./cmd/gh-tui

test:
	go test -race ./...

bench:
	go test -run '^$$' -bench . -benchmem ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

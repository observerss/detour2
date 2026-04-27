GO ?= go
BIN ?= dist/detour

.PHONY: all build test vet race check profile

all: build

build:
	mkdir -p $(dir $(BIN))
	$(GO) build -o $(BIN) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

race:
	$(GO) test -race ./...

check: test vet race build

profile:
	curl -o server.pprof http://localhost:3811/debug/pprof/profile?seconds=30
	$(GO) tool pprof -http : server.pprof
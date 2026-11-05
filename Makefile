.PHONY: check build test test-race vet fmt fmt-check lint clean require-cc help

# mattn/go-sqlite3 is cgo, and the fts5 tag is what gives `tasc search` a real
# index instead of the LIKE fallback. Both are load-bearing — see AGENTS.md.
GO_TAGS := fts5
GO_FILES := $(shell git ls-files '*.go')

# The whole gate. Run it as `nix develop --command make check`.
check: require-cc fmt-check vet test

# A gate that cannot find its C compiler fails rather than silently skipping:
# without cgo, every sqlite package drops out of the build and `go test` would
# report green having compiled almost nothing.
require-cc:
	@command -v cc >/dev/null 2>&1 || command -v gcc >/dev/null 2>&1 || { \
	  echo "error: no C compiler on PATH, and tasc needs cgo (mattn/go-sqlite3 + fts5)."; \
	  echo "       run the gate in the dev shell: nix develop --command make check"; \
	  exit 1; }

build: require-cc
	go build -tags $(GO_TAGS) -o tasc .

test: require-cc
	go test -tags $(GO_TAGS) ./...

test-race: require-cc
	go test -race -tags $(GO_TAGS) ./...

vet: require-cc
	go vet -tags $(GO_TAGS) ./...

fmt:
	gofmt -l -w $(GO_FILES)

fmt-check:
	@out=$$(gofmt -l $(GO_FILES)); \
	if [ -n "$$out" ]; then \
	  echo "gofmt needed:"; echo "$$out"; exit 1; \
	fi

lint: vet

clean:
	rm -f tasc

help:
	@echo "check       the whole gate: fmt-check, vet, test (needs the dev shell)"
	@echo "test        go test -tags fts5 ./..."
	@echo "test-race   the suite under the race detector"
	@echo "vet         go vet -tags fts5 ./..."
	@echo "fmt         rewrite unformatted files"
	@echo "fmt-check   fail if any tracked .go file is unformatted"
	@echo "build       build ./tasc from the local tree (does not touch the installed binary)"
	@echo "clean       remove ./tasc"

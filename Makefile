# Load .env if present (API keys, etc.)
ifneq (,$(wildcard .env))
include .env
export
endif

BINARY := takumi
MODULE := github.com/tfitz/takumi
BUILD_DIR := build
COVER_DIR := coverage

.PHONY: build test lint install cover cover-html integration-test integration-test-llm benchmark benchmark-llm benchmark-perf benchmark-iterate test-all clean

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/takumi
	ln -sf $(BINARY) $(BUILD_DIR)/t

install:
	go install ./cmd/takumi
	@ln -sf "$$(go env GOPATH)/bin/takumi" "$$(go env GOPATH)/bin/t" 2>/dev/null || true

test:
	go test ./...

lint:
	go vet ./...

cover:
	@mkdir -p $(COVER_DIR)
	go test -coverprofile=$(COVER_DIR)/coverage.out -covermode=atomic ./src/...
	go tool cover -func=$(COVER_DIR)/coverage.out

cover-html: cover
	go tool cover -html=$(COVER_DIR)/coverage.out -o $(COVER_DIR)/coverage.html
	@echo "Coverage report: $(COVER_DIR)/coverage.html"

TESTDATA := testdata

integration-test: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	@mkdir -p $(TESTDATA)
	cd tests/integration/promptfoo && TAKUMI_BIN=../../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval --no-cache -o ../../../$(TESTDATA)/promptfoo-results.txt -o ../../../$(TESTDATA)/promptfoo-results.json
	go test ./tests/integration/mcp/ -v
	@echo ""
	@echo "Test logs:"
	@ls -1 $(TESTDATA)/*.log $(TESTDATA)/promptfoo-results.* 2>/dev/null | sed 's/^/  /'

integration-test-llm: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	@test -n "$$ANTHROPIC_API_KEY" || { echo "Error: ANTHROPIC_API_KEY not set."; exit 1; }
	cd tests/integration/promptfoo && TAKUMI_BIN=../../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval -c promptfooconfig.llm.yaml --no-cache

benchmark: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	cd tests/benchmark && TAKUMI_BIN=../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval --no-cache

benchmark-llm: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	@test -n "$$ANTHROPIC_API_KEY" || { echo "Error: ANTHROPIC_API_KEY not set."; exit 1; }
	cd tests/benchmark && TAKUMI_BIN=../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval -c promptfooconfig.llm.yaml --no-cache

PYTHON := $(shell python3 -c "import anthropic" 2>/dev/null && echo python3 || echo python3.12)

benchmark-perf: build
	@test -n "$$ANTHROPIC_API_KEY" || { echo "Error: ANTHROPIC_API_KEY not set."; exit 1; }
	@$(PYTHON) -c "import anthropic" 2>/dev/null || { echo "Error: anthropic package not found. Run: pip install anthropic"; exit 1; }
	TAKUMI_BIN=$(BUILD_DIR)/$(BINARY) $(PYTHON) tests/benchmark/perf/benchmark.py $(ARGS)

benchmark-iterate: build
	@test -n "$$ANTHROPIC_API_KEY" || { echo "Error: ANTHROPIC_API_KEY not set."; exit 1; }
	@$(PYTHON) -c "import anthropic" 2>/dev/null || { echo "Error: anthropic package not found. Run: pip install anthropic"; exit 1; }
	TAKUMI_BIN=$(BUILD_DIR)/$(BINARY) $(PYTHON) tests/benchmark/iterative/benchmark.py $(ARGS)

test-all: test integration-test

clean:
	rm -rf $(BUILD_DIR) $(COVER_DIR)

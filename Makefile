BINARY := takumi
MODULE := github.com/tfitz/takumi
BUILD_DIR := build
COVER_DIR := coverage

.PHONY: build test lint install cover cover-html integration-test integration-test-llm benchmark benchmark-llm test-all clean

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/takumi

install:
	go install ./cmd/takumi

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

integration-test: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	cd tests/integration && TAKUMI_BIN=../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval --no-cache

integration-test-llm: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	@test -n "$$ANTHROPIC_API_KEY" || { echo "Error: ANTHROPIC_API_KEY not set."; exit 1; }
	cd tests/integration && TAKUMI_BIN=../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval -c promptfooconfig.llm.yaml --no-cache

benchmark: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	cd tests/benchmark && TAKUMI_BIN=../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval --no-cache

benchmark-llm: build
	@command -v npx >/dev/null 2>&1 || { echo "Error: npx not found. Install Node.js 18+."; exit 1; }
	@test -n "$$ANTHROPIC_API_KEY" || { echo "Error: ANTHROPIC_API_KEY not set."; exit 1; }
	cd tests/benchmark && TAKUMI_BIN=../../$(BUILD_DIR)/$(BINARY) npx --yes promptfoo@latest eval -c promptfooconfig.llm.yaml --no-cache

test-all: test integration-test

clean:
	rm -rf $(BUILD_DIR) $(COVER_DIR)

# ==========================
# Variables
# ==========================
BINARY_NAME=2L1nk
CMD_PATH=./cmd/2L1nk
BUILD_DIR=./bin
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

GO=go

# ==========================
# Default Target
# ==========================
.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build:"
	@echo "  make build          - Build normal binaries (Linux + Windows)"
	@echo "  make build-static   - Build static binaries (Linux + Windows)"
	@echo "  make run            - Run Linux binary from ./bin/$(BINARY_NAME)-linux-x86-64"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run all tests with race detector"
	@echo "  make test-verbose   - Run all tests with race detector and verbose output"
	@echo "  make test-api       - Run API tests only"
	@echo "  make test-db        - Run DB tests only"
	@echo ""
	@echo "Coverage:"
	@echo "  make coverage       - Show test coverage in terminal"
	@echo "  make coverage-html  - Generate HTML coverage report"
	@echo ""
	@echo "Quality:"
	@echo "  make fmt            - Format all Go files"
	@echo "  make lint           - Run go vet"
	@echo "  make tidy           - Clean up go.mod/go.sum"
	@echo ""
	@echo "Maintenance:"
	@echo "  make clean          - Remove build artifacts and coverage files"

# ==========================
# Build (Normal)
# ==========================
.PHONY: build
build:
	mkdir -p $(BUILD_DIR)

	# Linux (x86-64 / ARM64)
	GOOS=linux GOARCH=amd64 \
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-x86-64 $(CMD_PATH)

	GOOS=linux GOARCH=arm64 \
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)

	# Windows (x86-64 / ARM64)
	GOOS=windows GOARCH=amd64 \
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-x86-64.exe $(CMD_PATH)

	GOOS=windows GOARCH=arm64 \
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(CMD_PATH)

	# macOS (Intel x86-64 / Apple Silicon ARM64)
	GOOS=darwin GOARCH=amd64 \
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-x86-64 $(CMD_PATH)

	GOOS=darwin GOARCH=arm64 \
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)

# ==========================
# Build (Static)
# ==========================
.PHONY: build-static
build-static:
	mkdir -p $(BUILD_DIR)

	# Linux static (x86-64 / ARM64)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/$(BINARY_NAME)-linux-x86-64 $(CMD_PATH)

	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)

	# Windows static (x86-64 / ARM64)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/$(BINARY_NAME)-windows-x86-64.exe $(CMD_PATH)

	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(CMD_PATH)

	# macOS static (Intel x86-64 / Apple Silicon ARM64)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-x86-64 $(CMD_PATH)

	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)

# ==========================
# Run
# ==========================
.PHONY: run
run:
	$(BUILD_DIR)/$(BINARY_NAME)-linux-x86-64

# ==========================
# Clean
# ==========================
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)

# ==========================
# Tests
# ==========================
.PHONY: test
test:
	$(GO) test -race ./...

.PHONY: test-verbose
test-verbose:
	$(GO) test -race -v ./...

.PHONY: test-api
test-api:
	$(GO) test ./tests/api -v

.PHONY: test-db
test-db:
	$(GO) test ./tests/db -v

# ==========================
# Coverage
# ==========================
.PHONY: coverage
coverage:
	$(GO) test ./... -coverprofile=$(COVERAGE_FILE)
	$(GO) tool cover -func=$(COVERAGE_FILE)

.PHONY: coverage-html
coverage-html:
	$(GO) test ./... -coverprofile=$(COVERAGE_FILE)
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Open $(COVERAGE_HTML) in your browser"

# ==========================
# Quality
# ==========================
.PHONY: lint
lint:
	$(GO) vet ./...

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy
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
	@echo "  make run            - Run Linux normal binary"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run all tests"
	@echo "  make test-api       - Run API tests only"
	@echo "  make test-db        - Run DB tests only"
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
	mkdir -p $(BUILD_DIR)/linux
	mkdir -p $(BUILD_DIR)/windows

	# Linux normal build
	GOOS=linux GOARCH=amd64 \
	$(GO) build -o $(BUILD_DIR)/linux/$(BINARY_NAME) $(CMD_PATH)

	# Windows normal build
	GOOS=windows GOARCH=amd64 \
	$(GO) build -o $(BUILD_DIR)/windows/$(BINARY_NAME).exe $(CMD_PATH)

# ==========================
# Build (Static)
# ==========================
.PHONY: build-static
build-static:
	mkdir -p $(BUILD_DIR)/linux
	mkdir -p $(BUILD_DIR)/windows

	# Linux static
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/linux/$(BINARY_NAME)-static $(CMD_PATH)

	# Windows static
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
	$(GO) build -a -ldflags="-w -s" \
	-o $(BUILD_DIR)/windows/$(BINARY_NAME)-static.exe $(CMD_PATH)

# ==========================
# Run
# ==========================
.PHONY: run
run:
	$(BUILD_DIR)/linux/$(BINARY_NAME)

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
	$(GO) test ./...

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
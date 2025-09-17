# kubectl-odh CLI Makefile

# Binary name
BINARY_NAME=bin/kubectl-odh

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build flags
LDFLAGS = -X 'github.com/lburgazzoli/odh-cli/internal/version.Version=$(VERSION)' \
          -X 'github.com/lburgazzoli/odh-cli/internal/version.Commit=$(COMMIT)' \
          -X 'github.com/lburgazzoli/odh-cli/internal/version.Date=$(DATE)'

# Build the binary
.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) cmd/main.go

# Run the doctor command
.PHONY: run
run:
	go run -ldflags "$(LDFLAGS)" cmd/main.go doctor

# Tidy up dependencies
.PHONY: tidy
tidy:
	go mod tidy

# Clean build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY_NAME)

# Format code
.PHONY: fmt
fmt:
	go fmt ./...

# Run tests
.PHONY: test
test:
	go test ./...

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build       - Build the kubectl-odh binary"
	@echo "  doctor      - Build and run the doctor command"
	@echo "  doctor-json - Build and run the doctor command with JSON output"
	@echo "  tidy        - Tidy up Go module dependencies"
	@echo "  clean       - Remove build artifacts"
	@echo "  fmt         - Format Go code"
	@echo "  test        - Run tests"
	@echo "  help        - Show this help message"
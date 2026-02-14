# Makefile for GCI
# Usage: make build VERSION=1.0.0

# Default version
VERSION ?= dev

# Build information
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell git log -1 --format=%cd --date=iso-strict 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS := -X gci/internal/version.Version=$(VERSION) -X gci/internal/version.Commit=$(COMMIT) -X gci/internal/version.Date=$(DATE)

.PHONY: help build test clean install hooks tag

help: ## Show this help message
	@echo "GCI Build System"
	@echo "=================="
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build GCI binary (use VERSION=x.x.x to set version)
	@echo "Building GCI..."
	@echo "  Version: $(VERSION)"
	@echo "  Commit:  $(COMMIT)"
	@echo "  Date:    $(DATE)"
	@echo
	go build -ldflags "$(LDFLAGS)" -o gci .
	@echo "✅ Build complete: ./gci"

test: ## Run all tests
	go test ./...

clean: ## Clean build artifacts
	rm -f gci

install: build ## Install GCI to $GOPATH/bin
	go install -ldflags "$(LDFLAGS)" .

version: build ## Show version information
	./gci version

# Development shortcuts
dev: ## Quick development build
	@$(MAKE) build VERSION=dev

release: ## Build release version (requires VERSION=x.x.x)
	@if [ "$(VERSION)" = "dev" ]; then \
		echo "Error: Please specify VERSION for release build"; \
		echo "Usage: make release VERSION=1.0.0"; \
		exit 1; \
	fi
	@$(MAKE) build VERSION=$(VERSION)
	@echo "Release build complete!"

hooks: ## Install local git hooks
	@ln -sf ../../scripts/pre-push .git/hooks/pre-push
	@echo "Installed pre-push hook"

tag: ## Create a release tag (requires VERSION=x.x.x)
	@if [ "$(VERSION)" = "dev" ]; then \
		echo "Error: Please specify VERSION for release tag"; \
		echo "Usage: make tag VERSION=1.1.0"; \
		exit 1; \
	fi
	git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	@echo "Created tag v$(VERSION). Push with: git push origin v$(VERSION)"

.DEFAULT_GOAL := help
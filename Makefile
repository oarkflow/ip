.PHONY: help build install check clean test

# Default target
help:
	@echo "Sandbox - Project Manager with Sandbox Containers"
	@echo ""
	@echo "Available targets:"
	@echo "  build     - Build the sandbox binary"
	@echo "  install   - Install required system dependencies"
	@echo "  check     - Check if required tools are installed"
	@echo "  clean     - Clean build artifacts"
	@echo "  test      - Run tests"
	@echo "  help      - Show this help message"

# Build the application
build:
	@echo "🔨 Building sandbox..."
	go build -o sandbox cmd/sandbox/main.go
	@echo "✅ Build complete: ./sandbox"

# Install system dependencies
install:
	@echo "📦 Installing required tools..."
	./install.sh

# Install development dependencies (including Go)
install-dev:
	@echo "📦 Installing required tools + development tools..."
	INSTALL_GO=true ./install.sh

# Check if required tools are available
check:
	@echo "🔍 Checking required tools..."
	@command -v docker >/dev/null 2>&1 && echo "✅ Docker: $(shell docker --version)" || echo "❌ Docker: Not found"
	@command -v git >/dev/null 2>&1 && echo "✅ Git: $(shell git --version)" || echo "❌ Git: Not found"
	@echo "ℹ️  Go is optional (only needed for development)"

# Check all tools including development ones
check-all:
	@echo "🔍 Checking all tools..."
	@command -v docker >/dev/null 2>&1 && echo "✅ Docker: $(shell docker --version)" || echo "❌ Docker: Not found"
	@command -v git >/dev/null 2>&1 && echo "✅ Git: $(shell git --version)" || echo "❌ Git: Not found"
	@command -v go >/dev/null 2>&1 && echo "✅ Go: $(shell go version)" || echo "❌ Go: Not found (optional for development)"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	rm -f sandbox
	rm -f cmd/sandbox/sandbox
	@echo "✅ Clean complete"

# Run tests
test:
	@echo "🧪 Running tests..."
	go test ./...
	@echo "✅ Tests complete"

# Development setup
dev-setup: check build
	@echo "🚀 Development environment ready!"
	@echo "Run: ./sandbox --help"

# Install and setup everything
setup: install dev-setup

# Docker helpers
docker-check:
	@echo "🐳 Docker status:"
	@docker --version
	@docker ps

# Git helpers
git-check:
	@echo "📚 Git status:"
	@git --version
	@git status --porcelain || echo "Not a git repository"

# Full system check
system-check: check docker-check git-check
	@echo "🎯 System check complete"

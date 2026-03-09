# TigerbeetleMiniPIX - PIX Clearing Engine build file

# Show available commands
@help:
    echo "TigerbeetleMiniPIX - PIX Clearing Engine"
    echo ""
    echo "Available commands:"
    echo "  just help              - Show this help message"
    echo "  just build             - Build all binaries"
    echo "  just test              - Run all tests"
    echo "  just test-clearing     - Run clearing engine tests"
    echo "  just test-loadtest     - Run load test tests"
    echo "  just clean             - Remove generated artifacts"
    echo "  just fmt               - Format all Go code"
    echo "  just vet               - Run go vet checks"

# Build all binaries
build:
    echo "Building binaries..."
    go build -o engine ./cmd/engine
    go build -o seed ./cmd/seed
    go build -o probe ./cmd/probe
    go build -o loadtest ./cmd/loadtest
    echo "✓ Build complete: engine, seed, probe, loadtest"

# Run all tests
test:
    echo "Running all tests..."
    go test -v ./...

# Run clearing engine tests
test-clearing:
    echo "Running clearing engine tests..."
    go test -v ./internal/clearing -timeout 30s

# Run load test tests
test-loadtest:
    echo "Running load test tests..."
    go test -v ./cmd/loadtest -timeout 30s

# Run tests with coverage
test-coverage:
    echo "Running tests with coverage..."
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    echo "✓ Coverage report: coverage.html"

# Format all Go code
fmt:
    echo "Formatting Go code..."
    go fmt ./...
    echo "✓ Format complete"

# Run go vet checks
vet:
    echo "Running go vet checks..."
    go vet ./...
    echo "✓ Vet checks complete"

# Clean build artifacts and cache
clean:
    echo "Cleaning artifacts..."
    rm -f engine seed probe loadtest coverage.out coverage.html
    go clean -testcache
    echo "✓ Clean complete"

# Run all checks (fmt, vet, build, test)
check: fmt vet build test
    echo "✓ All checks passed"

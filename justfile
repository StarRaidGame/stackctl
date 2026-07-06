# StarRaid stackctl — the stack control-center TUI. Run `just` to list recipes.
#
# Standalone Go module (no cross-repo imports, no go.work). It supervises the
# sibling components via their own `just` recipes, so build/install those first.

# List available recipes
default:
    @just --list

# Fetch deps (stdlib-only for now; kept for parity with the other components)
install:
    go mod download

# Build all packages
build:
    go build ./...

# Run stackctl (headless for now: brings the stack up, Ctrl-C stops all).
# Pass flags through, e.g. `just run -root /path/to/stack`.
run *args:
    go run ./cmd/stackctl {{args}}

# Run the tests
test:
    go test ./...

# Format Go code
fmt:
    go fmt ./...

# Vet
vet:
    go vet ./...

# Caddy Random File Module

## Project Name

Caddy Random File Module – A custom Caddy v2 handler that serves random files from disk with filtering, recursion, and caching.

## Overview

This repository implements the `http.handlers.random_file` module for Caddy v2. The handler resolves a configured root directory, discovers candidate files (optionally traversing subdirectories), filters them with case-insensitive glob patterns, and responds with a randomly selected file. It supports both JSON configuration and Caddyfile syntax, making it easy to drop into existing Caddy installations.

The project emphasizes safety and performance: paths are normalized to avoid traversal, only regular files are served, include patterns are matched with Go's glob engine, and an optional TTL cache prevents excessive filesystem scans while guarding against cache stampedes.

## Technology Stack

- **Language/Runtime:** Go 1.25.4
- **Framework(s):** Caddy v2 module API
- **Key Dependencies:** `github.com/caddyserver/caddy/v2`, `go.uber.org/zap`
- **Build Tools:** Go toolchain, `xcaddy`, optional Nix flake dev shell (Go + Caddy tooling)

## Project Structure

```
.
├── Caddyfile.example        # Sample server configuration using random_file
├── caddyrandom.go           # Registers the module when imported
├── flake.nix / flake.lock   # Reproducible dev shell with Go, caddy, xcaddy
├── go.mod / go.sum          # Go module definition + dependency lockfile
├── randomfile/              # Module implementation
│   ├── caddyfile.go         # Parser for the `random_file` directive
│   ├── handler.go           # HTTP handler, recursion, caching, include logic
│   ├── handler_test.go      # Unit tests for matching, caching, recursion
│   └── randomfile.go        # Module struct, registration hooks
└── caddy / _tmp.*           # Generated binaries/configs during development
```

## Key Features

- Random file selection from a configurable root directory
- Case-insensitive glob filtering with optional recursive traversal
- TTL-based candidate cache with stampede protection
- Native Caddyfile directive plus JSON configuration support

## Getting Started

### Prerequisites

- Go 1.25+ toolchain
- `xcaddy` for building custom binaries
- Optional: Nix with flakes enabled (`nix develop` uses `flake.nix`)

### Installation

```bash
git clone https://example.com/caddy-random.git
cd caddy-random
# Optional reproducible dev shell
defaultShell="nix develop --command"
$defaultShell bash -lc "go test ./..."
```

### Usage

```bash
# Build a custom Caddy binary that embeds the module
nix develop --command bash -lc \
  "xcaddy build --with example.com/caddy-random=./"

# Run the provided sample configuration
./caddy run --config Caddyfile.example
```

## Development

### Available Scripts / Commands

- `go test ./...` – Run unit tests
- `xcaddy build --with example.com/caddy-random=./` – Build Caddy with this module
- `nix develop --command <cmd>` – Execute commands inside the dev shell (Go, Caddy, xcaddy available)

### Development Workflow

1. Enter the dev shell (`nix develop`) or ensure Go + xcaddy are installed locally.
2. Update Go sources under `randomfile/`; keep formatting with `gofmt`.
3. Run `go test ./...` before submitting changes.
4. Build a custom binary via `xcaddy build --with example.com/caddy-random=./` to validate end-to-end.

## Configuration

- **Caddyfile Directive**
  ```
  random_file {
      root /path/to/files
      include *.jpg *.png   # optional glob filters (case-insensitive)
      recursive             # optional – scan subdirectories
      cache 10s             # optional – cache candidate list for TTL
  }
  ```
- **JSON Fields:** `root`, `include`, `recursive`, `cache` (duration string or seconds)
- Place the directive inside your site routing (e.g., `handle_path /wallpaper/* { ... }`).

## Architecture

- `RandomFile` implements `caddy.Provisioner`, `caddy.Validator`, and `caddyhttp.MiddlewareHandler`.
- `handler.go` resolves the target directory, enumerates files (`filepath.WalkDir` when recursive), applies include glob filters, and caches candidate lists using a TTL and keyed by configuration to avoid stale matches.
- `caddyfile.go` parses the `random_file` directive and maps subdirectives (`root`, `include`, `recursive`, `cache`) to struct fields.
- `handler_test.go` verifies include matching, recursion, and cache semantics.

## Contributing

- Format code with `gofmt` and keep tests updated.
- Open pull requests with clear descriptions and ensure `go test ./...` passes.
- Use `xcaddy build --with example.com/caddy-random=./` to confirm integration before submitting.

## License

No explicit LICENSE file is present. Treat the repository as proprietary unless the owners clarify licensing.

- `handler.go` resolves the root directory, discovers candidates (with optional recursion), applies glob filters, and caches results.
- The cache key incorporates directory, recursion flag, and include patterns to guarantee consistency across config changes.
- `caddyfile.go` maps directive syntax to the Go struct, enabling human-friendly configuration.

## Contributing

- Use standard Go formatting (`gofmt`), keep tests up to date, and add new ones for behavioral changes.
- Submit pull requests with clear descriptions and ensure `go test ./...` passes.

## License

A dedicated LICENSE file is not present. Unless otherwise specified by the repository owner, treat the code under the repository’s default terms.

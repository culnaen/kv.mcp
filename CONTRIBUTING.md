# Contributing to kv.mcp

We welcome contributions! This guide explains how to build, test, and submit changes to kv.mcp.

## Building from Source

To build the kv.mcp binary:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kv.mcp .
```

The flags minimize binary size by:
- Disabling CGO for static linking
- Stripping path information with `-trimpath`
- Removing symbols and debug info with `-ldflags="-s -w"`

## Running Tests

Run the full test suite with race detection:

```bash
go test ./... -race
```

The `-race` flag detects concurrent access issues that would otherwise go unnoticed.

## Running E2E Tests

End-to-end tests clone xray-core, build an index, and run 10 queries:

```bash
make e2e
```

This validates the full indexing and search pipeline on a real-world codebase.

## Running Benchmarks

To measure indexing and query performance against a baseline:

```bash
make bench
```

The benchmark harness compares KV store performance vs grep+read baseline on xray-core.

## Code Style

Ensure code meets Go standards:

```bash
go vet ./...
golangci-lint run ./...
```

Both checks must pass before submitting a PR.

## PR Process

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes and ensure all tests pass locally
4. Push to your fork and open a PR against `main`
5. Address any feedback from maintainers

## Reporting Issues

When reporting an issue, include:

- Go version: `go version`
- Operating system: `uname -a`
- kv.mcp version: `./kv.mcp --version`
- Reproduction steps: Clear instructions to recreate the issue
- Expected vs actual behavior

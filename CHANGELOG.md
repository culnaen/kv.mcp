# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-04-20

### Added
- MCP server with 4 tools: `search`, `get_function`, `get_code`, `update_function`
- Go AST indexer using golang.org/x/tools (packages.Load)
- bbolt-backed KV store with extracted/curated dual-bucket model
- Token benchmark harness comparing KV vs grep+read baseline on Xray-core
- CI pipeline: vet, test, lint, build, binary size check

### Known Limitations
- Indexes host GOOS/GOARCH only (cross-platform support planned for v2)
- Test function attachment uses heuristic short-name matching
- Go projects only (multi-language support planned for v2)
- bbolt single-writer constraint: stop serve before re-indexing

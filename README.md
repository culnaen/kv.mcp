# kv.mcp

Token-optimized codebase access for LLM agents via [MCP](https://modelcontextprotocol.io).

Instead of reading whole files, agents query indexed function metadata — reducing tool-output tokens by **40–87%** on real Go codebases (estimated lower bound, see [Benchmark](#benchmark)).

## Why

When an LLM agent searches a large codebase, the naive path is:

1. `grep "Dial" .` → hundreds of matches, kilobytes of context
2. Read `proxy/socks/server.go` → 300+ lines for one function

kv.mcp replaces this with:

1. `search("Dial")` → 50 matches, names + descriptions only
2. `get_function("socks.(*Client).Dial")` → loc, signature, deps
3. `get_code("proxy/socks/client.go:45-89")` → exactly those 45 lines

## Architecture

```
Go project
    │
    ▼
kv.mcp index ──► bbolt DB ─────────────────────────────────┐
                  ├── extracted (AST-indexed functions)     │
                  └── curated  (agent-annotated overrides)  │
                                                            │
MCP client (Claude Code) ◄── kv.mcp serve (stdio/JSON-RPC) ┘
```

Two buckets per function: **extracted** (auto from `packages.Load` + AST) and **curated** (agent-written, persists across reindexing). Curated fields override extracted on read.

## Quick Start

### Install

**Download a pre-built binary** from the [Releases](https://github.com/culnaen/kv.mcp/releases) page, or build from source:

```bash
git clone https://github.com/culnaen/kv.mcp
cd kv.mcp
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kv.mcp .
```

### Index your project

```bash
./kv.mcp index --db /absolute/path/to/.kv.mcp.db /path/to/your/project
```

> **Note:** Indexes host GOOS/GOARCH only by default. Use `--tags` to include code behind specific build tags. Full cross-platform coverage (multiple GOOS/GOARCH) is v2 scope.

> **Important:** Stop `kv.mcp serve` before re-indexing the same DB (bbolt single-writer constraint).

### Configure Claude Code

Add to `.claude/settings.json` (use absolute paths — MCP server CWD is unpredictable):

```json
{
  "mcpServers": {
    "kv.mcp": {
      "command": "/absolute/path/to/kv.mcp",
      "args": [
        "serve",
        "--db", "/absolute/path/to/.kv.mcp.db",
        "--root", "/path/to/your/project"
      ]
    }
  }
}
```

Start a new Claude Code session — the tools appear automatically.

## MCP Tools

| Tool | Description |
|------|-------------|
| `search(query)` | Case-insensitive substring search across function names and descriptions. Returns up to 50 matches. |
| `get_function(name)` | Full metadata for a function: loc, description, depends, test. Partial names return all matches. |
| `get_code(loc)` | Read source lines for a loc string (e.g. `proxy/socks/server.go:10-30`). Capped at `--max-lines` (default: 150, hard cap 500). |
| `update_function(name, ...)` | Write curated metadata. Persists across reindexing. |

### update_function merge contract

| Field value | Effect |
|-------------|--------|
| absent / `null` | keep existing extracted value |
| empty string / empty array | clear curated override |
| non-empty value | replace (wins over extracted) |

## Agent Workflow Example

Exploring how Xray-core handles outbound connections:

```
# 1. Find candidates
search("Dial")
→ 23 matches including socks.(*Client).Dial, vmess.(*Handler).Dial, ...

# 2. Inspect the most relevant one
get_function("socks.(*Client).Dial")
→ { loc: "proxy/socks/client.go:45-89", depends: ["net.Dial", "context.Context"], ... }

# 3. Read only those lines
get_code("proxy/socks/client.go:45-89")
→ 45 lines (vs 300+ for the full file)

# 4. Annotate for next session
update_function("socks.(*Client).Dial",
  description: "Establishes SOCKS5 connection; calls net.Dial then sends CONNECT request")
```

## Benchmark

Tested on [Xray-core](https://github.com/XTLS/Xray-core) (~6,400 indexed functions). 10 representative queries, comparing KV path vs baseline (ripgrep + 80-line read window).

**KV wins 7 / 8 meaningful queries.**

| # | Query | Baseline Tokens | KV Tokens | Δ | Win |
|---|-------|----------------|-----------|---|-----|
| 1 | Dial | 14,442 | 8,655 | −5,787 | KV |
| 2 | Register | 8,629 | 3,939 | −4,690 | KV |
| 3 | ServeHTTP | 1,497 | 1,445 | −52 | KV |
| 5 | NewHandler | 2,136 | 611 | −1,525 | KV |
| 6 | Process | 7,363 | 5,069 | −2,294 | KV |
| 8 | Dispatch | 9,447 | 12,854 | +3,407 | Baseline |
| 9 | ReadHeader | 1,083 | 18 | −1,065 | KV |
| 10 | AddUser | 2,758 | 2,396 | −362 | KV |

> **Methodology:** Token counts use `characters ÷ 4` heuristic — a reproducible, dependency-free **estimated lower bound**, not exact LLM tokenizer output. Full results in [`bench/tokens/results.md`](bench/tokens/results.md). Reproduce with `make bench`.

## Configuration

### `kv.mcp index`

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | required | Path to bbolt database file |
| `--tags` | _(empty)_ | Build tags passed to `go/packages` (e.g. `--tags integration,linux`) |
| `--verbose` | false | Print each indexed function name to stderr |
| (positional) | required | Project root directory |

### `kv.mcp serve`

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | required | Path to bbolt database file |
| `--root` | required | Project root (used to resolve relative locs) |
| `--max-lines` | 150 | Maximum lines returned by `get_code`. Hard cap: 500. Tuned against xray-core and docker/compose: p99 function size is ~120 lines. |

## Limitations

- **Go only.** Multi-language support is v2 scope.
- **Host platform.** Indexes GOOS/GOARCH of the machine running `index`. Use `--tags` to include code behind specific build tags; cross-platform indexing (multiple GOOS/GOARCH) is v2 scope.
- **Test heuristic.** `TestFoo` is attached to `Foo` by short-name match. May collide on common names.
- **Single-writer.** bbolt allows one writer at a time. Stop `serve` before running `index` against the same DB.
- **Description starts empty.** Extracted functions have no description until an agent calls `update_function`.

## v2 Roadmap

- LSP / gopls integration (cross-package symbol resolution)
- Multi-language support
- Cross-platform build-tag coverage
- Full LLM-in-the-loop token measurement
- BM25 search ranking

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).

# kv.mcp

Token-optimized codebase access layer for LLM agents. Agents query MCP tools instead of opening files directly, reducing token consumption.

## Install

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kv.mcp .
```

## Index a Go project

```bash
./kv.mcp index --db /absolute/path/to/.kv.mcp.db /path/to/project
```

> **Note:** Build-tag scope in v1 is host GOOS/GOARCH only. Files guarded by non-host platform tags (e.g. `_windows.go` on Linux) are not indexed. Cross-platform coverage is v2 scope.

> **Important:** Stop `kv.mcp serve` before running `kv.mcp index` against the same DB (bbolt single-writer constraint).

## Wire up with Claude Code

Add to `.claude/settings.json`:

```json
{
  "mcpServers": {
    "kv.mcp": {
      "command": "/absolute/path/to/kv.mcp",
      "args": ["serve", "--db", "/absolute/path/to/.kv.mcp.db", "--root", "/path/to/project"]
    }
  }
}
```

> **Important:** Use absolute paths for `--db` and binary. When Claude Code spawns MCP servers, the CWD is not predictable.

## MCP Tools

### get_function(name)
Returns function metadata: loc, description, depends, test. Partial names return all matches.

### search(query)
Case-insensitive substring search across function names and descriptions. Returns up to 50 matches.

### get_code(loc)
Read source lines for a loc string (e.g. `proxy/socks/server.go:10-30`). Use loc from `get_function` to stay within the `--max-lines` cap (default: 500).

### update_function(name, description?, loc?, depends?, test?)
Update curated metadata. Curated data persists across reindexing.

**Merge contract:**
| Field value | Effect |
|---|---|
| absent / null | leave existing extracted value |
| empty string / empty array | clear curated override |
| non-empty value | replace (wins over extracted) |

## Workflow

```
kv.mcp index ./          # build initial index
kv.mcp serve --db ...    # start MCP server

Agent workflow:
  search("proxy handler")   → find relevant functions
  get_function("pkg.Func")  → get loc, depends
  get_code("file.go:10-30") → read specific lines (not whole file)
  update_function(...)      → refine descriptions as you learn
```

## Token Reduction

The KV path reduces tokens by replacing full-file reads with targeted symbol lookups. See `bench/tokens/results.md` for benchmark results on Xray-core.

## v2 Roadmap

- LSP / gopls integration (cross-package symbol resolution)
- Multi-language support
- Cross-platform build-tag coverage
- Full LLM-in-the-loop token measurement
- BM25 search ranking

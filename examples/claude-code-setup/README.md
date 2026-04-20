# Setting Up kv.mcp with Claude Code

This guide covers the complete setup process for integrating kv.mcp with Claude Code, enabling token-optimized codebase exploration.

## Prerequisites

- **Go 1.21 or later** — required to build from source
- **Claude Code installed** — the MCP client
- **A Go project** — the codebase you want to index and explore

Verify Go version:

```bash
go version
```

## Installation

### Option 1: Download a Pre-Built Release

Visit the [Releases](https://github.com/culnaen/kv.mcp/releases) page and download the binary for your platform.

```bash
# Example: downloading for Linux x86_64
wget https://github.com/culnaen/kv.mcp/releases/download/v0.1.0/kv.mcp-linux-amd64
chmod +x kv.mcp-linux-amd64
mv kv.mcp-linux-amd64 /usr/local/bin/kv.mcp
```

Verify installation:

```bash
kv.mcp --help
```

### Option 2: Build from Source

Clone the repository and build:

```bash
git clone https://github.com/culnaen/kv.mcp
cd kv.mcp
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kv.mcp .
```

Move the binary to your preferred location:

```bash
mv kv.mcp /usr/local/bin/
```

## Indexing Your Project

Once kv.mcp is installed, index your Go project. Use absolute paths for both the database and project directory.

```bash
kv.mcp index --db ~/.kv.mcp/myproject.db /path/to/your/go/project
```

Example:

```bash
kv.mcp index --db ~/.kv.mcp/xray-core.db ~/src/Xray-core
```

This command:
- Scans the project using `packages.Load` and AST parsing
- Extracts all function metadata (location, signature, dependencies)
- Creates two buckets per function: **extracted** (auto-generated) and **curated** (agent-annotated, persists across reindexing)
- Stores everything in a bbolt database

Notes:
- Indexing may take a minute on large codebases (~6,000+ functions)
- Only the host platform's files are indexed (see Limitations section in the main README)
- Stop any running `kv.mcp serve` process before re-indexing (bbolt single-writer constraint)

## Configure Claude Code

Add the MCP server configuration to `.claude/settings.json` in your Claude Code project. Use **absolute paths** — the MCP server's working directory is unpredictable.

```json
{
  "mcpServers": {
    "kv.mcp": {
      "command": "/absolute/path/to/kv.mcp",
      "args": [
        "serve",
        "--db", "/absolute/path/to/.kv.mcp/myproject.db",
        "--root", "/path/to/your/go/project"
      ]
    }
  }
}
```

Example (Linux):

```json
{
  "mcpServers": {
    "kv.mcp": {
      "command": "/usr/local/bin/kv.mcp",
      "args": [
        "serve",
        "--db", "/home/user/.kv.mcp/xray-core.db",
        "--root", "/home/user/src/Xray-core"
      ]
    }
  }
}
```

Example (macOS with Homebrew):

```json
{
  "mcpServers": {
    "kv.mcp": {
      "command": "/usr/local/bin/kv.mcp",
      "args": [
        "serve",
        "--db", "/Users/user/.kv.mcp/xray-core.db",
        "--root", "/Users/user/src/Xray-core"
      ]
    }
  }
}
```

## Verification

### Start a New Claude Code Session

Open Claude Code and start a fresh session (important: tools appear on session startup, not mid-session).

Type:

```
What MCP tools do you have?
```

You should see kv.mcp tools listed:

- `search(query)` — Search function names and descriptions
- `get_function(name)` — Get metadata for a function
- `get_code(loc)` — Read source lines from a location
- `update_function(name, ...)` — Annotate function metadata

If the tools appear, your setup is complete. You can now use kv.mcp for token-optimized codebase exploration.

### Quick Test

Ask Claude Code to explore your project:

```
Search for "Handler" in my codebase and tell me what types implement it.
```

Claude Code will use `search("Handler")` and return a curated list instead of grep noise.

## Troubleshooting

### "Tool not found" error

Symptom: Claude Code says kv.mcp tools are not available.

**Solution:**

1. Verify paths in `.claude/settings.json` are absolute (no `~` or relative paths)
2. Check that the binary exists at the command path:
   ```bash
   ls -la /absolute/path/to/kv.mcp
   ```
3. Test the binary directly:
   ```bash
   /absolute/path/to/kv.mcp serve --db /path/to/db --root /path/to/project &
   ```
4. Restart Claude Code entirely (close and reopen the application)

### "DB locked" error

Symptom: Indexing fails with "database locked".

**Cause:** A `kv.mcp serve` process is still running against that DB.

**Solution:**

```bash
# Find and stop the serve process
pkill -f "kv.mcp serve"

# Verify it stopped
pgrep -f "kv.mcp serve" || echo "Stopped"

# Now re-index
kv.mcp index --db ~/.kv.mcp/myproject.db /path/to/project
```

### Index is stale

Symptom: Searching returns old results, or functions you just added don't appear.

**Solution:** Re-run the index command. Curated annotations (descriptions and overrides) are preserved:

```bash
kv.mcp index --db ~/.kv.mcp/myproject.db /path/to/project
```

This re-extracts the AST and updates the "extracted" bucket while keeping your "curated" bucket intact.

### Settings file in wrong location

If you're unsure where `.claude/settings.json` is:

```bash
# In your project root
find . -name settings.json -o -name settings.local.json 2>/dev/null
```

The settings file can be in:
- `<project-root>/.claude/settings.json` (project-level, recommended)
- `~/.claude/settings.json` (global, applies to all projects)

For kv.mcp setup, use project-level settings so the database path is relative to your project.

## Next Steps

Once verification passes:

1. Read the [Agent Workflow Example](../workflow/README.md) to see kv.mcp in action
2. Explore your codebase using Claude Code — ask architectural questions without grepping
3. Annotate important functions with `update_function` to improve future explorations

## Performance Notes

On a typical codebase (1,000-10,000 functions), kv.mcp search and metadata retrieval is instantaneous (<100ms). The main cost is initial indexing, which is a one-time operation.

For a codebase with 6,400 functions (Xray-core), indexing takes roughly 30-60 seconds.

## See Also

- [Main README](../../README.md)
- [Agent Workflow Example](../workflow/README.md)
- [kv.mcp GitHub Releases](https://github.com/culnaen/kv.mcp/releases)

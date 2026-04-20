# Baseline Agent Policy

The baseline agent simulates a competent Claude Code agent WITHOUT the kv.mcp MCP tools.

## Available operations
- `Grep(pattern, path)`: runs ripgrep for the pattern in the project root
- `Read(file, offset, limit)`: reads `limit` lines from `file` starting at `offset`

## Policy per query
1. Run `Grep(query, root)` to find files containing the symbol
2. For each matched file (up to 3 files), run `Read(file, max(0, matchLine-40), 80)` to get an 80-line window
3. Sum all tool output tokens

This is a realistic, competent baseline — not a strawman (no full-file reads).

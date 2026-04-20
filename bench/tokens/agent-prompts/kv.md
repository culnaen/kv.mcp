# KV Agent Policy

The KV agent uses only the 4 kv.mcp MCP tools.

## Available operations
- `search(query)`: case-insensitive substring search across function names and descriptions
- `get_function(name)`: get loc, description, depends, test for a function
- `get_code(loc)`: read specific line range

## Policy per query
1. Call `search(query)` to find relevant functions
2. Take top 1 result only — call `get_function(name)` to get metadata
3. Call `get_code(loc)` for that function's exact loc
4. Sum all tool output tokens

This path returns precise, minimal context — only the lines the agent needs.

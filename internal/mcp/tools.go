package mcp

// toolsList returns the MCP tools/list result.
func toolsList() interface{} {
	return map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "get_function",
				"description": "Get function metadata from the KV index. Returns loc, description, depends, test fields. If stale is true, the source file no longer exists.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Fully-qualified function name (e.g. 'pkg.Func' or 'pkg.(*Receiver).Method'). Partial names return all matches.",
						},
					},
					"required": []string{"name"},
				},
			},
			map[string]interface{}{
				"name":        "search",
				"description": "Search functions by name or description substring (case-insensitive). Returns up to 50 matches.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Substring to search for in function names and descriptions.",
						},
					},
					"required": []string{"query"},
				},
			},
			map[string]interface{}{
				"name":        "get_code",
				"description": "Read source lines for a loc string (e.g. 'path/file.go:10-30'). Use the loc from get_function to stay within the --max-lines cap.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"loc": map[string]interface{}{
							"type":        "string",
							"description": "Location string: 'relpath/file.go:N' or 'relpath/file.go:N-M'",
						},
					},
					"required": []string{"loc"},
				},
			},
			map[string]interface{}{
				"name":        "update_function",
				"description": "Update curated metadata for a function. Curated data persists across reindexing.\n\nMerge contract for loc, depends, test:\n- absent/null: leave existing extracted value unchanged\n- empty string or empty array: clear curated override (extracted becomes authoritative)\n- non-empty value: replace curated override (wins over extracted on read)\n\nFor description: empty string clears, non-empty replaces.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Fully-qualified function name",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Human description. Empty string clears.",
						},
						"loc": map[string]interface{}{
							"type":        "string",
							"description": "Override loc. Empty string clears.",
						},
						"depends": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Override depends list. Empty array clears.",
						},
						"test": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Override test list. Empty array clears.",
						},
					},
					"required": []string{"name"},
				},
			},
		},
	}
}

package mcp

type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string             `json:"type"`
	Properties map[string]propDef `json:"properties"`
	Required   []string           `json:"required"`
}

type propDef struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Items       *propDef `json:"items,omitempty"`
}

func toolsList() interface{} {
	strProp := func(desc string) propDef {
		return propDef{Type: "string", Description: desc}
	}
	arrProp := func(desc string) propDef {
		return propDef{Type: "array", Description: desc, Items: &propDef{Type: "string"}}
	}

	return map[string]interface{}{
		"tools": []toolDef{
			{
				Name:        "get_function",
				Description: "Get function metadata from the KV index. Returns loc, description, depends, test fields. If stale is true, the source file no longer exists.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]propDef{
						"name": strProp("Fully-qualified function name (e.g. 'pkg.Func' or 'pkg.(*Receiver).Method'). Partial names return all matches."),
					},
					Required: []string{"name"},
				},
			},
			{
				Name:        "search",
				Description: "Search functions by name or description substring (case-insensitive). Returns up to 50 matches.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]propDef{
						"query": strProp("Substring to search for in function names and descriptions."),
					},
					Required: []string{"query"},
				},
			},
			{
				Name:        "get_code",
				Description: "Read source lines for a loc string (e.g. 'path/file.go:10-30'). Use the loc from get_function to stay within the --max-lines cap.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]propDef{
						"loc": strProp("Location string: 'relpath/file.go:N' or 'relpath/file.go:N-M'"),
					},
					Required: []string{"loc"},
				},
			},
			{
				Name: "update_function",
				Description: "Update curated metadata for a function. Curated data persists across reindexing.\n\n" +
					"Merge contract for loc, depends, test:\n" +
					"- absent/null: leave existing extracted value unchanged\n" +
					"- empty string or empty array: clear curated override (extracted becomes authoritative)\n" +
					"- non-empty value: replace curated override (wins over extracted on read)\n\n" +
					"For description: empty string clears, non-empty replaces.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]propDef{
						"name":        strProp("Fully-qualified function name"),
						"description": strProp("Human description. Empty string clears."),
						"loc":         strProp("Override loc. Empty string clears."),
						"depends":     arrProp("Override depends list. Empty array clears."),
						"test":        arrProp("Override test list. Empty array clears."),
					},
					Required: []string{"name"},
				},
			},
		},
	}
}

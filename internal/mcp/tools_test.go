package mcp

import (
	"encoding/json"
	"reflect"
	"testing"
)

// golden captures the semantically expected toolsList output.
// Verified against the original map[string]interface{} implementation.
const toolsListGolden = `{"tools":[{"description":"Get function metadata from the KV index. Returns loc, description, depends, test fields. If stale is true, the source file no longer exists.","inputSchema":{"properties":{"name":{"description":"Fully-qualified function name (e.g. 'pkg.Func' or 'pkg.(*Receiver).Method'). Partial names return all matches.","type":"string"}},"required":["name"],"type":"object"},"name":"get_function"},{"description":"Search functions by name or description substring (case-insensitive). Returns up to 50 matches.","inputSchema":{"properties":{"query":{"description":"Substring to search for in function names and descriptions.","type":"string"}},"required":["query"],"type":"object"},"name":"search"},{"description":"Read source lines for a loc string (e.g. 'path/file.go:10-30'). Use the loc from get_function to stay within the --max-lines cap.","inputSchema":{"properties":{"loc":{"description":"Location string: 'relpath/file.go:N' or 'relpath/file.go:N-M'","type":"string"}},"required":["loc"],"type":"object"},"name":"get_code"},{"description":"Update curated metadata for a function. Curated data persists across reindexing.\n\nMerge contract for loc, depends, test:\n- absent/null: leave existing extracted value unchanged\n- empty string or empty array: clear curated override (extracted becomes authoritative)\n- non-empty value: replace curated override (wins over extracted on read)\n\nFor description: empty string clears, non-empty replaces.","inputSchema":{"properties":{"depends":{"description":"Override depends list. Empty array clears.","items":{"type":"string"},"type":"array"},"description":{"description":"Human description. Empty string clears.","type":"string"},"loc":{"description":"Override loc. Empty string clears.","type":"string"},"name":{"description":"Fully-qualified function name","type":"string"},"test":{"description":"Override test list. Empty array clears.","items":{"type":"string"},"type":"array"}},"required":["name"],"type":"object"},"name":"update_function"}]}`

func TestToolsListGolden(t *testing.T) {
	got, err := json.Marshal(toolsList())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var gotVal, wantVal interface{}
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal([]byte(toolsListGolden), &wantVal); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Errorf("toolsList() JSON mismatch\ngot:  %s\nwant: %s", got, toolsListGolden)
	}
}

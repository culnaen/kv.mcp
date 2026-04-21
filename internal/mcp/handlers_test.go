package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/culnaen/kv.mcp/internal/kv"
)

// --- test helpers ---

// newTempStore opens a fresh store at a per-test path and registers cleanup.
func newTempStore(t *testing.T) (kv.Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := kv.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, dir
}

// writeFile writes content to root/rel and returns the rel path.
func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return rel
}

// seedExtracted inserts an ExtractedFunction.
func seedExtracted(t *testing.T, store kv.Store, f kv.ExtractedFunction) {
	t.Helper()
	if err := store.PutExtracted(f); err != nil {
		t.Fatalf("put extracted: %v", err)
	}
}

// callTool invokes a handler via the server dispatch path and returns the
// parsed inner JSON payload (the content[0].text string, json-decoded) or an error.
func callTool(t *testing.T, s *Server, name string, args interface{}) (map[string]interface{}, *jsonrpcError) {
	t.Helper()
	argData, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	params, err := json.Marshal(toolsCallParams{Name: name, Arguments: argData})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	result, rpcErr := s.handleToolsCall(params)
	if rpcErr != nil {
		return nil, rpcErr
	}
	return decodeToolResult(t, result)
}

// callToolRaw is like callTool but args is already-encoded JSON (to test absent fields).
func callToolRaw(t *testing.T, s *Server, name string, rawArgs string) (map[string]interface{}, *jsonrpcError) {
	t.Helper()
	params, err := json.Marshal(toolsCallParams{Name: name, Arguments: json.RawMessage(rawArgs)})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	result, rpcErr := s.handleToolsCall(params)
	if rpcErr != nil {
		return nil, rpcErr
	}
	return decodeToolResult(t, result)
}

// decodeToolResult peels the MCP content wrapper and returns the JSON body.
func decodeToolResult(t *testing.T, result interface{}) (map[string]interface{}, *jsonrpcError) {
	t.Helper()
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	content, ok := m["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("expected content array, got %+v", m)
	}
	block, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected content block map, got %T", content[0])
	}
	text, ok := block["text"].(string)
	if !ok {
		t.Fatalf("expected text string, got %T", block["text"])
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("decode tool result json: %v (raw=%s)", err, text)
	}
	return out, nil
}

// --- tests ---

func TestGetFunctionReturnsMerged(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "pkg/a.go", "line1\nline2\nline3\n")
	seedExtracted(t, store, kv.ExtractedFunction{
		Name:      "pkg.Foo",
		Loc:       "pkg/a.go:1-3",
		GodocStub: "Foo does things.",
	})
	// Add a curated description override.
	if err := store.PutCurated(kv.CuratedFunction{Name: "pkg.Foo", Description: "curated desc"}); err != nil {
		t.Fatalf("put curated: %v", err)
	}

	s := NewServer(store, root, 500)
	out, rpcErr := callTool(t, s, "get_function", map[string]string{"name": "pkg.Foo"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if out["name"] != "pkg.Foo" {
		t.Errorf("name=%v", out["name"])
	}
	if out["description"] != "curated desc" {
		t.Errorf("description=%v want 'curated desc'", out["description"])
	}
	if out["loc"] != "pkg/a.go:1-3" {
		t.Errorf("loc=%v", out["loc"])
	}
}

func TestGetFunctionMissingReturnsError(t *testing.T) {
	store, root := newTempStore(t)
	s := NewServer(store, root, 500)

	_, rpcErr := callTool(t, s, "get_function", map[string]string{"name": "does.not.exist"})
	if rpcErr == nil {
		t.Fatalf("expected rpc error")
	}
	if rpcErr.Code != codeInvalidParams {
		t.Errorf("code=%d want %d", rpcErr.Code, codeInvalidParams)
	}
}

func TestGetFunctionPartialSingleMatch(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.FooBar", Loc: "a.go:1-1", GodocStub: "FooBar stub."})
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Baz", Loc: "a.go:1-1", GodocStub: "Baz stub."})
	s := NewServer(store, root, 500)

	// Exact lookup of "FooBar" fails; fallback finds pkg.FooBar as single match.
	out, rpcErr := callTool(t, s, "get_function", map[string]string{"name": "FooBar"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if out["name"] != "pkg.FooBar" {
		t.Errorf("name=%v want pkg.FooBar", out["name"])
	}
}

func TestGetFunctionPartialMultiMatch(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.FooAlpha", Loc: "a.go:1-1"})
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.FooBeta", Loc: "a.go:1-1"})
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Bar", Loc: "a.go:1-1"})
	s := NewServer(store, root, 500)

	// Exact lookup of "Foo" fails; fallback finds 2 matches.
	out, rpcErr := callTool(t, s, "get_function", map[string]string{"name": "Foo"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	matches, ok := out["matches"].([]interface{})
	if !ok {
		t.Fatalf("expected matches array, got %T: %v", out["matches"], out)
	}
	if len(matches) != 2 {
		t.Errorf("matches count=%d want 2", len(matches))
	}
}

func TestSearchSubstringMatches(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Alpha", Loc: "a.go:1-1", GodocStub: "Alpha func."})
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Beta", Loc: "a.go:1-1", GodocStub: "Does alpha-related things."})
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Gamma", Loc: "a.go:1-1", GodocStub: "Unrelated."})

	s := NewServer(store, root, 500)
	out, rpcErr := callTool(t, s, "search", map[string]string{"query": "alpha"})
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	count, _ := out["count"].(float64)
	if int(count) != 2 {
		t.Errorf("count=%v want 2", out["count"])
	}
}

func TestGetFunctionStale(t *testing.T) {
	store, root := newTempStore(t)
	// loc points to a file that does NOT exist → stale=true
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Gone", Loc: "pkg/gone.go:1-5"})
	s := NewServer(store, root, 500)

	out, rpcErr := callTool(t, s, "get_function", map[string]string{"name": "pkg.Gone"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if out["stale"] != true {
		t.Errorf("stale=%v want true", out["stale"])
	}
}

func TestGetFunctionNotStale(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "pkg/exists.go", "line1\nline2\nline3\nline4\nline5\n")
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Alive", Loc: "pkg/exists.go:1-5"})
	s := NewServer(store, root, 500)

	out, rpcErr := callTool(t, s, "get_function", map[string]string{"name": "pkg.Alive"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if stale, ok := out["stale"]; ok && stale == true {
		t.Errorf("stale=%v want false/absent for existing file", stale)
	}
}

func TestSearchCacheInvalidatedOnUpdate(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Widget", Loc: "a.go:1-1", GodocStub: "original."})
	s := NewServer(store, root, 500)

	// Prime cache: no matches for "curated".
	out, _ := callTool(t, s, "search", map[string]string{"query": "curated"})
	if c, _ := out["count"].(float64); int(c) != 0 {
		t.Fatalf("expected 0 initial matches, got %v", out["count"])
	}

	// Update curated description -> should invalidate cache.
	desc := "curated prose"
	_, rpcErr := callTool(t, s, "update_function", map[string]interface{}{
		"name":        "pkg.Widget",
		"description": desc,
	})
	if rpcErr != nil {
		t.Fatalf("update rpc error: %+v", rpcErr)
	}

	out, _ = callTool(t, s, "search", map[string]string{"query": "curated"})
	if c, _ := out["count"].(float64); int(c) != 1 {
		t.Errorf("after update expected 1 match, got %v", out["count"])
	}
}

func TestGetCodeReturnsLines(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "src/main.go", "line1\nline2\nline3\nline4\nline5\n")
	s := NewServer(store, root, 500)

	out, rpcErr := callTool(t, s, "get_code", map[string]string{"loc": "src/main.go:2-4"})
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	if out["content"] != "line2\nline3\nline4" {
		t.Errorf("content=%q", out["content"])
	}
	if out["loc"] != "src/main.go:2-4" {
		t.Errorf("loc=%v", out["loc"])
	}
}

func TestGetCodeExceedingMaxLinesErrors(t *testing.T) {
	store, root := newTempStore(t)
	// 10 lines total.
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	writeFile(t, root, "big.go", b.String())
	s := NewServer(store, root, 5) // cap at 5

	_, rpcErr := callTool(t, s, "get_code", map[string]string{"loc": "big.go:1-10"})
	if rpcErr == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(rpcErr.Message, "response truncated") {
		t.Errorf("message=%q want contains 'response truncated'", rpcErr.Message)
	}
	if !strings.Contains(rpcErr.Message, "max-lines cap of 5") {
		t.Errorf("message=%q want includes cap", rpcErr.Message)
	}
}

func TestUpdateFunctionAbsentLeavesExisting(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{
		Name:    "pkg.Foo",
		Loc:     "a.go:1-1",
		Depends: []string{"bar"},
	})
	s := NewServer(store, root, 500)

	// Update only description; loc/depends absent.
	_, rpcErr := callToolRaw(t, s, "update_function", `{"name":"pkg.Foo","description":"hello"}`)
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}

	out, rpcErr := callTool(t, s, "get_function", map[string]string{"name": "pkg.Foo"})
	if rpcErr != nil {
		t.Fatalf("get rpc error: %+v", rpcErr)
	}
	if out["loc"] != "a.go:1-1" {
		t.Errorf("loc should be unchanged, got %v", out["loc"])
	}
	deps, _ := out["depends"].([]interface{})
	if len(deps) != 1 || deps[0] != "bar" {
		t.Errorf("depends should be unchanged, got %v", out["depends"])
	}
	if out["description"] != "hello" {
		t.Errorf("description=%v", out["description"])
	}
}

func TestUpdateFunctionEmptyArrayClearsDepends(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{
		Name:    "pkg.Foo",
		Loc:     "a.go:1-1",
		Depends: []string{"bar", "baz"},
	})
	s := NewServer(store, root, 500)

	_, rpcErr := callToolRaw(t, s, "update_function", `{"name":"pkg.Foo","depends":[]}`)
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}

	out, _ := callTool(t, s, "get_function", map[string]string{"name": "pkg.Foo"})
	if deps := out["depends"]; deps != nil {
		// After clearing the override, the extracted depends should be authoritative again.
		// Per MergeFunction: *c.Depends with len==0 clears to nil.
		// So merged Depends should be nil -> json omit or null.
		if arr, ok := deps.([]interface{}); ok && len(arr) != 0 {
			t.Errorf("expected cleared depends, got %v", deps)
		}
	}
}

func TestUpdateFunctionNonEmptyReplaces(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{
		Name:    "pkg.Foo",
		Loc:     "a.go:1-1",
		Depends: []string{"bar"},
	})
	s := NewServer(store, root, 500)

	_, rpcErr := callToolRaw(t, s, "update_function", `{"name":"pkg.Foo","depends":["x","y"]}`)
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}

	out, _ := callTool(t, s, "get_function", map[string]string{"name": "pkg.Foo"})
	deps, _ := out["depends"].([]interface{})
	if len(deps) != 2 || deps[0] != "x" || deps[1] != "y" {
		t.Errorf("depends=%v", out["depends"])
	}
}

func TestUpdateFunctionEmptyDescriptionClears(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	seedExtracted(t, store, kv.ExtractedFunction{Name: "pkg.Foo", Loc: "a.go:1-1", GodocStub: "extracted stub."})
	if err := store.PutCurated(kv.CuratedFunction{Name: "pkg.Foo", Description: "curated"}); err != nil {
		t.Fatalf("seed curated: %v", err)
	}
	s := NewServer(store, root, 500)

	// Set description="" via update -> clears curated override, extracted stub wins.
	_, rpcErr := callToolRaw(t, s, "update_function", `{"name":"pkg.Foo","description":""}`)
	if rpcErr != nil {
		t.Fatalf("rpc error: %+v", rpcErr)
	}
	out, _ := callTool(t, s, "get_function", map[string]string{"name": "pkg.Foo"})
	if out["description"] != "extracted stub." {
		t.Errorf("description=%v want 'extracted stub.'", out["description"])
	}
}

func TestConcurrentSearchAndUpdate(t *testing.T) {
	store, root := newTempStore(t)
	writeFile(t, root, "a.go", "x\n")
	for i := 0; i < 20; i++ {
		seedExtracted(t, store, kv.ExtractedFunction{
			Name:      fmt.Sprintf("pkg.Func%d", i),
			Loc:       "a.go:1-1",
			GodocStub: fmt.Sprintf("stub %d", i),
		})
	}
	s := NewServer(store, root, 500)

	stop := make(chan struct{})
	var readerWg, writerWg sync.WaitGroup

	for i := 0; i < 4; i++ {
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = callTool(t, s, "search", map[string]string{"query": "stub"})
				}
			}
		}()
	}

	for i := 0; i < 2; i++ {
		writerWg.Add(1)
		go func(id int) {
			defer writerWg.Done()
			for j := 0; j < 20; j++ {
				raw := fmt.Sprintf(`{"name":"pkg.Func%d","description":"writer%d-%d"}`, j%20, id, j)
				_, _ = callToolRaw(t, s, "update_function", raw)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		writerWg.Wait()
		close(stop)
		readerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestConcurrentSearchAndUpdate timed out")
	}
}

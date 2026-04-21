package index

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/culnaen/kv.mcp/internal/kv"
)

func openTestStore(t *testing.T) kv.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := kv.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func collectExtracted(t *testing.T, store kv.Store) map[string]kv.ExtractedFunction {
	t.Helper()
	out := map[string]kv.ExtractedFunction{}
	err := store.ScanExtracted(func(f kv.ExtractedFunction) bool {
		out[f.Name] = f
		return true
	})
	if err != nil && err.Error() != "stop" {
		t.Fatalf("scan: %v", err)
	}
	return out
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestIndexFixtures(t *testing.T) {
	store := openTestStore(t)
	count, err := IndexRoot("fixtures", store, false, "")
	if err != nil {
		t.Fatalf("IndexRoot: %v", err)
	}
	if count < 3 {
		t.Fatalf("expected at least 3 indexed functions, got %d", count)
	}

	got := collectExtracted(t, store)

	greet, ok := got["example.Greet"]
	if !ok {
		t.Fatalf("example.Greet not indexed; got %v", keys(got))
	}
	if !strings.Contains(greet.GodocStub, "Greet") {
		t.Errorf("example.Greet godoc stub missing 'Greet': %q", greet.GodocStub)
	}
	if !strings.Contains(greet.Loc, "example/example.go:") {
		t.Errorf("example.Greet loc unexpected: %q", greet.Loc)
	}

	start, ok := got["example.(*Server).Start"]
	if !ok {
		t.Fatalf("example.(*Server).Start not indexed; got %v", keys(got))
	}
	if !containsStr(start.Depends, "example.Greet") {
		t.Errorf("Start.Depends should contain example.Greet, got %v", start.Depends)
	}

	// TestGreet's loc should be attached to example.Greet's Test slice.
	foundTest := false
	for _, loc := range greet.Test {
		if strings.Contains(loc, "example_test.go:") {
			foundTest = true
			break
		}
	}
	if !foundTest {
		t.Errorf("example.Greet should have TestGreet attached, got Test=%v", greet.Test)
	}

	add, ok := got["example.Add"]
	if !ok {
		t.Fatalf("example.Add not indexed")
	}
	foundAddTest := false
	for _, loc := range add.Test {
		if strings.Contains(loc, "example_test.go:") {
			foundAddTest = true
			break
		}
	}
	if !foundAddTest {
		t.Errorf("example.Add should have TestAdd attached, got Test=%v", add.Test)
	}
}

func TestIdempotent(t *testing.T) {
	store := openTestStore(t)
	if _, err := IndexRoot("fixtures", store, false, ""); err != nil {
		t.Fatalf("first IndexRoot: %v", err)
	}
	first := collectExtracted(t, store)

	if _, err := IndexRoot("fixtures", store, false, ""); err != nil {
		t.Fatalf("second IndexRoot: %v", err)
	}
	second := collectExtracted(t, store)

	if len(first) != len(second) {
		t.Fatalf("entry count changed: %d -> %d", len(first), len(second))
	}
	for name, a := range first {
		b, ok := second[name]
		if !ok {
			t.Errorf("entry %s disappeared on reindex", name)
			continue
		}
		if a.Loc != b.Loc || a.GodocStub != b.GodocStub {
			t.Errorf("entry %s changed between runs: %+v vs %+v", name, a, b)
		}
		if !sliceEqual(a.Depends, b.Depends) {
			t.Errorf("entry %s depends changed: %v vs %v", name, a.Depends, b.Depends)
		}
		if !sliceEqual(a.Test, b.Test) {
			t.Errorf("entry %s test changed: %v vs %v", name, a.Test, b.Test)
		}
	}
}

func TestPreservesCurated(t *testing.T) {
	store := openTestStore(t)

	curated := kv.CuratedFunction{
		Name:        "example.Greet",
		Description: "custom curated description",
	}
	if err := store.PutCurated(curated); err != nil {
		t.Fatalf("seed curated: %v", err)
	}

	if _, err := IndexRoot("fixtures", store, false, ""); err != nil {
		t.Fatalf("IndexRoot: %v", err)
	}

	got, ok, err := store.GetCurated("example.Greet")
	if err != nil {
		t.Fatalf("GetCurated: %v", err)
	}
	if !ok {
		t.Fatalf("curated entry lost after reindex")
	}
	if got.Description != "custom curated description" {
		t.Errorf("curated description changed: %q", got.Description)
	}
}

func keys(m map[string]kv.ExtractedFunction) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

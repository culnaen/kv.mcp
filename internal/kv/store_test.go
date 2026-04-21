package kv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) (Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func strPtr(s string) *string { return &s }

func sliceRef(s []string) *[]string { return &s }

func collectExtracted(t *testing.T, s Store) []ExtractedFunction {
	t.Helper()
	var out []ExtractedFunction
	if err := s.ScanExtracted(func(f ExtractedFunction) bool {
		out = append(out, f)
		return true
	}); err != nil {
		t.Fatalf("ScanExtracted: %v", err)
	}
	return out
}


func TestPutAndScanExtracted(t *testing.T) {
	s, _ := newTestStore(t)
	e1 := ExtractedFunction{
		Name:      "pkg.Foo",
		Loc:       "pkg/foo.go:10-20",
		Depends:   []string{"pkg.Bar"},
		Test:      []string{"pkg.TestFoo"},
		GodocStub: "Foo does something.",
	}
	e2 := ExtractedFunction{
		Name: "pkg.Bar",
		Loc:  "pkg/bar.go:5-8",
	}
	if err := s.PutExtracted(e1); err != nil {
		t.Fatalf("PutExtracted e1: %v", err)
	}
	if err := s.PutExtracted(e2); err != nil {
		t.Fatalf("PutExtracted e2: %v", err)
	}
	got := collectExtracted(t, s)
	if len(got) != 2 {
		t.Fatalf("expected 2 extracted, got %d", len(got))
	}
	// bbolt iterates in key order
	if got[0].Name != "pkg.Bar" || got[1].Name != "pkg.Foo" {
		t.Errorf("unexpected iteration order: %v, %v", got[0].Name, got[1].Name)
	}
	if !reflect.DeepEqual(got[1], e1) {
		t.Errorf("e1 round-trip mismatch: %+v vs %+v", got[1], e1)
	}
}

func TestPutAndGetCurated(t *testing.T) {
	s, _ := newTestStore(t)
	c := CuratedFunction{
		Name:        "pkg.Foo",
		Description: "curated description",
		Depends:     sliceRef([]string{"pkg.Baz"}),
	}
	if err := s.PutCurated(c); err != nil {
		t.Fatalf("PutCurated: %v", err)
	}
	got, ok, err := s.GetCurated("pkg.Foo")
	if err != nil {
		t.Fatalf("GetCurated: %v", err)
	}
	if !ok {
		t.Fatal("GetCurated: not found")
	}
	if got.Description != "curated description" {
		t.Errorf("description: got %q", got.Description)
	}
	if got.Depends == nil || !reflect.DeepEqual(*got.Depends, []string{"pkg.Baz"}) {
		t.Errorf("depends: got %+v", got.Depends)
	}

	// Missing key
	_, ok, err = s.GetCurated("missing")
	if err != nil {
		t.Fatalf("GetCurated missing: %v", err)
	}
	if ok {
		t.Error("expected missing=false")
	}
}

func TestGetMergedExtractedOnly(t *testing.T) {
	s, root := newTestStore(t)
	// ensure the loc file exists so Stale=false
	writeLocFile(t, root, "pkg/foo.go", 20)
	e := ExtractedFunction{
		Name:      "pkg.Foo",
		Loc:       "pkg/foo.go:10-20",
		Depends:   []string{"pkg.Bar"},
		Test:      []string{"pkg.TestFoo"},
		GodocStub: "Foo stub.",
	}
	if err := s.PutExtracted(e); err != nil {
		t.Fatalf("PutExtracted: %v", err)
	}
	f, ok, err := s.GetMerged("pkg.Foo", root)
	if err != nil {
		t.Fatalf("GetMerged: %v", err)
	}
	if !ok {
		t.Fatal("GetMerged: not found")
	}
	if f.Description != "Foo stub." {
		t.Errorf("description: got %q want godoc stub", f.Description)
	}
	if f.Loc != "pkg/foo.go:10-20" {
		t.Errorf("loc: got %q", f.Loc)
	}
	if f.Stale {
		t.Error("expected Stale=false when file exists")
	}
}

func TestGetMergedCuratedDescriptionWins(t *testing.T) {
	s, root := newTestStore(t)
	writeLocFile(t, root, "pkg/foo.go", 20)
	_ = s.PutExtracted(ExtractedFunction{
		Name: "pkg.Foo", Loc: "pkg/foo.go:10-20", GodocStub: "stub",
	})
	_ = s.PutCurated(CuratedFunction{
		Name: "pkg.Foo", Description: "curated wins",
	})
	f, _, _ := s.GetMerged("pkg.Foo", root)
	if f.Description != "curated wins" {
		t.Errorf("description: got %q", f.Description)
	}
}

func TestGetMergedLocNil(t *testing.T) {
	s, root := newTestStore(t)
	writeLocFile(t, root, "pkg/foo.go", 20)
	_ = s.PutExtracted(ExtractedFunction{Name: "pkg.Foo", Loc: "pkg/foo.go:10-20"})
	_ = s.PutCurated(CuratedFunction{Name: "pkg.Foo", Description: "d"})
	f, _, _ := s.GetMerged("pkg.Foo", root)
	if f.Loc != "pkg/foo.go:10-20" {
		t.Errorf("loc should come from extracted, got %q", f.Loc)
	}
}

func TestGetMergedLocClearedEmptyString(t *testing.T) {
	s, root := newTestStore(t)
	writeLocFile(t, root, "pkg/foo.go", 20)
	_ = s.PutExtracted(ExtractedFunction{Name: "pkg.Foo", Loc: "pkg/foo.go:10-20"})
	_ = s.PutCurated(CuratedFunction{Name: "pkg.Foo", Loc: strPtr("")})
	f, _, _ := s.GetMerged("pkg.Foo", root)
	if f.Loc != "pkg/foo.go:10-20" {
		t.Errorf("loc should fall back to extracted when curated is empty, got %q", f.Loc)
	}
}

func TestGetMergedLocOverride(t *testing.T) {
	s, root := newTestStore(t)
	writeLocFile(t, root, "new/path.go", 10)
	_ = s.PutExtracted(ExtractedFunction{Name: "pkg.Foo", Loc: "pkg/foo.go:10-20"})
	_ = s.PutCurated(CuratedFunction{Name: "pkg.Foo", Loc: strPtr("new/path.go:5")})
	f, _, _ := s.GetMerged("pkg.Foo", root)
	if f.Loc != "new/path.go:5" {
		t.Errorf("loc should be curated override, got %q", f.Loc)
	}
}

func TestGetMergedDependsClearedEmptySlice(t *testing.T) {
	s, root := newTestStore(t)
	writeLocFile(t, root, "pkg/foo.go", 20)
	_ = s.PutExtracted(ExtractedFunction{
		Name: "pkg.Foo", Loc: "pkg/foo.go:10-20", Depends: []string{"pkg.Bar"},
	})
	_ = s.PutCurated(CuratedFunction{Name: "pkg.Foo", Depends: sliceRef([]string{})})
	f, _, _ := s.GetMerged("pkg.Foo", root)
	if f.Depends != nil {
		t.Errorf("depends should be cleared to nil, got %+v", f.Depends)
	}
}

func TestGetMergedDependsNil(t *testing.T) {
	s, root := newTestStore(t)
	writeLocFile(t, root, "pkg/foo.go", 20)
	_ = s.PutExtracted(ExtractedFunction{
		Name: "pkg.Foo", Loc: "pkg/foo.go:10-20", Depends: []string{"pkg.Bar"},
	})
	_ = s.PutCurated(CuratedFunction{Name: "pkg.Foo", Description: "d"})
	f, _, _ := s.GetMerged("pkg.Foo", root)
	if !reflect.DeepEqual(f.Depends, []string{"pkg.Bar"}) {
		t.Errorf("depends should come from extracted, got %+v", f.Depends)
	}
}

func TestClearExtractedPreservesCurated(t *testing.T) {
	s, _ := newTestStore(t)
	_ = s.PutExtracted(ExtractedFunction{Name: "pkg.Foo", Loc: "pkg/foo.go:1-5"})
	_ = s.PutCurated(CuratedFunction{Name: "pkg.Foo", Description: "keep me"})

	if err := s.ClearExtracted(); err != nil {
		t.Fatalf("ClearExtracted: %v", err)
	}

	if got := collectExtracted(t, s); len(got) != 0 {
		t.Errorf("expected extracted wiped, got %d entries", len(got))
	}
	if _, ok, err := s.GetCurated("pkg.Foo"); err != nil || !ok {
		t.Errorf("expected curated preserved: ok=%v err=%v", ok, err)
	}
}

func TestStaleFlag(t *testing.T) {
	s, root := newTestStore(t)
	// loc points to file that does NOT exist
	_ = s.PutExtracted(ExtractedFunction{
		Name: "pkg.Gone", Loc: "pkg/gone.go:1-5",
	})
	f, ok, err := s.GetMerged("pkg.Gone", root)
	if err != nil {
		t.Fatalf("GetMerged: %v", err)
	}
	if !ok {
		t.Fatal("not found")
	}
	if !f.Stale {
		t.Error("expected Stale=true for missing file")
	}
}

func TestScanMerged(t *testing.T) {
	s, root := newTestStore(t)
	writeLocFile(t, root, "a.go", 10)
	writeLocFile(t, root, "b.go", 10)
	_ = s.PutExtracted(ExtractedFunction{Name: "A", Loc: "a.go:1-5", GodocStub: "A stub"})
	_ = s.PutExtracted(ExtractedFunction{Name: "B", Loc: "b.go:1-5", GodocStub: "B stub"})
	_ = s.PutCurated(CuratedFunction{Name: "B", Description: "curated B"})

	seen := map[string]string{}
	if err := s.ScanMerged(root, func(f Function) bool {
		seen[f.Name] = f.Description
		return true
	}); err != nil {
		t.Fatalf("ScanMerged: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 merged entries, got %d", len(seen))
	}
	if seen["A"] != "A stub" {
		t.Errorf("A: got %q want godoc stub", seen["A"])
	}
	if seen["B"] != "curated B" {
		t.Errorf("B: got %q want curated override", seen["B"])
	}
}

// writeLocFile creates a file with n blank lines at root/relpath.
func writeLocFile(t *testing.T, root, rel string, n int) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := strings.Repeat("line\n", n)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}
}

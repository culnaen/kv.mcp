package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkPackageDirsSkipsIgnored(t *testing.T) {
	root := t.TempDir()

	mk := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	mk("top/a.go", "package top\n")
	mk("top/sub/b.go", "package sub\n")
	mk("top/vendor/v.go", "package v\n")
	mk("top/testdata/td.go", "package td\n")
	mk("top/_skip/s.go", "package skip\n")
	mk("top/.hidden/h.go", "package hidden\n")
	mk("top/readme.txt", "not go\n")

	dirs, err := WalkPackageDirs(root)
	if err != nil {
		t.Fatalf("WalkPackageDirs: %v", err)
	}

	has := func(rel string) bool {
		want := filepath.Join(root, rel)
		for _, d := range dirs {
			if d == want {
				return true
			}
		}
		return false
	}

	if !has("top") {
		t.Errorf("expected top/ in dirs, got %v", dirs)
	}
	if !has("top/sub") {
		t.Errorf("expected top/sub in dirs, got %v", dirs)
	}
	for _, d := range dirs {
		low := strings.ToLower(d)
		if strings.Contains(low, "vendor") || strings.Contains(low, "testdata") ||
			strings.Contains(d, "/_skip") || strings.Contains(d, "/.hidden") {
			t.Errorf("unexpected dir in result: %s", d)
		}
	}
}

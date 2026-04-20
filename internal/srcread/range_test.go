package srcread

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string, lines int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		b.WriteString("line")
		b.WriteByte(byte('0' + (i % 10)))
		b.WriteByte('\n')
	}
	// trim trailing newline for a cleaner EOF
	s := strings.TrimRight(b.String(), "\n")
	if err := os.WriteFile(path, []byte(s), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}
}

func TestReadSingleLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), 10)
	got, err := Read(dir, "a.go:3")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != "line3" {
		t.Errorf("got %q, want %q", got, "line3")
	}
}

func TestReadRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), 10)
	got, err := Read(dir, "a.go:1-5")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d: %q", len(lines), got)
	}
	if lines[0] != "line1" || lines[4] != "line5" {
		t.Errorf("lines: %+v", lines)
	}
}

func TestReadFullFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), 3)
	got, err := Read(dir, "a.go:1-3")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != "line1\nline2\nline3" {
		t.Errorf("got %q", got)
	}
}

func TestReadStartZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), 10)
	_, err := Read(dir, "a.go:0")
	if err == nil {
		t.Fatal("expected error for line 0")
	}
	if !strings.Contains(err.Error(), ">= 1") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadEndBeforeStart(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), 10)
	_, err := Read(dir, "a.go:5-3")
	if err == nil {
		t.Fatal("expected error for end<start")
	}
	if !strings.Contains(err.Error(), "end line") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadBeyondEOF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), 5)
	_, err := Read(dir, "a.go:3-10")
	if err == nil {
		t.Fatal("expected error for end past EOF")
	}
	if !strings.Contains(err.Error(), "file has") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Read(dir, "missing.go:1")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist, got %v", err)
	}
}

func TestReadRootResolvesRelative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub", "a.go"), 3)
	got, err := Read(dir, "sub/a.go:2")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != "line2" {
		t.Errorf("got %q", got)
	}
}

func TestReadInvalidLoc(t *testing.T) {
	_, err := Read("", "no-colon")
	if err == nil {
		t.Fatal("expected error for missing colon")
	}
}

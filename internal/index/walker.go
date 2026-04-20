package index

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// WalkPackageDirs returns unique package directories under root,
// skipping vendor/, testdata/, and dirs starting with _ or .
func WalkPackageDirs(root string) ([]string, error) {
	seen := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// Never skip the root itself based on its own name.
			if path == root {
				return nil
			}
			if name == "vendor" || name == "testdata" {
				return fs.SkipDir
			}
			if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		dir := filepath.Dir(path)
		seen[dir] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs, nil
}

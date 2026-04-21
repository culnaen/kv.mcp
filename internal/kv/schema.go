package kv

import "os"

// fileExists is a package-level variable so tests can override filesystem checks.
var fileExists = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const (
	BucketExtracted = "functions_extracted"
	BucketCurated   = "functions_curated"
)

// ExtractedFunction written only by indexer, overwritten on every reindex.
type ExtractedFunction struct {
	Name      string   `json:"name"`
	Loc       string   `json:"loc"`
	Depends   []string `json:"depends"`
	Test      []string `json:"test"`
	GodocStub string   `json:"godoc_stub"`
}

// CuratedFunction written only by update_function MCP tool, preserved across reindex.
// Pointer fields encode the merge contract:
//
//	nil   → leave existing extracted value
//	ptr to "" / ptr to []  → clear curated override (extracted becomes authoritative)
//	ptr to non-empty value → replace (wins over extracted on read)
type CuratedFunction struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Loc         *string   `json:"loc,omitempty"`
	Depends     *[]string `json:"depends,omitempty"`
	Test        *[]string `json:"test,omitempty"`
}

// Function is the merged DTO returned by get_function and search.
type Function struct {
	Name        string   `json:"name"`
	Loc         string   `json:"loc"`
	Description string   `json:"description"`
	Depends     []string `json:"depends"`
	Test        []string `json:"test"`
	Stale       bool     `json:"stale,omitempty"`
}

// MergeFunction assembles a Function from extracted + optional curated.
// c may be nil (no curated entry).
func MergeFunction(e ExtractedFunction, c *CuratedFunction, root string) Function {
	f := Function{
		Name:    e.Name,
		Loc:     e.Loc,
		Depends: e.Depends,
		Test:    e.Test,
	}

	// description: curated wins if non-empty, else godoc stub
	f.Description = e.GodocStub
	if c != nil {
		if c.Description != "" {
			f.Description = c.Description
		}
		// loc override
		if c.Loc != nil {
			if *c.Loc != "" {
				f.Loc = *c.Loc
			}
			// *c.Loc == "" means clear — use extracted (already set)
		}
		// depends override
		if c.Depends != nil {
			if len(*c.Depends) > 0 {
				f.Depends = *c.Depends
			} else {
				f.Depends = nil // cleared
			}
		}
		// test override
		if c.Test != nil {
			if len(*c.Test) > 0 {
				f.Test = *c.Test
			} else {
				f.Test = nil // cleared
			}
		}
	}

	// stale check: if loc file no longer exists
	filePath := locFilePath(f.Loc, root)
	if filePath != "" && !fileExists(filePath) {
		f.Stale = true
	}

	return f
}


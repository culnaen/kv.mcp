package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/culnaen/kv.mcp/internal/kv"
	"github.com/culnaen/kv.mcp/internal/srcread"
)

// searchCache holds the in-memory search index.
type searchCache struct {
	mu      sync.RWMutex
	entries []kv.Function // nil means not built yet
}

// search returns matches and a boolean indicating whether the cache was populated.
func (c *searchCache) search(query string) ([]kv.Function, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.entries == nil {
		return nil, false
	}
	q := strings.ToLower(query)
	var results []kv.Function
	for _, f := range c.entries {
		if strings.Contains(strings.ToLower(f.Name), q) ||
			strings.Contains(strings.ToLower(f.Description), q) {
			results = append(results, f)
			if len(results) >= 50 {
				break
			}
		}
	}
	return results, true
}

// rebuild replaces the cache contents by scanning the store.
func (c *searchCache) rebuild(store kv.Store, root string) error {
	var entries []kv.Function
	err := store.ScanMerged(root, func(f kv.Function) bool {
		entries = append(entries, f)
		return true
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.entries = entries
	c.mu.Unlock()
	return nil
}

// invalidate drops the cache so the next search rebuilds it.
func (c *searchCache) invalidate() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// --- tool arg structs ---

type getFunctionArgs struct {
	Name string `json:"name"`
}

type searchArgs struct {
	Query string `json:"query"`
}

type getCodeArgs struct {
	Loc string `json:"loc"`
}

// updateFunctionArgs decodes the update_function payload while preserving
// the tri-state semantics (absent / empty / non-empty) for loc, depends, test.
type updateFunctionArgs struct {
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Loc         json.RawMessage `json:"loc,omitempty"`
	Depends     json.RawMessage `json:"depends,omitempty"`
	Test        json.RawMessage `json:"test,omitempty"`
}

// --- handlers ---

// handleGetFunction returns the merged record for name, or partial matches when exact lookup fails.
func (s *Server) handleGetFunction(args json.RawMessage) (interface{}, *jsonrpcError) {
	var a getFunctionArgs
	if err := unmarshalArgs(args, &a); err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "name is required"}
	}

	f, ok, err := s.store.GetMerged(a.Name, s.root)
	if err != nil {
		return nil, &jsonrpcError{Code: codeInternalError, Message: "get_merged: " + err.Error()}
	}
	if ok {
		return toolResult(f), nil
	}

	// Fallback: partial match on name (substring).
	var matches []kv.Function
	q := strings.ToLower(a.Name)
	scanErr := s.store.ScanMerged(s.root, func(fn kv.Function) bool {
		if strings.Contains(strings.ToLower(fn.Name), q) {
			matches = append(matches, fn)
			if len(matches) >= 50 {
				return false
			}
		}
		return true
	})
	if scanErr != nil && scanErr.Error() != "stop" {
		return nil, &jsonrpcError{Code: codeInternalError, Message: "scan: " + scanErr.Error()}
	}
	if len(matches) == 0 {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: fmt.Sprintf("function not found: %s", a.Name)}
	}
	if len(matches) == 1 {
		return toolResult(matches[0]), nil
	}
	return toolResult(map[string]interface{}{"matches": matches}), nil
}

// handleSearch returns up to 50 matches. Builds the cache lazily.
func (s *Server) handleSearch(args json.RawMessage) (interface{}, *jsonrpcError) {
	var a searchArgs
	if err := unmarshalArgs(args, &a); err != nil {
		return nil, err
	}
	if a.Query == "" {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "query is required"}
	}

	results, built := s.cache.search(a.Query)
	if !built {
		if err := s.cache.rebuild(s.store, s.root); err != nil {
			return nil, &jsonrpcError{Code: codeInternalError, Message: "cache rebuild: " + err.Error()}
		}
		results, _ = s.cache.search(a.Query)
	}

	payload := map[string]interface{}{
		"matches": results,
		"count":   len(results),
	}
	if len(results) >= 50 {
		payload["truncated"] = true
	}
	return toolResult(payload), nil
}

// handleGetCode reads the requested lines and enforces the maxLines cap.
func (s *Server) handleGetCode(args json.RawMessage) (interface{}, *jsonrpcError) {
	var a getCodeArgs
	if err := unmarshalArgs(args, &a); err != nil {
		return nil, err
	}
	if a.Loc == "" {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "loc is required"}
	}

	content, err := srcread.Read(s.root, a.Loc)
	if err != nil {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "read: " + err.Error()}
	}

	var lineCount int
	if len(content) > 0 {
		lineCount = strings.Count(content, "\n") + 1
	}
	if lineCount > s.maxLines {
		msg := fmt.Sprintf("response truncated: %d lines exceeds --max-lines cap of %d; use loc from get_function to request a precise range", lineCount, s.maxLines)
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: msg}
	}

	return toolResult(map[string]interface{}{
		"content": content,
		"loc":     a.Loc,
	}), nil
}

// handleUpdateFunction persists a curated record with tri-state semantics.
func (s *Server) handleUpdateFunction(args json.RawMessage) (interface{}, *jsonrpcError) {
	var a updateFunctionArgs
	if err := unmarshalArgs(args, &a); err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "name is required"}
	}

	curated := kv.CuratedFunction{Name: a.Name}
	if a.Description != nil {
		curated.Description = *a.Description
	}

	// loc: string | null | absent
	if loc, ok, err := decodeRawString(a.Loc); err != nil {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "loc: " + err.Error()}
	} else if ok {
		curated.Loc = &loc
	}

	// depends: []string | null | absent
	if deps, ok, err := decodeRawStringSlice(a.Depends); err != nil {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "depends: " + err.Error()}
	} else if ok {
		curated.Depends = &deps
	}

	// test: []string | null | absent
	if tests, ok, err := decodeRawStringSlice(a.Test); err != nil {
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "test: " + err.Error()}
	} else if ok {
		curated.Test = &tests
	}

	if err := s.store.PutCurated(curated); err != nil {
		return nil, &jsonrpcError{Code: codeInternalError, Message: "put_curated: " + err.Error()}
	}
	s.cache.invalidate()
	return toolResult(map[string]interface{}{"ok": true}), nil
}

// --- helpers ---

// unmarshalArgs decodes a tools/call arguments payload into out.
// An empty payload is treated as {}.
func unmarshalArgs(raw json.RawMessage, out interface{}) *jsonrpcError {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &jsonrpcError{Code: codeInvalidParams, Message: "invalid arguments: " + err.Error()}
	}
	return nil
}

// decodeRawString returns (value, present, err).
// present=false means the key was absent or JSON null (keep existing value).
// present=true with value="" means clear the override.
func decodeRawString(raw json.RawMessage) (string, bool, error) {
	if len(raw) == 0 {
		return "", false, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false, err
	}
	return s, true, nil
}

// decodeRawStringSlice returns (value, present, err).
// A present empty slice signals clear.
func decodeRawStringSlice(raw json.RawMessage) ([]string, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" {
		return nil, false, nil
	}
	var v []string
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false, err
	}
	if v == nil {
		// "[]" unmarshals to non-nil empty slice, but be defensive.
		v = []string{}
	}
	return v, true, nil
}

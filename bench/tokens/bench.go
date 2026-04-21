// Command bench measures tool-output token consumption for two paths:
// a baseline agent (ripgrep + targeted reads) and a KV agent (kv.mcp tools).
// It runs both paths against the 10 pre-registered Xray-core queries and
// emits a markdown comparison table.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/culnaen/kv.mcp/internal/kv"
	"github.com/culnaen/kv.mcp/internal/srcread"
)

type query struct {
	ID            int    `json:"id"`
	Query         string `json:"query"`
	Justification string `json:"justification"`
}

type result struct {
	ID         int
	Query      string
	BaseTurns  int
	BaseTokens int
	KVTurns    int
	KVTokens   int
	KVCount    int
}

// approxTokens uses the characters ÷ 4 heuristic (ceiling). Dependency-free and
// reproducible — documented in README.md.
func approxTokens(s string) int {
	return (len(s) + 3) / 4
}

// readLines reads at most limit lines starting at 1-indexed offset (inclusive).
// If offset < 1 it is clamped to 1. Missing files return "".
func readLines(path string, offset, limit int) string {
	if offset < 1 {
		offset = 1
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck

	var out strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	collected := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if collected >= limit {
			break
		}
		if collected > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(scanner.Text())
		collected++
	}
	return out.String()
}

// rgMatch is a decoded ripgrep --json line (text submatch events only).
type rgMatch struct {
	File string
	Line int
}

// runRipgrep executes ripgrep and returns (rawOutput, matches).
// Falls back to grep -rn when rg is not on PATH.
func runRipgrep(query, root string) (string, []rgMatch) {
	if _, err := exec.LookPath("rg"); err == nil {
		cmd := exec.Command("rg", "--json", "-n", "--no-heading", query, root)
		out, _ := cmd.Output()
		raw := string(out)
		return raw, parseRipgrepJSON(raw)
	}
	// Fallback: grep -rn
	cmd := exec.Command("grep", "-rn", query, root)
	out, _ := cmd.Output()
	raw := string(out)
	return raw, parseGrepOutput(raw, root)
}

func parseRipgrepJSON(raw string) []rgMatch {
	var matches []rgMatch
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		var evt struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				LineNumber int `json:"line_number"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if evt.Type != "match" {
			continue
		}
		matches = append(matches, rgMatch{File: evt.Data.Path.Text, Line: evt.Data.LineNumber})
	}
	return matches
}

func parseGrepOutput(raw, root string) []rgMatch {
	var matches []rgMatch
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		// format: path:line:content
		first := strings.Index(line, ":")
		if first < 0 {
			continue
		}
		rest := line[first+1:]
		second := strings.Index(rest, ":")
		if second < 0 {
			continue
		}
		lineNum, err := strconv.Atoi(rest[:second])
		if err != nil {
			continue
		}
		matches = append(matches, rgMatch{File: line[:first], Line: lineNum})
	}
	return matches
}

// runBaseline: Grep + up to 3 targeted reads of an 80-line window around the first hit per file.
func runBaseline(queryStr, root string) (int, int) {
	rgRaw, matches := runRipgrep(queryStr, root)
	tokens := approxTokens(rgRaw)
	turns := 1

	if len(matches) == 0 {
		return tokens, turns
	}

	// First hit per file, preserving ripgrep's discovery order.
	seen := make(map[string]bool)
	var firstHits []rgMatch
	for _, m := range matches {
		if seen[m.File] {
			continue
		}
		seen[m.File] = true
		firstHits = append(firstHits, m)
		if len(firstHits) >= 3 {
			break
		}
	}

	for _, hit := range firstHits {
		offset := hit.Line - 40
		if offset < 1 {
			offset = 1
		}
		content := readLines(hit.File, offset, 80)
		tokens += approxTokens(content)
		turns++
	}
	return tokens, turns
}

// toolResultJSON mirrors the envelope MCP handlers produce via toolResult(), so
// token counts reflect realistic tool output size.
func toolResultJSON(v interface{}) string {
	payload := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": mustJSON(v)},
		},
	}
	return mustJSON(payload)
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// runKV: search → get_function (top 1) → get_code(loc) for top match.
func runKV(queryStr, root string, store kv.Store) (int, int, int) {
	// 1) search(query): substring match on name/description, cap at 50.
	q := strings.ToLower(queryStr)
	var results []kv.Function
	scanErr := store.ScanMerged(root, func(f kv.Function) bool {
		if strings.Contains(strings.ToLower(f.Name), q) ||
			strings.Contains(strings.ToLower(f.Description), q) {
			results = append(results, f)
			if len(results) >= 50 {
				return false
			}
		}
		return true
	})
	if scanErr != nil && scanErr.Error() != "stop" {
		fmt.Fprintf(os.Stderr, "search error: %v\n", scanErr)
	}

	searchPayload := map[string]interface{}{
		"matches": results,
		"count":   len(results),
	}
	if len(results) >= 50 {
		searchPayload["truncated"] = true
	}
	tokens := approxTokens(toolResultJSON(searchPayload))
	turns := 1
	count := len(results)

	if count == 0 {
		return tokens, turns, count
	}

	// 2) get_function for top 1 match only.
	top := results[:1]
	for _, r := range top {
		f, ok, err := store.GetMerged(r.Name, root)
		if err != nil || !ok {
			continue
		}
		tokens += approxTokens(toolResultJSON(f))
		turns++
	}

	// 3) get_code(loc) for top match.
	if len(top) > 0 {
		loc := top[0].Loc
		content, err := srcread.Read(root, loc)
		if err == nil {
			payload := map[string]interface{}{"content": content, "loc": loc}
			tokens += approxTokens(toolResultJSON(payload))
			turns++
		}
	}
	return tokens, turns, count
}

func loadQueries(path string) ([]query, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var qs []query
	if err := json.Unmarshal(b, &qs); err != nil {
		return nil, err
	}
	return qs, nil
}

// isNoMatch returns true when neither path found anything useful — baseline
// paid 0 tokens (rg found nothing) and KV search returned 0 results. These
// queries are excluded from the win count.
func isNoMatch(r result) bool {
	return r.BaseTokens == 0 && r.KVCount == 0
}

func renderTable(results []result) string {
	var b strings.Builder
	b.WriteString("| # | Query | Baseline Turns | Baseline Tokens | KV Turns | KV Tokens | Δ Tokens | Win |\n")
	b.WriteString("|---|-------|---------------|----------------|---------|----------|---------|-----|\n")
	for _, r := range results {
		delta := r.KVTokens - r.BaseTokens
		var win string
		switch {
		case isNoMatch(r):
			win = "No match"
		case r.KVTokens < r.BaseTokens:
			win = "KV"
		case r.KVTokens == r.BaseTokens:
			win = "Tie"
		default:
			win = "Baseline"
		}
		fmt.Fprintf(&b, "| %d | %s | %d | %d | %d | %d | %d | %s |\n",
			r.ID, r.Query, r.BaseTurns, r.BaseTokens, r.KVTurns, r.KVTokens, delta, win)
	}
	return b.String()
}

func main() {
	dbPath := flag.String("db", "/tmp/xray.db", "path to indexed BoltDB")
	root := flag.String("root", "/tmp/xray-core", "project root")
	outPath := flag.String("output", "bench/tokens/results.md", "markdown results path")
	queriesPath := flag.String("queries", "", "queries JSON path (auto-resolves when empty)")
	flag.Parse()

	qpath := *queriesPath
	if qpath == "" {
		candidates := []string{
			"bench/tokens/testdata/xray-core-queries.json",
			"testdata/xray-core-queries.json",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				qpath = c
				break
			}
		}
		if qpath == "" {
			fmt.Fprintln(os.Stderr, "cannot locate testdata/xray-core-queries.json")
			os.Exit(1)
		}
	}

	queries, err := loadQueries(qpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load queries: %v\n", err)
		os.Exit(1)
	}

	store, err := kv.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close() //nolint:errcheck

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		absRoot = *root
	}

	results := make([]result, 0, len(queries))
	for _, q := range queries {
		baseTok, baseTurns := runBaseline(q.Query, absRoot)
		kvTok, kvTurns, kvCount := runKV(q.Query, absRoot, store)
		results = append(results, result{
			ID:         q.ID,
			Query:      q.Query,
			BaseTurns:  baseTurns,
			BaseTokens: baseTok,
			KVTurns:    kvTurns,
			KVTokens:   kvTok,
			KVCount:    kvCount,
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	table := renderTable(results)
	kvWins := 0
	noMatch := 0
	for _, r := range results {
		if isNoMatch(r) {
			noMatch++
			continue
		}
		if r.KVTokens < r.BaseTokens {
			kvWins++
		}
	}
	meaningful := len(results) - noMatch
	summary := fmt.Sprintf("\n**Summary:** KV wins: %d / %d meaningful queries (excluding %d no-match queries).\n",
		kvWins, meaningful, noMatch)

	fmt.Println(table)
	fmt.Print(summary)

	// Resolve output path relative to CWD; if it's bench/tokens/results.md and
	// we're inside bench/tokens, trim the prefix for convenience.
	out := *outPath
	if _, err := os.Stat(filepath.Dir(out)); os.IsNotExist(err) {
		out = strings.TrimPrefix(out, "bench/tokens/")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil && filepath.Dir(out) != "." {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
	}
	body := "# Token Benchmark Results\n\n" + table + summary
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write results: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", out)
}

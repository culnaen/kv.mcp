package srcread

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Read parses a loc string ("file.go:N" or "file.go:N-M"), reads lines N..M
// (1-indexed, inclusive), and returns them joined by newline.
// root is prepended to relative paths if non-empty.
func Read(root, loc string) (string, error) {
	colon := strings.LastIndex(loc, ":")
	if colon < 0 {
		return "", fmt.Errorf("invalid loc %q: missing ':'", loc)
	}
	filePart := loc[:colon]
	rangePart := loc[colon+1:]

	var start, end int
	if dash := strings.Index(rangePart, "-"); dash >= 0 {
		var err error
		start, err = strconv.Atoi(rangePart[:dash])
		if err != nil {
			return "", fmt.Errorf("invalid loc %q: %w", loc, err)
		}
		end, err = strconv.Atoi(rangePart[dash+1:])
		if err != nil {
			return "", fmt.Errorf("invalid loc %q: %w", loc, err)
		}
	} else {
		n, err := strconv.Atoi(rangePart)
		if err != nil {
			return "", fmt.Errorf("invalid loc %q: %w", loc, err)
		}
		start, end = n, n
	}

	if start < 1 {
		return "", fmt.Errorf("invalid loc %q: line number must be >= 1", loc)
	}
	if end < start {
		return "", fmt.Errorf("invalid loc %q: end line %d < start line %d", loc, end, start)
	}

	path := filePart
	if root != "" && !filepath.IsAbs(filePart) {
		path = filepath.Join(root, filePart)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck

	var lines []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= start && lineNum <= end {
			lines = append(lines, scanner.Text())
		}
		if lineNum > end {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if lineNum < end {
		return "", fmt.Errorf("loc %q: file has %d lines, requested up to %d", loc, lineNum, end)
	}
	return strings.Join(lines, "\n"), nil
}

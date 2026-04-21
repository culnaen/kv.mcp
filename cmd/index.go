package cmd

import (
	"flag"
	"fmt"
	"time"

	"github.com/culnaen/kv.mcp/internal/index"
	"github.com/culnaen/kv.mcp/internal/kv"
)

func IndexCmd(args []string) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	dbPath := fs.String("db", "./.kv.mcp.db", "path to bbolt database (use absolute path when CWD is unpredictable)")
	verbose := fs.Bool("verbose", false, "verbose output")
	tags := fs.String("tags", "", "build tags to pass to go/packages (e.g. integration,linux)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}

	store, err := kv.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close() //nolint:errcheck

	start := time.Now()
	count, err := index.IndexRoot(root, store, *verbose, *tags)
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}
	fmt.Printf("indexed %d functions in %s\n", count, time.Since(start).Round(time.Millisecond))
	return nil
}

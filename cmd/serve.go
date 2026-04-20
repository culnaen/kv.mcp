package cmd

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/culnaen/kv.mcp/internal/kv"
	"github.com/culnaen/kv.mcp/internal/mcp"
)

func ServeCmd(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dbPath := fs.String("db", "./.kv.mcp.db", "path to bbolt database (use absolute path)")
	root := fs.String("root", ".", "project root for resolving relative loc paths")
	maxLines := fs.Int("max-lines", 500, "max lines returned by get_code (hard ceiling: 2000)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *maxLines > 2000 {
		*maxLines = 2000
	}

	store, err := kv.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close() //nolint:errcheck

	srv := mcp.NewServer(store, *root, *maxLines)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		_ = store.Close()
		os.Exit(0)
	}()

	return srv.Run(os.Stdin, os.Stdout)
}

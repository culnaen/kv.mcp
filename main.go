package main

import (
	"fmt"
	"os"

	"github.com/culnaen/kv.mcp/cmd"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: kv.mcp <command> [options]\ncommands: index, serve")
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "index":
		err = cmd.IndexCmd(os.Args[2:])
	case "serve":
		err = cmd.ServeCmd(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\ncommands: index, serve\n", os.Args[1])
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

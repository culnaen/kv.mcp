package main

import (
	"fmt"
	"os"

	"github.com/culnaen/kv.mcp/cmd"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: kv.mcp <command> [options]\ncommands: index, serve")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "--version", "-v":
		fmt.Println("kv.mcp", version)
		return
	case "index":
		if err := cmd.IndexCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "serve":
		if err := cmd.ServeCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\ncommands: index, serve\n", os.Args[1])
		os.Exit(1)
	}
}

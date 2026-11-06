// Command streamdrop-mcp is the standalone console build of the StreamDrop
// MCP server. The release ships only the main binary: run "streamdrop mcp"
// there. This console binary exists for development and testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"streamdrop/internal/mcpserver"
)

func main() {
	dataDir := flag.String("data-dir", "", "StreamDrop data directory (default: beside this executable, else platform default)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := mcpserver.Run(ctx, mcpserver.ResolveDataDir(*dataDir)); err != nil {
		fmt.Fprintln(os.Stderr, "streamdrop-mcp:", err)
		os.Exit(1)
	}
}

// Command streamdrop starts the desktop service: configuration, embedded UI,
// local API, queue, rclone daemon and system tray.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"streamdrop/internal/app"
	"streamdrop/internal/config"
	"streamdrop/internal/mcpserver"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "mcp" {
		runMCP(args[1:])
		return
	}

	cfg, err := config.Load(args)
	if errors.Is(err, config.ErrVersionRequested) {
		fmt.Printf("StreamDrop %s\n", config.Version)
		return
	}
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "streamdrop:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "streamdrop:", err)
		os.Exit(1)
	}
}

// runMCP serves as an MCP server (stdio) until interrupted. This makes the
// single shipped binary usable by MCP clients without any extra downloads.
func runMCP(args []string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dataDir := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--data-dir" {
			dataDir = args[i+1]
		}
	}
	if err := mcpserver.Run(ctx, mcpserver.ResolveDataDir(dataDir)); err != nil {
		fmt.Fprintln(os.Stderr, "streamdrop mcp:", err)
		os.Exit(1)
	}
}

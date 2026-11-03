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
)

func main() {
	cfg, err := config.Load(os.Args[1:])
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

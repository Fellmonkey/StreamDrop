// Package tray provides the system tray icon, the native notification channel
// and the platform-specific stubs (back_spec.md §2, §3.10).
package tray

import "context"

// Callbacks are the tray menu actions.
type Callbacks struct {
	URL           string
	OnOpen        func() // open the desktop app window
	OnOpenBrowser func() // open the UI in the default browser
	OnQuit        func()
	// Summary returns the current transfer status line for the tray
	// ("filename: 42% · ↓3,2 МБ/s ↑2,1 МБ/s"), or "" when idle.
	Summary func() string
}

// Start runs the tray if the platform supports it. It returns immediately; the
// tray lives until ctx is cancelled.
func Start(ctx context.Context, cb Callbacks) {
	start(ctx, cb)
}

// Available reports whether this build shows a tray icon.
func Available() bool { return available() }

// Toast is the native notification channel.
type Toast interface {
	Show(title, message string) error
}

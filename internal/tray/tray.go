// Package tray provides the system tray icon, the native notification channel
// and the platform-specific stubs (back_spec.md §2, §3.10).
package tray

import "context"

// Callbacks are the tray menu actions.
type Callbacks struct {
	URL      string
	OnOpen   func()
	OnPause  func()
	OnResume func()
	OnQuit   func()
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

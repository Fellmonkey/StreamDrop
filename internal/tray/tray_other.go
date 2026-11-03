//go:build !windows

package tray

import "context"

// Linux and macOS builds are best effort: the tray integration is Windows-only
// in this release. The UI keeps working and transfers never depend on the tray.
func available() bool { return false }

func start(ctx context.Context, cb Callbacks) {}

type noopToast struct{}

func (noopToast) Show(title, message string) error { return nil }

// Toaster returns a no-op channel on platforms without native toasts.
func Toaster() Toast { return noopToast{} }

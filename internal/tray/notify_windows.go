//go:build windows

package tray

import "github.com/go-toast/toast"

// windowsToast delivers native Windows toast notifications. This is a separate
// delivery channel from the tray icon: energye/systray only offers icon, menu
// and tooltip and has no native notification API (back_spec.md §3.10).
type windowsToast struct{}

func (windowsToast) Show(title, message string) error {
	n := toast.Notification{
		AppID:   "StreamDrop",
		Title:   title,
		Message: message,
	}
	return n.Push()
}

//go:build windows

package tray

import (
	"context"
	"sync"
	"time"

	"github.com/energye/systray"
)

var toastImpl = &windowsToast{}

func available() bool { return true }

const idleStatusTitle = "No active transfers"

// start launches the tray. systray.Run blocks, so it lives in its own goroutine
// and is stopped explicitly when the context ends.
func start(ctx context.Context, cb Callbacks) {
	go func() {
		var once sync.Once
		quitApp := func() {
			once.Do(func() {
				if cb.OnQuit != nil {
					cb.OnQuit()
				}
				systray.Quit()
			})
		}

		var statusItem *systray.MenuItem
		systray.Run(func() {
			systray.SetTitle("StreamDrop")
			systray.SetTooltip("StreamDrop — stream to cloud")
			if len(icon) > 0 {
				systray.SetIcon(icon)
			}

			open := systray.AddMenuItem("Open StreamDrop", "Open the desktop app window")
			openBrowser := systray.AddMenuItem("Open in browser", "Open the web interface in your default browser")
			statusItem = systray.AddMenuItem(idleStatusTitle, "Active transfer progress and speed")
			statusItem.Disable()
			systray.AddSeparator()
			quit := systray.AddMenuItem("Quit", "Stop StreamDrop and exit")

			open.Click(func() {
				if cb.OnOpen != nil {
					cb.OnOpen()
				}
			})
			openBrowser.Click(func() {
				if cb.OnOpenBrowser != nil {
					cb.OnOpenBrowser()
				}
			})
			quit.Click(quitApp)
		}, func() {})

		// Update the status line while the tray runs.
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				quitApp()
				return
			case <-ticker.C:
				if statusItem == nil || cb.Summary == nil {
					continue
				}
				s := cb.Summary()
				if s == "" {
					statusItem.SetTitle(idleStatusTitle)
					statusItem.Disable()
				} else {
					statusItem.SetTitle(s)
					statusItem.Enable()
				}
			}
		}
	}()
}

// Toaster returns the Windows toast channel.
func Toaster() Toast { return toastImpl }

//go:build windows

package tray

import (
	"context"
	"sync"

	"github.com/energye/systray"
)

var toastImpl = &windowsToast{}

func available() bool { return true }

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

		systray.Run(func() {
			systray.SetTitle("StreamDrop")
			systray.SetTooltip("StreamDrop — stream to cloud")

			open := systray.AddMenuItem("Open StreamDrop", "Open the desktop app window")
			openBrowser := systray.AddMenuItem("Open in browser", "Open the web interface in your default browser")
			pause := systray.AddMenuItem("Pause queue", "Pause active transfers")
			resume := systray.AddMenuItem("Resume queue", "Resume the transfer queue")
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
			pause.Click(func() {
				if cb.OnPause != nil {
					cb.OnPause()
				}
			})
			resume.Click(func() {
				if cb.OnResume != nil {
					cb.OnResume()
				}
			})
			quit.Click(quitApp)
		}, func() {})

		<-ctx.Done()
		quitApp()
	}()
}

// Toaster returns the Windows toast channel.
func Toaster() Toast { return toastImpl }

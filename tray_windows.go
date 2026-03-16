//go:build windows

package main

import (
	_ "embed"
	"os"

	"github.com/energye/systray"
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIconBytes []byte

// setupSystray starts the system tray icon with show/quit options.
// Must be called from OnStartup (after ctx is set).
func (a *App) setupSystray() {
	go systray.Run(func() {
		systray.SetIcon(trayIconBytes)
		systray.SetTitle("TallyBridge")
		systray.SetTooltip("TallyBridge Connector — Running")

		mShow := systray.AddMenuItem("Show Window", "Show the main window")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit TallyBridge completely")

		mShow.Click(func() {
			wailsrt.WindowShow(a.ctx)
		})

		mQuit.Click(func() {
			systray.Quit()
			os.Exit(0)
		})

		systray.SetOnClick(func(menu systray.IMenu) {
			wailsrt.WindowShow(a.ctx)
		})

		systray.SetOnDClick(func(menu systray.IMenu) {
			wailsrt.WindowShow(a.ctx)
		})
	}, nil)
}

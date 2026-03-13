package main

import (
	"context"
	_ "embed"
	"os"

	"github.com/energye/systray"
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/appicon.png
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

		// Left-click on tray icon restores window
		systray.SetOnClick(func(menu systray.IMenu) {
			wailsrt.WindowShow(a.ctx)
		})

		// Double-click also restores
		systray.SetOnDClick(func(menu systray.IMenu) {
			wailsrt.WindowShow(a.ctx)
		})
	}, nil)
}

// hideWindowInsteadOfClose is used as the OnBeforeClose callback.
// It hides the window instead of quitting, so background sync keeps running.
func (a *App) hideWindowInsteadOfClose(ctx context.Context) bool {
	wailsrt.WindowHide(a.ctx)
	return true // true = prevent default close behaviour
}

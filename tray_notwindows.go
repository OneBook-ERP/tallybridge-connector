//go:build !windows

package main

// setupSystray is a no-op on non-Windows platforms.
// On macOS, energye/systray's nativeLoop conflicts with Wails' main thread ownership.
// Windows production builds use tray_windows.go for the real systray implementation.
func (a *App) setupSystray() {}

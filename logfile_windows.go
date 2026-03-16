//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// init hides any console window and redirects all stdout/stderr to a log file.
// Belt-and-suspenders: the linker sets subsystem=GUI (-H windowsgui / -mwindows) so
// Windows should never allocate a console window, but we also call FreeConsole in case
// the CGO/zig toolchain didn't honour that flag on this particular build.
func init() {
	// Hide + free any console window before anything else runs.
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	user32 := windows.NewLazySystemDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")
	freeConsole := kernel32.NewProc("FreeConsole")

	if hwnd, _, _ := getConsoleWindow.Call(); hwnd != 0 {
		const swHide = 0
		showWindow.Call(hwnd, swHide)
		freeConsole.Call()
	}

	appData := os.Getenv("APPDATA")
	if appData == "" {
		return
	}
	logDir := filepath.Join(appData, "TallyBridge")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return
	}
	logPath := filepath.Join(logDir, "tallybridge.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}

	// Redirect Go runtime stdout/stderr
	os.Stdout = f
	os.Stderr = f

	// Redirect Windows kernel handles so CGO code and built-in println also go to the file.
	hFile := windows.Handle(f.Fd())
	_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, hFile)
	_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, hFile)

	// Point the standard log package at the file too
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

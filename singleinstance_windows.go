//go:build windows

package main

import (
	"log"
	"unsafe"

	"golang.org/x/sys/windows"
)

const mutexName = "Global\\TallyBridgeConnectorMutex"

// ensureSingleInstance creates a named mutex. If another instance already holds
// it, we find the existing window by title, bring it to the foreground, and
// return false so the caller can exit immediately.
func ensureSingleInstance() bool {
	name, _ := windows.UTF16PtrFromString(mutexName)
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		// ERROR_ALREADY_EXISTS means another instance owns the mutex
		if err == windows.ERROR_ALREADY_EXISTS {
			log.Println("Another TallyBridge instance is already running — activating it.")
			bringExistingWindowToFront()
			if h != 0 {
				windows.CloseHandle(h)
			}
			return false
		}
		// Other errors: log but proceed (best-effort)
		log.Printf("CreateMutex warning: %v", err)
	}
	// Keep the handle open for the lifetime of the process (don't close it).
	// When the process exits, Windows auto-releases it.
	return true
}

func bringExistingWindowToFront() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	showWindow := user32.NewProc("ShowWindow")
	setForegroundWindow := user32.NewProc("SetForegroundWindow")

	title, _ := windows.UTF16PtrFromString("TallyBridge Connector")

	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		log.Println("Could not find existing TallyBridge window")
		return
	}

	const swRestore = 9
	showWindow.Call(hwnd, swRestore)
	setForegroundWindow.Call(hwnd)
}

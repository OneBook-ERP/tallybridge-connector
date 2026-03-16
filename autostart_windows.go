//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	autoStartRegKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartAppName = "TallyBridge"
)

// GetAutoStartEnabled returns true if TallyBridge is set to run at Windows login.
func (a *App) GetAutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, autoStartRegKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(autoStartAppName)
	return err == nil
}

// SetAutoStart adds or removes the Windows login auto-start registry entry.
func (a *App) SetAutoStart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, autoStartRegKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if !enable {
		// Ignore "not found" — already removed
		err = k.DeleteValue(autoStartAppName)
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Use clean absolute path; quote it in case the install dir has spaces
	exePath := `"` + filepath.Clean(exe) + `"`
	return k.SetStringValue(autoStartAppName, exePath)
}

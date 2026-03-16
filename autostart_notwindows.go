//go:build !windows

package main

// GetAutoStartEnabled is a no-op stub on non-Windows platforms.
func (a *App) GetAutoStartEnabled() bool { return false }

// SetAutoStart is a no-op stub on non-Windows platforms.
func (a *App) SetAutoStart(enable bool) error { return nil }

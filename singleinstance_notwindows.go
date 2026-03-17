//go:build !windows

package main

// ensureSingleInstance is a no-op on non-Windows platforms.
// On macOS, the OS already enforces single-instance for .app bundles.
func ensureSingleInstance() bool {
	return true
}

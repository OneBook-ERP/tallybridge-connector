package main

import (
	"context"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// hideWindowInsteadOfClose is used as the OnBeforeClose callback.
// It hides the window instead of quitting, so background sync keeps running.
func (a *App) hideWindowInsteadOfClose(ctx context.Context) bool {
	wailsrt.WindowHide(a.ctx)
	return true // true = prevent default close behaviour
}

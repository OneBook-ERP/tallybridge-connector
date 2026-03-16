package main

import (
	"context"
	"embed"
	"log"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/tallybridge/connector/store"
	"github.com/tallybridge/connector/tally"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 1. Initialize local SQLite DB
	db, err := store.InitDB("connector.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 2. Initialize Tally Client
	tclient := tally.NewClient(
		tally.WithTimeout(30*time.Second),
		tally.WithAddr("http://192.168.1.12:9000"),
	)

	// Create an instance of the app structure
	app := NewApp(db, tclient)

	log.Println("Starting TallyBridge Connector UI...")

	// Create application with options
	err = wails.Run(&options.App{
		Title:  "TallyBridge Connector",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 243, G: 244, B: 246, A: 255}, // Tailwind gray-100
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			app.setupSystray()
		},
		OnBeforeClose:    app.hideWindowInsteadOfClose,
		HideWindowOnClose: true,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Printf("Wails run error: %v", err)
	}
}

package main

import (
	"embed"
	"log"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/HamzaBaghdadi/quarry/internal/store"
	"github.com/HamzaBaghdadi/quarry/internal/updater"

	// Database adapters register themselves. Adding an engine = one import.
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/clickhouse"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/mongo"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/mssql"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/mysql"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/postgres"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/redis"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/sqlite"
)

// version is stamped at release time: -ldflags "-X main.version=0.2.0".
var version = "dev"

//go:embed all:frontend/out
var assets embed.FS

func main() {
	updater.Cleanup()
	s, err := store.Open()
	if err != nil {
		log.Fatal(err)
	}
	app := NewApp(s)

	err = wails.Run(&options.App{
		Title:     "Quarry",
		Width:     defaultW,
		Height:    defaultH,
		MinWidth:  minW,
		MinHeight: minH,
		// Shown once sized to fit the screen (see window.go).
		StartHidden: true,
		// Our own title bar on Windows & Linux; macOS keeps its traffic lights
		// over a full-size content view.
		Frameless:        goruntime.GOOS != "darwin",
		BackgroundColour: &options.RGBA{R: 8, G: 8, B: 10, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnDomReady:       app.placeWindow,
		OnBeforeClose:    app.saveWindowState,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			Theme:                             windows.Dark,
			DisableFramelessWindowDecorations: false, // keep the shadow & rounded corners
		},
		Mac: &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
			About: &mac.AboutInfo{
				Title:   "Quarry",
				Message: "A small, fast client for every database.\nApache License 2.0",
			},
		},
		Linux: &linux.Options{ProgramName: "Quarry"},
	})
	if err != nil {
		log.Fatal(err)
	}
}

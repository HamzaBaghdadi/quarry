package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/HamzaBaghdadi/quarry/internal/store"

	// Database adapters register themselves. Adding an engine = one import.
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/clickhouse"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/mongo"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/mssql"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/mysql"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/postgres"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/redis"
	_ "github.com/HamzaBaghdadi/quarry/internal/drivers/sqlite"
)

var version = "0.1.0"

//go:embed all:frontend/out
var assets embed.FS

func main() {
	s, err := store.Open()
	if err != nil {
		log.Fatal(err)
	}
	app := NewApp(s)

	err = wails.Run(&options.App{
		Title:            "Quarry",
		Width:            1360,
		Height:           860,
		MinWidth:         900,
		MinHeight:        560,
		BackgroundColour: &options.RGBA{R: 11, G: 15, B: 25, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "JARVIS",
		Width:  520,
		Height: 360,

		Frameless:   true,
		AlwaysOnTop: true,

		BackgroundColour: &options.RGBA{
			R: 0,
			G: 0,
			B: 0,
			A: 0,
		},

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},

		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			DisableWindowIcon:    true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

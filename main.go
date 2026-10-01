package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Frontend assets are embedded into the binary for production builds.
//
//go:embed all:frontend/dist
var assets embed.FS

// Version is the application version. Keep in sync with build/config.yml and
// build/windows/info.json.
const Version = "1.0.0"

func main() {
	rt, err := newRuntime()
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	vaultService := &VaultService{rt: rt}
	taskService := &TaskService{rt: rt}

	app := application.New(application.Options{
		Name:        "ElectricHamster",
		Description: "电子仓鼠 - 自动加密归档工具",
		Services: []application.Service{
			application.NewService(vaultService),
			application.NewService(taskService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	rt.app = app

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "ElectricHamster",
		Width:            1100,
		Height:           720,
		BackgroundColour: application.NewRGB(18, 20, 28),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

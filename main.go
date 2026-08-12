package main

import (
	"context"
	"embed"
	"log"

	"embedtools/internal/module"
	"embedtools/internal/modules"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	mods := modules.All()

	seen := make(map[string]bool, len(mods))
	binds := make([]any, 0, len(mods))
	for _, m := range mods {
		if seen[m.ID()] {
			log.Fatalf("模块 ID 重复: %s", m.ID())
		}
		seen[m.ID()] = true
		binds = append(binds, m.Bindings()...)
	}

	err := wails.Run(&options.App{
		Title:            "Estun Codroid 机器人工具箱",
		Width:            1180,
		Height:           760,
		MinWidth:         960,
		MinHeight:        620,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 244, G: 245, B: 247, A: 1},
		OnStartup: func(ctx context.Context) {
			for _, m := range mods {
				if s, ok := m.(module.Startupper); ok {
					s.Startup(ctx)
				}
			}
		},
		Bind: binds,
	})
	if err != nil {
		log.Fatal(err)
	}
}

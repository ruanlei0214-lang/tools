package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"embedtools/internal/module"
	"embedtools/internal/modules"
	"embedtools/internal/toolbox"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	mods := modules.All()

	seen := make(map[string]bool, len(mods))
	// 顶栏改地址的入口不属于任何模块，始终编进产物。
	binds := make([]any, 0, len(mods)+1)
	binds = append(binds, toolbox.New())
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
		// WebView2 缓存放 exe 旁边：第二次打开不用再往 %APPDATA% 里冷启动，
		// 整夹拷走缓存也跟着走。目录建不出来就让 Wails 走默认，别为此起不来。
		Windows: &windows.Options{WebviewUserDataPath: webviewDataDir()},
		// 文件区要把资源管理器拖进来的路径交给前端，不能让 WebView 自己打开那个文件。
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
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

func webviewDataDir() string {
	dir, err := module.DataDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "webview2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}

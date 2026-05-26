//go:build wails

package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"lan-transfer/internal/desktop"
)

//go:embed all:static
var assets embed.FS

func main() {
	control := desktop.NewApp()

	app := application.New(application.Options{
		Name:        "简传",
		Description: "局域网文件传输工具",
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Services: []application.Service{
			application.NewService(control),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "简传",
		Width:  920,
		Height: 680,
		URL:    "/",
	})

	tray := app.SystemTray.New()
	tray.SetLabel("简传")
	tray.SetMenu(application.NewMenuFromItems(
		application.NewMenuItem("打开主窗口").OnClick(func(ctx *application.Context) {
			window.Show()
			window.Focus()
		}),
		application.NewMenuItem("打开浏览器传输页").OnClick(func(ctx *application.Context) {
			_ = control.OpenBrowser()
		}),
		application.NewMenuItem("复制访问地址").OnClick(func(ctx *application.Context) {
			if url := control.PrimaryURL(); url != "" {
				_ = app.Clipboard.SetText(url)
			}
		}),
		application.NewMenuItem("启动服务").OnClick(func(ctx *application.Context) {
			_ = control.StartService()
		}),
		application.NewMenuItem("停止服务").OnClick(func(ctx *application.Context) {
			_ = control.StopService()
		}),
		application.NewMenuItem("退出").OnClick(func(ctx *application.Context) {
			_ = control.StopService()
			app.Quit()
		}),
	))

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

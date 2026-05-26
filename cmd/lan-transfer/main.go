package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lan-transfer/internal/config"
	"lan-transfer/internal/server"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help" {
		usage()
		return
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runServe(args []string) {
	cfg := config.Default()
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.IntVar(&cfg.Port, "port", cfg.Port, "监听端口")
	fs.StringVar(&cfg.StorageDir, "dir", cfg.StorageDir, "文件保存目录")
	fs.StringVar(&cfg.AccessCode, "code", cfg.AccessCode, "访问码")
	_ = fs.Parse(args)

	srv, err := server.New(server.Options{
		Config: cfg,
		Logger: slog.Default(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "监听失败: %v\n", err)
		os.Exit(1)
	}

	status := srv.Status()
	fmt.Println("简传服务已启动")
	fmt.Printf("保存目录: %s\n", status.StorageDir)
	fmt.Printf("访问码: %s\n", status.AccessCode)
	fmt.Println("访问地址:")
	for _, url := range status.URLs {
		fmt.Printf("  %s\n", url)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "停止失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("服务已停止")
}

func usage() {
	fmt.Println(`简传

用法:
  lan-transfer serve [--port 8080] [--dir ./data/uploads] [--code 123456]

说明:
  启动后，同一局域网内的设备可通过浏览器访问地址上传或下载文件。`)
}

//go:build !wails

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "桌面版需要安装 Wails 后使用 -tags wails 构建；CLI 可先使用: go run ./cmd/lan-transfer serve")
	os.Exit(1)
}

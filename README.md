# 简传

一个面向局域网的轻量文件传输工具。一台 Windows/macOS/Linux 机器启动监听服务，其他设备通过浏览器访问地址，输入访问码后上传或下载文件。

## 当前能力

- Go 单二进制 CLI 服务
- 浏览器上传、文件列表、下载、删除
- 访问码登录，登录后使用 HTTP-only Cookie
- 上传文件流式落盘，先写入 `.part` 再原子改名
- 中文文件名支持
- 局域网地址展示
- 二维码接口
- Wails 桌面壳代码骨架，安装 Wails 后可继续启用构建

## CLI 使用

```bash
go run ./cmd/lan-transfer serve --port 8080 --dir ./data/uploads --code 123456
```

启动后终端会输出访问地址和访问码。同一局域网内其他设备打开地址即可进入浏览器传输页。

## 桌面 UI

桌面入口位于 `cmd/lan-transfer-desktop`，默认构建为提示程序，避免没有 Wails 环境时影响核心服务测试。

安装 Wails v3 工具链后，可用构建标签启用桌面入口：

```bash
go run -tags wails ./cmd/lan-transfer-desktop
```

如果本地还没有 Wails 依赖，可先执行：

```bash
go get github.com/wailsapp/wails/v3/pkg/application
```

打包当前平台桌面 App：

```bash
go run ./scripts/package.go
```

macOS / Linux 也可以使用：

```bash
./scripts/package.sh
```

Windows PowerShell 可使用：

```powershell
.\scripts\package.ps1
```

也可以继续通过 Wails 入口触发同一套打包流程：

```bash
wails3 package
```

按平台显式打包：

```bash
go run ./scripts/package.go -target macos
go run ./scripts/package.go -target windows
go run ./scripts/package.go -target linux
```

桌面 App 依赖目标系统的 WebView/窗口库，建议在对应系统本机打包。输出目录默认为 `dist/`，可用 `-out` 修改，用 `-clean` 清理旧产物，用 `-skip-tests` 跳过测试。

macOS 打包产物示例：

```text
dist/简传-macos-arm64/简传.app
dist/简传-macos-arm64.zip
```

桌面 UI 负责：

- 启动 / 停止传输服务
- 显示服务状态、访问地址、访问码和二维码
- 修改端口、保存目录、访问码等配置
- 打开浏览器传输页
- 提供托盘菜单入口

桌面图标源文件位于 `build/assets/app-icon.svg`。打包时会自动生成 macOS `AppIcon.icns`，Windows 打包会生成临时 `.syso` 资源并把图标、版本信息和 manifest 写入 `.exe`。

## 测试

```bash
go test ./...
```

## 安全说明

首版面向可信局域网，不建议直接暴露到公网。访问码用于防误入，不等同于强认证或端到端加密。

package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	appName    = "简传"
	executable = "lan-transfer-desktop"
)

type options struct {
	target    string
	arch      string
	outDir    string
	skipTests bool
	clean     bool
	sign      bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "打包失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var opt options
	flag.StringVar(&opt.target, "target", runtime.GOOS, "目标平台: macos/darwin/windows/linux")
	flag.StringVar(&opt.arch, "arch", runtime.GOARCH, "目标架构，默认当前架构")
	flag.StringVar(&opt.outDir, "out", "dist", "输出目录")
	flag.BoolVar(&opt.skipTests, "skip-tests", false, "跳过测试")
	flag.BoolVar(&opt.clean, "clean", false, "打包前清理输出目录")
	flag.BoolVar(&opt.sign, "sign", true, "macOS 打包时执行本地临时代码签名")
	flag.Parse()

	root, err := projectRoot()
	if err != nil {
		return err
	}
	target, err := normalizeTarget(opt.target)
	if err != nil {
		return err
	}
	if err := validateNativeTarget(target, opt.arch); err != nil {
		return err
	}

	outRoot := filepath.Join(root, opt.outDir)
	if opt.clean {
		if err := os.RemoveAll(outRoot); err != nil {
			return err
		}
	}
	if !opt.skipTests {
		if err := runCommand(root, nil, "go", "test", "./..."); err != nil {
			return err
		}
		if err := runCommand(root, nil, "go", "test", "-tags", "wails", "./cmd/lan-transfer-desktop"); err != nil {
			return err
		}
	}

	packageDir := filepath.Join(outRoot, fmt.Sprintf("%s-%s-%s", appName, platformLabel(target), opt.arch))
	if err := os.RemoveAll(packageDir); err != nil {
		return err
	}
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		return err
	}

	switch target {
	case "darwin":
		return packageDarwin(root, packageDir, opt)
	case "windows":
		return packageWindows(root, packageDir, opt)
	case "linux":
		return packageLinux(root, packageDir, opt)
	default:
		return fmt.Errorf("不支持的平台: %s", target)
	}
}

func projectRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("无法定位脚本路径")
	}
	return filepath.Dir(filepath.Dir(file)), nil
}

func normalizeTarget(target string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "macos", "mac", "darwin":
		return "darwin", nil
	case "win", "windows":
		return "windows", nil
	case "linux":
		return "linux", nil
	default:
		return "", fmt.Errorf("不支持的平台: %s", target)
	}
}

func validateNativeTarget(target, arch string) error {
	if target != runtime.GOOS {
		return fmt.Errorf("桌面 App 依赖目标系统的 WebView/窗口库，请在目标系统本机打包；当前=%s，目标=%s", runtime.GOOS, target)
	}
	if arch != runtime.GOARCH {
		return fmt.Errorf("当前脚本只做本机架构打包；当前=%s，目标=%s", runtime.GOARCH, arch)
	}
	return nil
}

func platformLabel(target string) string {
	if target == "darwin" {
		return "macos"
	}
	return target
}

func packageDarwin(root, packageDir string, opt options) error {
	if err := ensureDarwinIcon(root); err != nil {
		return err
	}

	appDir := filepath.Join(packageDir, appName+".app")
	macOSDir := filepath.Join(appDir, "Contents", "MacOS")
	resourcesDir := filepath.Join(appDir, "Contents", "Resources")
	if err := os.MkdirAll(macOSDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		return err
	}

	if err := buildDesktop(root, filepath.Join(macOSDir, executable), "darwin", opt.arch); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(root, "build", "darwin", "Info.plist"), filepath.Join(appDir, "Contents", "Info.plist"), 0o644); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(root, "build", "darwin", "AppIcon.icns"), filepath.Join(resourcesDir, "AppIcon.icns"), 0o644); err != nil {
		return err
	}

	if opt.sign {
		if _, err := exec.LookPath("codesign"); err != nil {
			fmt.Println("跳过签名: 未找到 codesign")
		} else if err := runCommand(root, nil, "codesign", "--force", "--deep", "--sign", "-", appDir); err != nil {
			return err
		}
	}

	archivePath := packageDir + ".zip"
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := zipDir(packageDir, archivePath); err != nil {
		return err
	}
	fmt.Printf("完成: %s\n", appDir)
	fmt.Printf("归档: %s\n", archivePath)
	return nil
}

func packageWindows(root, packageDir string, opt options) error {
	exePath := filepath.Join(packageDir, appName+".exe")
	if err := buildDesktop(root, exePath, "windows", opt.arch); err != nil {
		return err
	}
	archivePath := packageDir + ".zip"
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := zipDir(packageDir, archivePath); err != nil {
		return err
	}
	fmt.Printf("完成: %s\n", exePath)
	fmt.Printf("归档: %s\n", archivePath)
	return nil
}

func packageLinux(root, packageDir string, opt options) error {
	binPath := filepath.Join(packageDir, appName)
	if err := buildDesktop(root, binPath, "linux", opt.arch); err != nil {
		return err
	}
	if err := os.Chmod(binPath, 0o755); err != nil {
		return err
	}

	iconDir := filepath.Join(packageDir, "share", "icons", "hicolor", "scalable", "apps")
	appDir := filepath.Join(packageDir, "share", "applications")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(root, "build", "assets", "app-icon.svg"), filepath.Join(iconDir, "jianchuan.svg"), 0o644); err != nil {
		return err
	}
	desktopFile := `[Desktop Entry]
Type=Application
Name=简传
Comment=局域网文件传输工具
Exec=简传
Icon=jianchuan
Terminal=false
Categories=Utility;Network;
`
	if err := os.WriteFile(filepath.Join(appDir, "jianchuan.desktop"), []byte(desktopFile), 0o644); err != nil {
		return err
	}

	archivePath := packageDir + ".tar.gz"
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := tarGzDir(packageDir, archivePath); err != nil {
		return err
	}
	fmt.Printf("完成: %s\n", binPath)
	fmt.Printf("归档: %s\n", archivePath)
	return nil
}

func buildDesktop(root, output, target, arch string) error {
	args := []string{"build", "-tags", "wails"}
	if target == "windows" {
		args = append(args, "-ldflags", "-H=windowsgui")
	}
	args = append(args, "-o", output, "./cmd/lan-transfer-desktop")
	return runCommand(root, []string{"GOOS=" + target, "GOARCH=" + arch}, "go", args...)
}

func ensureDarwinIcon(root string) error {
	svg := filepath.Join(root, "build", "assets", "app-icon.svg")
	iconset := filepath.Join(root, "build", "darwin", "AppIcon.iconset")
	icns := filepath.Join(root, "build", "darwin", "AppIcon.icns")
	if _, err := exec.LookPath("sips"); err != nil {
		if _, statErr := os.Stat(icns); statErr == nil {
			return nil
		}
		return fmt.Errorf("生成 macOS 图标需要 sips")
	}
	if _, err := exec.LookPath("iconutil"); err != nil {
		if _, statErr := os.Stat(icns); statErr == nil {
			return nil
		}
		return fmt.Errorf("生成 macOS 图标需要 iconutil")
	}
	if err := os.MkdirAll(iconset, 0o755); err != nil {
		return err
	}

	sizes := []struct {
		file string
		size string
	}{
		{"icon_16x16.png", "16"},
		{"icon_16x16@2x.png", "32"},
		{"icon_32x32.png", "32"},
		{"icon_32x32@2x.png", "64"},
		{"icon_128x128.png", "128"},
		{"icon_128x128@2x.png", "256"},
		{"icon_256x256.png", "256"},
		{"icon_256x256@2x.png", "512"},
		{"icon_512x512.png", "512"},
		{"icon_512x512@2x.png", "1024"},
	}
	for _, item := range sizes {
		out := filepath.Join(iconset, item.file)
		if err := runCommandQuiet(root, "sips", "-s", "format", "png", "-z", item.size, item.size, svg, "--out", out); err != nil {
			return err
		}
	}
	return runCommand(root, nil, "iconutil", "-c", "icns", iconset, "-o", icns)
}

func runCommand(dir string, env []string, name string, args ...string) error {
	fmt.Printf("$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCommandQuiet(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func zipDir(source, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	zipWriter := zip.NewWriter(out)
	defer zipWriter.Close()

	base := filepath.Dir(source)
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if entry.IsDir() {
			name += "/"
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.SetMode(info.Mode())
		if !entry.IsDir() {
			header.Method = zip.Deflate
		}
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
}

func tarGzDir(source, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	gzipWriter := gzip.NewWriter(out)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	base := filepath.Dir(source)
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tarWriter, file)
		return err
	})
}

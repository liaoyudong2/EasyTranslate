package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

const (
	appName        = "简传"
	appDescription = "局域网文件传输工具"
	appVersion     = "0.1.0"
	executable     = "lan-transfer-desktop"

	windowsWailsAppIconID = 3
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
	resourcePath, err := ensureWindowsResource(root, opt.arch)
	if err != nil {
		return err
	}
	defer os.Remove(resourcePath)

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
	iconset := filepath.Join(root, "build", "darwin", "AppIcon.iconset")
	icns := filepath.Join(root, "build", "darwin", "AppIcon.icns")
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
		size int
	}{
		{"icon_16x16.png", 16},
		{"icon_16x16@2x.png", 32},
		{"icon_32x32.png", 32},
		{"icon_32x32@2x.png", 64},
		{"icon_128x128.png", 128},
		{"icon_128x128@2x.png", 256},
		{"icon_256x256.png", 256},
		{"icon_256x256@2x.png", 512},
		{"icon_512x512.png", 512},
		{"icon_512x512@2x.png", 1024},
	}
	for _, item := range sizes {
		out := filepath.Join(iconset, item.file)
		if err := writePNG(out, renderAppIcon(item.size)); err != nil {
			return err
		}
	}
	return runCommand(root, nil, "iconutil", "-c", "icns", iconset, "-o", icns)
}

func ensureWindowsResource(root, arch string) (string, error) {
	targetArch, err := windowsResourceArch(arch)
	if err != nil {
		return "", err
	}

	rs, err := newWindowsResourceSet()
	if err != nil {
		return "", err
	}

	resourcePath := filepath.Join(root, "cmd", "lan-transfer-desktop", fmt.Sprintf("rsrc_windows_%s.syso", arch))
	out, err := os.Create(resourcePath)
	if err != nil {
		return "", err
	}
	if err := rs.WriteObject(out, targetArch); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return resourcePath, nil
}

func newWindowsResourceSet() (*winres.ResourceSet, error) {
	icon, err := winres.NewIconFromResizedImage(renderAppIcon(1024), []int{256, 128, 64, 48, 32, 16})
	if err != nil {
		return nil, err
	}

	rs := &winres.ResourceSet{}
	if err := rs.SetIcon(winres.ID(windowsWailsAppIconID), icon); err != nil {
		return nil, err
	}

	vi := version.Info{
		FileVersion:    [4]uint16{0, 1, 0, 0},
		ProductVersion: [4]uint16{0, 1, 0, 0},
	}
	versionFields := map[string]string{
		version.CompanyName:      appName,
		version.FileDescription:  appDescription,
		version.FileVersion:      appVersion,
		version.InternalName:     executable,
		version.OriginalFilename: appName + ".exe",
		version.ProductName:      appName,
		version.ProductVersion:   appVersion,
	}
	for key, value := range versionFields {
		if err := vi.Set(version.LangDefault, key, value); err != nil {
			return nil, err
		}
	}
	rs.SetVersionInfo(vi)
	rs.SetManifest(winres.AppManifest{
		Identity: winres.AssemblyIdentity{
			Name:    "com.local.lantransfer",
			Version: [4]uint16{0, 1, 0, 0},
		},
		Description:         appDescription,
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		LongPathAware:       true,
		UseCommonControlsV6: true,
	})
	return rs, nil
}

func windowsResourceArch(arch string) (winres.Arch, error) {
	switch arch {
	case "386":
		return winres.ArchI386, nil
	case "amd64":
		return winres.ArchAMD64, nil
	case "arm":
		return winres.ArchARM, nil
	case "arm64":
		return winres.ArchARM64, nil
	default:
		return "", fmt.Errorf("不支持的 Windows 资源架构: %s", arch)
	}
}

func renderAppIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / 1024
	s := func(v float64) float64 { return v * scale }

	fillRoundedRect(img, s(0), s(0), s(1024), s(1024), s(228), func(x, y float64) color.NRGBA {
		t := clamp((x+y)/(2*float64(size)), 0, 1)
		return lerpColor(color.NRGBA{R: 12, G: 166, B: 120, A: 255}, color.NRGBA{R: 54, G: 79, B: 199, A: 255}, t)
	})
	fillRoundedRect(img, s(232), s(168), s(760), s(876), s(72), func(x, y float64) color.NRGBA {
		t := clamp((y-s(168))/(s(876)-s(168)), 0, 1)
		return lerpColor(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{R: 231, G: 245, B: 255, A: 255}, t)
	})
	fillPolygon(img, []point{
		{s(610), s(168)}, {s(760), s(318)}, {s(672), s(348)}, {s(610), s(286)},
	}, color.NRGBA{R: 191, G: 233, B: 255, A: 255})

	teal := color.NRGBA{R: 11, G: 114, B: 133, A: 255}
	green := color.NRGBA{R: 43, G: 138, B: 62, A: 255}
	drawStroke(img, s(350), s(656), s(638), s(656), s(46), teal)
	drawStroke(img, s(559), s(558), s(655), s(656), s(46), teal)
	drawStroke(img, s(655), s(656), s(559), s(754), s(46), teal)

	drawStroke(img, s(674), s(444), s(386), s(444), s(46), green)
	drawStroke(img, s(465), s(346), s(369), s(444), s(46), green)
	drawStroke(img, s(369), s(444), s(465), s(542), s(46), green)

	fillCircle(img, s(304), s(444), s(28), color.NRGBA{R: 18, G: 184, B: 134, A: 255})
	fillCircle(img, s(720), s(656), s(28), color.NRGBA{R: 21, G: 170, B: 191, A: 255})
	return img
}

type point struct {
	x float64
	y float64
}

func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1, radius float64, shade func(x, y float64) color.NRGBA) {
	minX, minY := int(math.Floor(x0)), int(math.Floor(y0))
	maxX, maxY := int(math.Ceil(x1)), int(math.Ceil(y1))
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if !insideRoundedRect(px, py, x0, y0, x1, y1, radius) {
				continue
			}
			setPixel(img, x, y, shade(px, py))
		}
	}
}

func insideRoundedRect(x, y, x0, y0, x1, y1, radius float64) bool {
	if x < x0 || x >= x1 || y < y0 || y >= y1 {
		return false
	}
	cx := clamp(x, x0+radius, x1-radius)
	cy := clamp(y, y0+radius, y1-radius)
	return math.Hypot(x-cx, y-cy) <= radius
}

func fillPolygon(img *image.RGBA, pts []point, c color.NRGBA) {
	if len(pts) < 3 {
		return
	}
	minY, maxY := pts[0].y, pts[0].y
	for _, p := range pts[1:] {
		minY = math.Min(minY, p.y)
		maxY = math.Max(maxY, p.y)
	}
	for y := int(math.Floor(minY)); y <= int(math.Ceil(maxY)); y++ {
		scanY := float64(y) + 0.5
		var xs []float64
		for i, p1 := range pts {
			p2 := pts[(i+1)%len(pts)]
			if (p1.y <= scanY && p2.y > scanY) || (p2.y <= scanY && p1.y > scanY) {
				x := p1.x + (scanY-p1.y)*(p2.x-p1.x)/(p2.y-p1.y)
				xs = append(xs, x)
			}
		}
		for i := 0; i+1 < len(xs); i += 2 {
			if xs[i] > xs[i+1] {
				xs[i], xs[i+1] = xs[i+1], xs[i]
			}
			for x := int(math.Floor(xs[i])); x <= int(math.Ceil(xs[i+1])); x++ {
				setPixel(img, x, y, c)
			}
		}
	}
}

func drawStroke(img *image.RGBA, x0, y0, x1, y1, width float64, c color.NRGBA) {
	radius := width / 2
	minX := int(math.Floor(math.Min(x0, x1) - radius))
	maxX := int(math.Ceil(math.Max(x0, x1) + radius))
	minY := int(math.Floor(math.Min(y0, y1) - radius))
	maxY := int(math.Ceil(math.Max(y0, y1) + radius))
	dx, dy := x1-x0, y1-y0
	length2 := dx*dx + dy*dy
	if length2 == 0 {
		fillCircle(img, x0, y0, radius, c)
		return
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := clamp(((px-x0)*dx+(py-y0)*dy)/length2, 0, 1)
			nx, ny := x0+t*dx, y0+t*dy
			if math.Hypot(px-nx, py-ny) <= radius {
				setPixel(img, x, y, c)
			}
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, radius float64, c color.NRGBA) {
	minX := int(math.Floor(cx - radius))
	maxX := int(math.Ceil(cx + radius))
	minY := int(math.Floor(cy - radius))
	maxY := int(math.Ceil(cy + radius))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) <= radius {
				setPixel(img, x, y, c)
			}
		}
	}
}

func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

func setPixel(img *image.RGBA, x, y int, c color.NRGBA) {
	if image.Pt(x, y).In(img.Bounds()) {
		img.Set(x, y, c)
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
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

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
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

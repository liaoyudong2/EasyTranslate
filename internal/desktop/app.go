package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"lan-transfer/internal/config"
	"lan-transfer/internal/server"
)

type App struct {
	mu     sync.Mutex
	cfg    config.Config
	srv    *server.Server
	status server.Status
}

type Snapshot struct {
	Config config.Config `json:"config"`
	Status server.Status `json:"status"`
}

func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	cfg = config.Normalize(cfg)
	if config.IsLegacyRootStorageDir(cfg.StorageDir) {
		cfg.StorageDir = config.DefaultStorageDir()
	}
	app := &App{cfg: cfg}
	if cfg.AutoStart {
		_ = app.StartService()
	}
	return app
}

func (a *App) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	status := a.status
	if a.srv != nil {
		status = a.srv.Status()
	}
	if status.Port == 0 {
		status = server.Status{
			Running:    false,
			Message:    "未启动",
			Port:       a.cfg.Port,
			StorageDir: a.cfg.StorageDir,
			AccessCode: a.cfg.AccessCode,
		}
	}
	return Snapshot{Config: a.cfg, Status: status}
}

func (a *App) StartService() error {
	a.mu.Lock()
	if a.srv != nil && a.srv.Status().Running {
		a.mu.Unlock()
		return nil
	}
	cfg := config.Normalize(a.cfg)
	srv, err := server.New(server.Options{Config: cfg})
	if err != nil {
		a.status = server.Status{
			Running:    false,
			Message:    err.Error(),
			Port:       cfg.Port,
			StorageDir: cfg.StorageDir,
			AccessCode: cfg.AccessCode,
		}
		a.mu.Unlock()
		return err
	}
	if err := srv.Start(); err != nil {
		a.status = srv.Status()
		a.mu.Unlock()
		return err
	}
	a.cfg = cfg
	a.srv = srv
	a.status = srv.Status()
	a.mu.Unlock()
	return nil
}

func (a *App) StopService() error {
	a.mu.Lock()
	srv := a.srv
	a.mu.Unlock()
	if srv == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := srv.Stop(ctx)

	a.mu.Lock()
	a.status = srv.Status()
	a.srv = nil
	a.mu.Unlock()
	return err
}

func (a *App) SaveConfig(cfg config.Config) error {
	cfg = config.Normalize(cfg)
	a.mu.Lock()
	running := a.srv != nil && a.srv.Status().Running
	a.cfg = cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		return err
	}
	if running {
		if err := a.StopService(); err != nil {
			return err
		}
		return a.StartService()
	}
	return nil
}

func (a *App) PrepareDirectoryPicker(current string) (string, error) {
	a.mu.Lock()
	fallback := a.cfg.StorageDir
	a.mu.Unlock()
	return resolveDialogDirectory(current, fallback)
}

func (a *App) OpenBrowser() error {
	snapshot := a.Snapshot()
	if len(snapshot.Status.URLs) == 0 {
		return fmt.Errorf("服务未启动")
	}
	return openURL(snapshot.Status.URLs[0])
}

func (a *App) PrimaryURL() string {
	snapshot := a.Snapshot()
	if len(snapshot.Status.URLs) == 0 {
		return ""
	}
	return snapshot.Status.URLs[0]
}

func resolveDialogDirectory(current, fallback string) (string, error) {
	candidates := []string{current, fallback, config.DefaultStorageDir()}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, home)
	}
	for _, candidate := range candidates {
		if dir, ok := ensureDialogDirectory(candidate); ok {
			return dir, nil
		}
	}
	return "", fmt.Errorf("没有可用的目录")
}

func ensureDialogDirectory(dir string) (string, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err == nil {
		if info.IsDir() {
			return abs, true
		}
		return nearestExistingDir(filepath.Dir(abs))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nearestExistingDir(abs)
	}
	if err := os.MkdirAll(abs, 0o755); err == nil {
		return abs, true
	}
	return nearestExistingDir(abs)
}

func nearestExistingDir(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	for {
		info, statErr := os.Stat(abs)
		if statErr == nil && info.IsDir() {
			return abs, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

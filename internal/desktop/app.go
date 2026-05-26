package desktop

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
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

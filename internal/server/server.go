package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"lan-transfer/internal/config"
	"lan-transfer/internal/netutil"
	"lan-transfer/internal/qr"
	"lan-transfer/internal/storage"
)

//go:embed static/*
var staticFiles embed.FS

type Options struct {
	Config config.Config
	Logger *slog.Logger
}

type Status struct {
	Running    bool     `json:"running"`
	Message    string   `json:"message"`
	Port       int      `json:"port"`
	StorageDir string   `json:"storageDir"`
	AccessCode string   `json:"accessCode"`
	URLs       []string `json:"urls"`
}

type Server struct {
	cfg    config.Config
	store  *storage.Store
	logger *slog.Logger

	mu       sync.Mutex
	sessions map[string]time.Time
	httpSrv  *http.Server
	listener net.Listener
	running  bool
	message  string
}

func New(opts Options) (*Server, error) {
	cfg := config.Normalize(opts.Config)
	store, err := storage.New(cfg.StorageDir)
	if err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:      cfg,
		store:    store,
		logger:   logger,
		sessions: map[string]time.Time{},
		message:  "未启动",
	}, nil
}

func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	addr := fmt.Sprintf("0.0.0.0:%d", s.cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		s.message = friendlyListenError(err)
		s.mu.Unlock()
		return err
	}

	s.listener = listener
	s.httpSrv = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.running = true
	s.message = "监听中"
	s.mu.Unlock()

	go func() {
		if err := s.httpSrv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("服务异常停止", "error", err)
			s.mu.Lock()
			s.running = false
			s.message = err.Error()
			s.mu.Unlock()
		}
	}()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running || s.httpSrv == nil {
		s.running = false
		s.message = "未启动"
		s.mu.Unlock()
		return nil
	}
	srv := s.httpSrv
	s.running = false
	s.message = "未启动"
	s.mu.Unlock()
	return srv.Shutdown(ctx)
}

func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{
		Running:    s.running,
		Message:    s.message,
		Port:       s.cfg.Port,
		StorageDir: s.store.Dir(),
		AccessCode: s.cfg.AccessCode,
		URLs:       netutil.LANAddresses(s.cfg.Port),
	}
}

func (s *Server) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/files", s.requireAuth(s.handleFiles))
	mux.HandleFunc("/api/files/", s.requireAuth(s.handleFile))
	mux.HandleFunc("/api/share/qr.png", s.handleQR)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.Handle("/", s.staticHandler())
	return mux
}

func (s *Server) staticHandler() http.HandlerFunc {
	content, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(content))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFileFS(w, r, content, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Code), []byte(s.cfg.AccessCode)) != 1 {
		writeError(w, http.StatusUnauthorized, "访问码错误")
		return
	}
	token, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建会话")
		return
	}
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(24 * time.Hour)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "lan_transfer_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	cookie, err := r.Cookie("lan_transfer_session")
	if err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "lan_transfer_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	status := s.Status()
	status.AccessCode = ""
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		files, err := s.store.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": files})
	case http.MethodPost:
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "上传内容无效")
			return
		}
		headers := r.MultipartForm.File["files"]
		if len(headers) == 0 {
			writeError(w, http.StatusBadRequest, "请选择文件")
			return
		}
		saved := make([]storage.FileInfo, 0, len(headers))
		for _, header := range headers {
			file, err := header.Open()
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			info, err := s.store.SaveMultipart(file, header)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			saved = append(saved, info)
		}
		writeJSON(w, http.StatusCreated, map[string]any{"files": saved})
	default:
		writeError(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/files/")
	id = path.Clean("/" + id)
	id = strings.TrimPrefix(id, "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "文件 ID 无效")
		return
	}

	switch r.Method {
	case http.MethodGet:
		file, info, err := s.store.Open(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		defer file.Close()
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name))
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, info.Name, info.MTime, file)
	case http.MethodDelete:
		if err := s.store.Delete(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "缺少 url")
		return
	}
	png, err := qr.PNG(rawURL, 256)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "二维码生成失败")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("lan_transfer_session")
		if err != nil || !s.validSession(cookie.Value) {
			writeError(w, http.StatusUnauthorized, "请先输入访问码")
			return
		}
		next(w, r)
	}
}

func (s *Server) validSession(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expireAt, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(expireAt) {
		delete(s.sessions, token)
		return false
	}
	return true
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": message,
	})
}

func friendlyListenError(err error) string {
	if strings.Contains(err.Error(), "address already in use") || strings.Contains(err.Error(), "bind: Only one usage") {
		return "端口被占用"
	}
	return err.Error()
}

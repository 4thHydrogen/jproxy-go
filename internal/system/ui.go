package system

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

type UIService struct {
	distPath string
}

func NewUIService(distPath string) *UIService {
	return &UIService{distPath: distPath}
}

func (s *UIService) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/user/isLoginEnabled", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, false)
	})
	mux.HandleFunc("/api/system/user/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "anonymous"})
	})
	mux.HandleFunc("/api/system/user/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"username": "Anonymous",
			"role":     "admin",
			"password": "******",
		})
	})
	mux.HandleFunc("/api/system/user/update", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "login disabled", http.StatusUnauthorized)
	})
	mux.HandleFunc("/api/system/user/logout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", s.serveStatic)
}

func (s *UIService) serveStatic(w http.ResponseWriter, r *http.Request) {
	if s.distPath == "" {
		http.NotFound(w, r)
		return
	}
	target := filepath.Join(s.distPath, filepath.Clean(r.URL.Path))
	if r.URL.Path == "/" {
		target = filepath.Join(s.distPath, "index.html")
	}
	info, err := os.Stat(target)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.distPath, "index.html"))
}

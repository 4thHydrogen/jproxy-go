package system

import (
	"encoding/json"
	"net/http"

	"jproxy/core-proxy/internal/app"
	"jproxy/core-proxy/internal/model"
)

type HTTPServer struct {
	proxy  *app.Service
	config *ConfigService
	syncer *SyncService
}

func NewHTTPServer(proxy *app.Service, config *ConfigService, syncer *SyncService) *HTTPServer {
	return &HTTPServer{proxy: proxy, config: config, syncer: syncer}
}

func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux) {
	s.proxy.RegisterRoutes(mux)
	mux.HandleFunc("/api/rule/test", s.handleRuleTest)
	mux.HandleFunc("/api/system/config/version", s.handleVersion)
	mux.HandleFunc("/api/system/config/author/list", s.handleAuthorList)
	mux.HandleFunc("/api/system/config/query", s.handleConfigQuery)
	mux.HandleFunc("/api/system/config/update", s.handleConfigUpdate)
	mux.HandleFunc("/api/system/sync/run", s.handleSyncRun)
	mux.HandleFunc("/api/system/sync/status", s.handleSyncStatus)
}

func (s *HTTPServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("go-rewrite"))
}

func (s *HTTPServer) handleAuthorList(w http.ResponseWriter, r *http.Request) {
	base, err := s.syncer.ruleLocation(r.Context())
	if err != nil {
		writeResult(w, []string{"LuckyPuppy514"}, nil)
		return
	}
	authors, err := s.syncer.ruleAuthors(r.Context(), base)
	if err != nil || len(authors) == 0 {
		writeResult(w, []string{"LuckyPuppy514"}, nil)
		return
	}
	writeResult(w, authors, nil)
}

func (s *HTTPServer) handleRuleTest(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	result, err := testRule(values.Get("regex"), values.Get("replacement"), values.Get("example"), atoiDefault(values.Get("offset"), 0))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(result))
}

func (s *HTTPServer) handleConfigQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := s.config.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (s *HTTPServer) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var items []model.SystemConfig
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.config.Update(r.Context(), items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.proxy.ResetCaches()
	w.WriteHeader(http.StatusOK)
}

func (s *HTTPServer) handleSyncRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	job := r.URL.Query().Get("job")
	if job == "" {
		job = "all"
	}
	if err := s.syncer.Run(r.Context(), job); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *HTTPServer) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.syncer.Status())
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

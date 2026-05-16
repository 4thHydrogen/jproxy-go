package system

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"jproxy/core-proxy/internal/model"
	"jproxy/core-proxy/internal/repository"
	"jproxy/core-proxy/internal/util"
)

const primaryTitleRuleID = "00000000000000000000000000000000"

type AdminService struct {
	repo   *repository.SQLiteRepository
	proxy  interface{ ResetCaches() }
	syncer *SyncService
}

func NewAdminService(repo *repository.SQLiteRepository, proxy interface{ ResetCaches() }, syncer *SyncService) *AdminService {
	return &AdminService{repo: repo, proxy: proxy, syncer: syncer}
}

func (s *AdminService) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/system/cache/clearAll", s.handleClearCache)
	mux.HandleFunc("/api/system/cache/clear", s.handleClearCache)

	s.registerRuleRoutes(mux, "/api/sonarr/rule", "sonarr_rule", "sonarr-rule")
	s.registerRuleRoutes(mux, "/api/radarr/rule", "radarr_rule", "radarr-rule")
	s.registerTitleRoutes(mux, "/api/sonarr/title", "sonarr_title", "sonarr-title")
	s.registerTitleRoutes(mux, "/api/radarr/title", "radarr_title", "radarr-title")
	s.registerExampleRoutes(mux, "/api/sonarr/example", "sonarr_example")
	s.registerExampleRoutes(mux, "/api/radarr/example", "radarr_example")
	mux.HandleFunc("/api/tmdb/title/query", s.handleTmdbQuery)
	mux.HandleFunc("/api/tmdb/title/remove", s.handleTmdbRemove)
	mux.HandleFunc("/api/tmdb/title/save", s.handleTmdbSave)
	mux.HandleFunc("/api/tmdb/title/sync", s.handleTmdbSync)
}

func (s *AdminService) registerExampleRoutes(mux *http.ServeMux, prefix, table string) {
	mux.HandleFunc(prefix+"/query", func(w http.ResponseWriter, r *http.Request) {
		page, err := s.repo.QueryExamples(r.Context(), table, pageQueryFromRequest(r))
		writeResult(w, page, err)
	})
	mux.HandleFunc(prefix+"/save", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			OriginalText string `json:"originalText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var examples []model.Example
		for _, line := range strings.Split(payload.OriginalText, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			examples = append(examples, model.Example{
				Hash:         strings.ToUpper(fmtMD5(line)),
				OriginalText: line,
				ValidStatus:  1,
			})
		}
		writeEmpty(w, s.repo.SaveExamples(r.Context(), table, examples))
	})
	mux.HandleFunc(prefix+"/remove", func(w http.ResponseWriter, r *http.Request) {
		var ids []string
		if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeEmpty(w, s.repo.RemoveByTextIDs(r.Context(), table, ids))
	})
}

func (s *AdminService) registerRuleRoutes(mux *http.ServeMux, prefix, table, syncJob string) {
	mux.HandleFunc(prefix+"/query", func(w http.ResponseWriter, r *http.Request) {
		page, err := s.repo.QueryRules(r.Context(), table, pageQueryFromRequest(r))
		writeResult(w, page, err)
	})
	mux.HandleFunc(prefix+"/save", func(w http.ResponseWriter, r *http.Request) {
		var rule model.RuleRecord
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if rule.ID == "" {
			rule.ID = newID()
		}
		if rule.ID == primaryTitleRuleID {
			http.Error(w, "primary title rule can not be modified", http.StatusBadRequest)
			return
		}
		if _, err := regexp.Compile(rule.Regex); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err := s.repo.SaveRule(r.Context(), table, rule)
		s.proxy.ResetCaches()
		writeEmpty(w, err)
	})
	mux.HandleFunc(prefix+"/remove", func(w http.ResponseWriter, r *http.Request) {
		var ids []string
		if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, id := range ids {
			if id == primaryTitleRuleID {
				http.Error(w, "primary title rule can not be modified", http.StatusBadRequest)
				return
			}
		}
		err := s.repo.RemoveByTextIDs(r.Context(), table, ids)
		s.proxy.ResetCaches()
		writeEmpty(w, err)
	})
	mux.HandleFunc(prefix+"/enable", func(w http.ResponseWriter, r *http.Request) {
		s.handleRuleStatus(w, r, table, 1)
	})
	mux.HandleFunc(prefix+"/disable", func(w http.ResponseWriter, r *http.Request) {
		s.handleRuleStatus(w, r, table, 0)
	})
	mux.HandleFunc(prefix+"/token/list", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.repo.ListRuleTokens(r.Context(), table)
		writeResult(w, items, err)
	})
	mux.HandleFunc(prefix+"/export", func(w http.ResponseWriter, r *http.Request) {
		page, err := s.repo.QueryRules(r.Context(), table, model.PageQuery{Current: 1, PageSize: 100000})
		writeResult(w, page.List, err)
	})
	mux.HandleFunc(prefix+"/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, 8<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var rules []model.RuleRecord
		if err := json.Unmarshal(content, &rules); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, rule := range rules {
			if rule.ID == primaryTitleRuleID {
				http.Error(w, "primary title rule can not be modified", http.StatusBadRequest)
				return
			}
			if _, err := regexp.Compile(rule.Regex); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		err = s.repo.UpsertRules(r.Context(), table, rules)
		s.proxy.ResetCaches()
		writeEmpty(w, err)
	})
	mux.HandleFunc(prefix+"/sync", func(w http.ResponseWriter, r *http.Request) {
		err := s.syncer.Run(r.Context(), syncJob)
		s.proxy.ResetCaches()
		writeEmpty(w, err)
	})
}

func (s *AdminService) registerTitleRoutes(mux *http.ServeMux, prefix, table, syncJob string) {
	mux.HandleFunc(prefix+"/query", func(w http.ResponseWriter, r *http.Request) {
		var result any
		var err error
		if table == "sonarr_title" {
			result, err = s.repo.QuerySonarrTitles(r.Context(), pageQueryFromRequest(r))
		} else {
			result, err = s.repo.QueryRadarrTitles(r.Context(), pageQueryFromRequest(r))
		}
		writeResult(w, result, err)
	})
	mux.HandleFunc(prefix+"/remove", func(w http.ResponseWriter, r *http.Request) {
		var ids []int
		if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err := s.repo.RemoveByIntIDs(r.Context(), table, ids)
		s.proxy.ResetCaches()
		writeEmpty(w, err)
	})
	mux.HandleFunc(prefix+"/sync", func(w http.ResponseWriter, r *http.Request) {
		err := s.syncer.Run(r.Context(), syncJob)
		s.proxy.ResetCaches()
		writeEmpty(w, err)
	})
}

func (s *AdminService) handleRuleStatus(w http.ResponseWriter, r *http.Request, table string, status int) {
	var ids []string
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, id := range ids {
		if status == 0 && id == primaryTitleRuleID {
			http.Error(w, "primary title rule can not be modified", http.StatusBadRequest)
			return
		}
	}
	err := s.repo.SetRuleValidStatus(r.Context(), table, ids, status)
	s.proxy.ResetCaches()
	writeEmpty(w, err)
}

func (s *AdminService) handleClearCache(w http.ResponseWriter, r *http.Request) {
	s.proxy.ResetCaches()
	w.WriteHeader(http.StatusOK)
}

func (s *AdminService) handleTmdbQuery(w http.ResponseWriter, r *http.Request) {
	query := pageQueryFromRequest(r)
	page, err := s.repo.QueryTmdbTitles(r.Context(), query)
	if err == nil {
		cleanRegex, cleanErr := s.repo.ConfigValue("cleanTitleRegex")
		if cleanErr == nil {
			for i := range page.List {
				page.List[i].CleanTitle = util.CleanTitle(page.List[i].Title, cleanRegex)
			}
		}
	}
	writeResult(w, page, err)
}

func (s *AdminService) handleTmdbRemove(w http.ResponseWriter, r *http.Request) {
	var ids []int
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err := s.repo.RemoveByIntIDs(r.Context(), "tmdb_title", ids)
	s.proxy.ResetCaches()
	writeEmpty(w, err)
}

func (s *AdminService) handleTmdbSave(w http.ResponseWriter, r *http.Request) {
	var title model.TmdbTitle
	if err := json.NewDecoder(r.Body).Decode(&title); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err := s.repo.SaveTmdbTitle(r.Context(), title)
	s.proxy.ResetCaches()
	writeEmpty(w, err)
}

func (s *AdminService) handleTmdbSync(w http.ResponseWriter, r *http.Request) {
	ids, err := s.repo.NeedSyncTmdbIDs(r.Context())
	if err == nil {
		err = s.syncer.SyncTmdbTitles(r.Context(), ids)
	}
	s.proxy.ResetCaches()
	writeEmpty(w, err)
}

func pageQueryFromRequest(r *http.Request) model.PageQuery {
	values := r.URL.Query()
	return model.PageQuery{
		Current:      atoiDefault(values.Get("current"), 1),
		PageSize:     atoiDefault(values.Get("pageSize"), 10),
		Title:        strings.TrimSpace(values.Get("title")),
		Token:        strings.TrimSpace(values.Get("token")),
		Remark:       strings.TrimSpace(values.Get("remark")),
		OriginalText: strings.TrimSpace(values.Get("originalText")),
		ValidStatus:  atoiDefault(values.Get("validStatus"), -1),
		TVDBID:       atoiDefault(values.Get("tvdbId"), 0),
		TMDBID:       atoiDefault(values.Get("tmdbId"), 0),
	}
}

func fmtMD5(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func atoiDefault(value string, fallback int) int {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return number
}

func writeResult(w http.ResponseWriter, payload any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, payload)
}

func writeEmpty(w http.ResponseWriter, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func newID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strings.ToUpper(strconv.FormatInt(time.Now().UnixNano(), 16))
	}
	return strings.ToUpper(hex.EncodeToString(buf[:]))
}

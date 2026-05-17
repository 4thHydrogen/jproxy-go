package system

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jproxy/core-proxy/internal/model"
	"jproxy/core-proxy/internal/repository"
)

func TestSyncSonarrTitlesAndTMDB(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v3/series"):
			_, _ = w.Write([]byte(`[{"id":11,"tvdbId":101,"title":"Three Body Problem","titleSlug":"three-body-problem","monitored":true,"alternateTitles":[{"title":"San Ti","sceneSeasonNumber":1}]}]`))
		case strings.HasPrefix(r.URL.Path, "/3/find/101"):
			_, _ = w.Write([]byte(`{"tv_results":[{"id":555,"name":"三体"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repo, err := NewTempRepo(t)
	if err != nil {
		t.Fatalf("temp repo: %v", err)
	}
	defer repo.Close()

	if err := setConfigValues(context.Background(), repo, map[string]string{
		"sonarrUrl":       upstream.URL,
		"sonarrApikey":    "abc",
		"tmdbUrl":         upstream.URL,
		"tmdbApikey":      "tmdb",
		"sonarrLanguage1": "zh-CN",
		"sonarrLanguage2": "en-US",
	}); err != nil {
		t.Fatalf("set configs: %v", err)
	}

	service := NewSyncService(repo, upstream.Client())
	if err := service.SyncSonarrTitles(context.Background()); err != nil {
		t.Fatalf("sync sonarr titles: %v", err)
	}

	item, err := repo.SonarrTitleByCleanTitle("three body problem")
	if err != nil {
		t.Fatalf("load sonarr title: %v", err)
	}
	if item == nil || item.TVDBID != 101 {
		t.Fatalf("unexpected sonarr title: %+v", item)
	}

	tmdbItems, err := repo.SonarrTMDBTitles(101)
	if err != nil {
		t.Fatalf("load tmdb titles: %v", err)
	}
	if len(tmdbItems) == 0 || tmdbItems[0].Title != "三体" {
		t.Fatalf("unexpected tmdb titles: %+v", tmdbItems)
	}
}

func NewTempRepo(t *testing.T) (*repository.SQLiteRepository, error) {
	t.Helper()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "jproxy.db"))
	if err != nil {
		return nil, err
	}
	if err := repo.EnsureSchema(context.Background()); err != nil {
		_ = repo.Close()
		return nil, err
	}
	return repo, nil
}

func setConfigValues(ctx context.Context, repo *repository.SQLiteRepository, values map[string]string) error {
	configs, err := repo.ListConfigs(ctx)
	if err != nil {
		return err
	}
	for index := range configs {
		if value, ok := values[configs[index].Key]; ok {
			configs[index].Value = value
			configs[index].ValidStatus = 1
		}
	}
	return repo.UpdateConfigs(ctx, configs)
}

func TestNormalizeTransmissionURL(t *testing.T) {
	got := normalizeTransmissionURL("http://host:9091/transmission/web/")
	if got != "http://host:9091/transmission/rpc" {
		t.Fatalf("unexpected transmission url: %s", got)
	}
}

func TestSyncRunSerializesSameJob(t *testing.T) {
	var active int32
	var maxActive int32
	release := make(chan struct{})
	var once sync.Once

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v3/series") {
			http.NotFound(w, r)
			return
		}
		current := atomic.AddInt32(&active, 1)
		for {
			previous := atomic.LoadInt32(&maxActive)
			if current <= previous || atomic.CompareAndSwapInt32(&maxActive, previous, current) {
				break
			}
		}
		once.Do(func() {
			time.AfterFunc(50*time.Millisecond, func() {
				close(release)
			})
		})
		<-release
		atomic.AddInt32(&active, -1)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer upstream.Close()

	repo, err := NewTempRepo(t)
	if err != nil {
		t.Fatalf("temp repo: %v", err)
	}
	defer repo.Close()
	if err := setConfigValues(context.Background(), repo, map[string]string{
		"sonarrUrl":       upstream.URL,
		"sonarrApikey":    "abc",
		"tmdbUrl":         upstream.URL,
		"tmdbApikey":      "tmdb",
		"sonarrLanguage1": "zh-CN",
		"sonarrLanguage2": "en-US",
	}); err != nil {
		t.Fatalf("set configs: %v", err)
	}

	service := NewSyncService(repo, upstream.Client())
	errs := make(chan error, 2)
	go func() { errs <- service.Run(context.Background(), "sonarr-title") }()
	go func() { errs <- service.Run(context.Background(), "sonarr-title") }()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("sync run: %v", err)
		}
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("sync job overlapped upstream calls, max active = %d", got)
	}
}

func TestSyncGetRejectsOversizedResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxSyncResponseLength+1)))
	}))
	defer upstream.Close()

	service := NewSyncService(nil, upstream.Client())
	_, err := service.get(context.Background(), upstream.URL)
	if !errors.Is(err, errSyncResponseTooLarge) {
		t.Fatalf("expected oversized response error, got %v", err)
	}
}

func TestSyncRulesRejectInvalidRegex(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sonarr@bad.json") {
			rules := []model.RuleRecord{{
				ID:          "bad-regex",
				Token:       "title",
				Priority:    1,
				Regex:       "(",
				Replacement: "",
				ValidStatus: 1,
			}}
			writeJSON(w, rules)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	repo, err := NewTempRepo(t)
	if err != nil {
		t.Fatalf("temp repo: %v", err)
	}
	defer repo.Close()
	if err := setConfigValues(context.Background(), repo, map[string]string{
		"ruleSyncAuthors": "bad",
	}); err != nil {
		t.Fatalf("set configs: %v", err)
	}

	service := NewSyncService(repo, upstream.Client())
	service.ruleBase = upstream.URL
	if err := service.SyncSonarrRules(context.Background()); err == nil {
		t.Fatalf("expected invalid regex to fail sync")
	}
	page, err := repo.QueryRules(context.Background(), "sonarr_rule", model.PageQuery{Current: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("query rules: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("invalid rule was saved: %+v", page.List)
	}
}

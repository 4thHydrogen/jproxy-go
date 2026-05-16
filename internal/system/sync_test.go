package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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

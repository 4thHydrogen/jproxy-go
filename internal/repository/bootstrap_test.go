package repository

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEnsureSchemaSeedsDefaultConfigs(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "jproxy.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	defer repo.Close()

	if err := repo.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	configs, err := repo.ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("list configs: %v", err)
	}
	if len(configs) < 10 {
		t.Fatalf("expected seeded configs, got %d", len(configs))
	}

	value, err := repo.ConfigValue("tmdbUrl")
	if err != nil {
		t.Fatalf("config value: %v", err)
	}
	if value != "https://api.themoviedb.org" {
		t.Fatalf("unexpected tmdbUrl: %s", value)
	}
}

func TestCatalogQueriesTolerateNullCleanTitle(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "jproxy.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	defer repo.Close()

	if err := repo.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	_, err = repo.db.ExecContext(context.Background(), `
		INSERT INTO sonarr_title (id, series_id, tvdb_id, sno, main_title, title, clean_title, season_number, monitored, valid_status)
		VALUES (1, 1, 100, 0, '主标题', 'Main Title', 'main title', 1, 1, 1);
		INSERT INTO tmdb_title (id, tvdb_id, tmdb_id, language, title, valid_status)
		VALUES (1, 100, 200, 'zh-CN', '别名', 1);
		INSERT INTO radarr_title (id, movie_id, tmdb_id, sno, main_title, title, clean_title, year, monitored, valid_status)
		VALUES (1, 1, 300, 0, '电影', 'Movie', 'movie', 2024, 1, 1);
	`)
	if err != nil {
		t.Fatalf("insert null title rows: %v", err)
	}

	sonarr, err := repo.SonarrCatalog()
	if err != nil {
		t.Fatalf("sonarr catalog: %v", err)
	}
	if len(sonarr) < 2 {
		t.Fatalf("expected sonarr catalog rows with tmdb alias, got %+v", sonarr)
	}

	radarr, err := repo.RadarrCatalog()
	if err != nil {
		t.Fatalf("radarr catalog: %v", err)
	}
	if len(radarr) != 1 || radarr[0].CleanTitle != "movie" {
		t.Fatalf("unexpected radarr catalog rows: %+v", radarr)
	}
}

package repository

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed sql/bootstrap.sql
var bootstrapSQL string

var defaultConfigs = []struct {
	ID          int
	Key         string
	Value       string
	ValidStatus int
}{
	{1, "sonarrUrl", "", 0},
	{2, "sonarrApikey", "", 0},
	{3, "sonarrIndexerFormat", "{title} {season}{episode} {language}{subtitle}{resolution}{quality}{dynamic_range}{group}", 1},
	{5, "sonarrLanguage1", "zh-CN", 1},
	{6, "sonarrLanguage2", "zh-TW", 1},
	{7, "radarrUrl", "", 0},
	{8, "radarrApikey", "", 0},
	{9, "radarrIndexerFormat", "{title} {year} {language}{subtitle}{resolution}{quality}{dynamic_range}{group}", 1},
	{10, "jackettUrl", "", 0},
	{11, "prowlarrUrl", "", 0},
	{12, "qbittorrentUrl", "", 0},
	{13, "transmissionUrl", "", 0},
	{14, "tmdbUrl", "https://api.themoviedb.org", 1},
	{15, "tmdbApikey", "", 0},
	{16, "cleanTitleRegex", "(`|,|~|!|@|#|%|&|_|=|''|\"|:|<|>|-|鈥攟路|锛寍锝瀨銆亅銆倈鈥榺鈥檤鈥渱鈥潀锛焲锛亅锛殀锛坾锛墊銆恷銆憒銆妡銆媩鈾€|/)", 1},
	{17, "ruleSyncAuthors", "ALL", 1},
	{18, "qbittorrentUsername", "", 0},
	{19, "qbittorrentPassword", "", 0},
	{21, "transmissionUsername", "", 0},
	{22, "transmissionPassword", "", 0},
}

func (r *SQLiteRepository) EnsureSchema(ctx context.Context) error {
	if err := r.execStatements(ctx, bootstrapSQL); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "sonarr_title", "series_id", "ALTER TABLE sonarr_title ADD COLUMN series_id INTEGER"); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "radarr_title", "movie_id", "ALTER TABLE radarr_title ADD COLUMN movie_id INTEGER"); err != nil {
		return err
	}
	for _, config := range defaultConfigs {
		if _, err := r.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO system_config (id, "key", value, valid_status) VALUES (?, ?, ?, ?)`,
			config.ID, config.Key, config.Value, config.ValidStatus,
		); err != nil {
			return fmt.Errorf("seed config %s: %w", config.Key, err)
		}
	}
	return nil
}

func (r *SQLiteRepository) execStatements(ctx context.Context, payload string) error {
	for _, statement := range strings.Split(payload, ";") {
		trimmed := strings.TrimSpace(statement)
		if trimmed == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, trimmed); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) ensureColumn(ctx context.Context, table, column, ddl string) error {
	rows, err := r.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = r.db.ExecContext(ctx, ddl)
	return err
}

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"

	"jproxy/core-proxy/internal/model"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", url.PathEscape(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *SQLiteRepository) ConfigValue(key string) (string, error) {
	var value string
	err := r.db.QueryRow(`SELECT value FROM system_config WHERE "key" = ? AND valid_status = 1 LIMIT 1`, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("config %s not found or invalid", key)
		}
		return "", err
	}
	return value, nil
}

func (r *SQLiteRepository) ListConfigs(ctx context.Context) ([]model.SystemConfig, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, "key", value, valid_status FROM system_config ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.SystemConfig
	for rows.Next() {
		var item model.SystemConfig
		if err := rows.Scan(&item.ID, &item.Key, &item.Value, &item.ValidStatus); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) UpdateConfigs(ctx context.Context, configs []model.SystemConfig) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `UPDATE system_config SET value = ?, valid_status = ?, update_time = DATETIME(CURRENT_TIMESTAMP, 'localtime') WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, config := range configs {
		if _, err := stmt.ExecContext(ctx, config.Value, config.ValidStatus, config.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) SonarrTitleByCleanTitle(cleanTitle string) (*model.SonarrTitle, error) {
	rows, err := r.db.Query(`
		SELECT tvdb_id, sno, main_title, title, COALESCE(clean_title, ''), season_number, monitored
		FROM sonarr_title
		WHERE valid_status = 1 AND clean_title = ?
		ORDER BY monitored DESC, LENGTH(title) DESC
		LIMIT 1`, cleanTitle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var item model.SonarrTitle
	if err := rows.Scan(&item.TVDBID, &item.SNO, &item.MainTitle, &item.Title, &item.CleanTitle, &item.SeasonNumber, &item.Monitored); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *SQLiteRepository) RadarrTitleByCleanTitle(cleanTitle string) (*model.RadarrTitle, error) {
	rows, err := r.db.Query(`
		SELECT tmdb_id, sno, main_title, title, COALESCE(clean_title, ''), year, monitored
		FROM radarr_title
		WHERE valid_status = 1 AND clean_title = ?
		ORDER BY monitored DESC, LENGTH(title) DESC
		LIMIT 1`, cleanTitle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var item model.RadarrTitle
	if err := rows.Scan(&item.TMDBID, &item.SNO, &item.MainTitle, &item.Title, &item.CleanTitle, &item.Year, &item.Monitored); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *SQLiteRepository) SonarrTMDBTitles(tvdbID int) ([]model.TmdbTitle, error) {
	rows, err := r.db.Query(`
		SELECT tvdb_id, title
		FROM tmdb_title
		WHERE valid_status = 1 AND tvdb_id = ?
		GROUP BY title
		ORDER BY id ASC`, tvdbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.TmdbTitle
	for rows.Next() {
		var item model.TmdbTitle
		if err := rows.Scan(&item.TVDBID, &item.Title); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) SonarrCatalog() ([]model.SonarrTitle, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(main_title, ''), COALESCE(title, ''), COALESCE(clean_title, ''), season_number, monitored
		FROM
		(
			SELECT st.main_title, st.title, st.clean_title, st.season_number, st.monitored
			FROM sonarr_title st
			WHERE st.valid_status = 1
			GROUP BY st.clean_title
			UNION
			SELECT st.main_title, tt.title, NULL clean_title, -1 season_number, st.monitored
			FROM sonarr_title st LEFT JOIN tmdb_title tt ON st.tvdb_id = tt.tvdb_id
			WHERE st.sno = 0 AND st.valid_status = 1 AND tt.valid_status = 1
			GROUP BY tt.title
		)
		WHERE title IS NOT NULL
		ORDER BY monitored DESC, LENGTH(title) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.SonarrTitle
	for rows.Next() {
		var item model.SonarrTitle
		if err := rows.Scan(&item.MainTitle, &item.Title, &item.CleanTitle, &item.SeasonNumber, &item.Monitored); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) RadarrCatalog() ([]model.RadarrTitle, error) {
	rows, err := r.db.Query(`
		SELECT tmdb_id, sno, COALESCE(main_title, ''), COALESCE(title, ''), COALESCE(clean_title, ''), year, monitored
		FROM radarr_title
		WHERE valid_status = 1
		GROUP BY clean_title
		ORDER BY monitored DESC, LENGTH(title) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.RadarrTitle
	for rows.Next() {
		var item model.RadarrTitle
		if err := rows.Scan(&item.TMDBID, &item.SNO, &item.MainTitle, &item.Title, &item.CleanTitle, &item.Year, &item.Monitored); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) SonarrRules(token string) ([]model.Rule, error) {
	return r.rules("sonarr_rule", token)
}

func (r *SQLiteRepository) RadarrRules(token string) ([]model.Rule, error) {
	return r.rules("radarr_rule", token)
}

func (r *SQLiteRepository) rules(table, token string) ([]model.Rule, error) {
	rows, err := r.db.Query(fmt.Sprintf(`
		SELECT token, priority, regex, replacement, offset
		FROM %s
		WHERE valid_status = 1 AND token = ?
		ORDER BY priority ASC`, table), token)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Rule
	for rows.Next() {
		var item model.Rule
		if err := rows.Scan(&item.Token, &item.Priority, &item.Regex, &item.Replacement, &item.Offset); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) UpsertSonarrTitles(ctx context.Context, titles []model.SonarrTitle) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO sonarr_title
			(id, series_id, tvdb_id, sno, main_title, title, clean_title, season_number, monitored, valid_status, update_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, DATETIME(CURRENT_TIMESTAMP, 'localtime'))
			ON CONFLICT(id) DO UPDATE SET
			series_id = excluded.series_id,
			tvdb_id = excluded.tvdb_id,
			sno = excluded.sno,
			main_title = excluded.main_title,
			title = excluded.title,
			clean_title = excluded.clean_title,
			season_number = excluded.season_number,
			monitored = excluded.monitored,
			valid_status = 1,
			update_time = DATETIME(CURRENT_TIMESTAMP, 'localtime')`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, title := range titles {
			if _, err := stmt.ExecContext(ctx, title.ID, title.SeriesID, title.TVDBID, title.SNO, title.MainTitle, title.Title, title.CleanTitle, title.SeasonNumber, title.Monitored); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) NeedSyncTmdbIDs(ctx context.Context) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT st.tvdb_id
		FROM sonarr_title st LEFT JOIN tmdb_title tt ON st.tvdb_id = tt.tvdb_id
		WHERE st.sno = 0 AND st.valid_status = 1 AND tt.tvdb_id IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) InsertTmdbTitles(ctx context.Context, titles []model.TmdbTitle) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO tmdb_title (tvdb_id, tmdb_id, language, title, valid_status, update_time)
			VALUES (?, ?, ?, ?, 1, DATETIME(CURRENT_TIMESTAMP, 'localtime'))`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, title := range titles {
			if _, err := stmt.ExecContext(ctx, title.TVDBID, title.TMDBID, title.Language, title.Title); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) UpsertRadarrTitles(ctx context.Context, titles []model.RadarrTitle) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO radarr_title
			(id, movie_id, tmdb_id, sno, main_title, title, clean_title, year, monitored, valid_status, update_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, DATETIME(CURRENT_TIMESTAMP, 'localtime'))
			ON CONFLICT(id) DO UPDATE SET
			movie_id = excluded.movie_id,
			tmdb_id = excluded.tmdb_id,
			sno = excluded.sno,
			main_title = excluded.main_title,
			title = excluded.title,
			clean_title = excluded.clean_title,
			year = excluded.year,
			monitored = excluded.monitored,
			valid_status = 1,
			update_time = DATETIME(CURRENT_TIMESTAMP, 'localtime')`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, title := range titles {
			if _, err := stmt.ExecContext(ctx, title.ID, title.MovieID, title.TMDBID, title.SNO, title.MainTitle, title.Title, title.CleanTitle, title.Year, title.Monitored); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) UpsertRules(ctx context.Context, table string, rules []model.RuleRecord) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`INSERT INTO %s
			(id, token, priority, regex, replacement, offset, example, remark, author, valid_status, update_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, DATETIME(CURRENT_TIMESTAMP, 'localtime'))
			ON CONFLICT(id) DO UPDATE SET
			token = excluded.token,
			priority = excluded.priority,
			regex = excluded.regex,
			replacement = excluded.replacement,
			offset = excluded.offset,
			example = excluded.example,
			remark = excluded.remark,
			author = excluded.author,
			valid_status = excluded.valid_status,
			update_time = DATETIME(CURRENT_TIMESTAMP, 'localtime')`, table))
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, rule := range rules {
			if _, err := stmt.ExecContext(ctx, rule.ID, rule.Token, rule.Priority, rule.Regex, rule.Replacement, rule.Offset, rule.Example, rule.Remark, rule.Author, rule.ValidStatus); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) QueryRules(ctx context.Context, table string, query model.PageQuery) (model.PageResponse[model.RuleRecord], error) {
	where := []string{"1 = 1"}
	args := []any{}
	if query.Token != "" {
		where = append(where, "token LIKE ?")
		args = append(args, "%"+query.Token+"%")
	}
	if query.Remark != "" {
		where = append(where, "remark LIKE ?")
		args = append(args, "%"+query.Remark+"%")
	}
	return queryPage(ctx, r.db, table, "id, token, priority, regex, replacement, offset, example, COALESCE(remark, ''), COALESCE(author, ''), valid_status", strings.Join(where, " AND "), "update_time DESC", query, args, scanRuleRecord)
}

func (r *SQLiteRepository) SaveRule(ctx context.Context, table string, rule model.RuleRecord) error {
	if rule.ValidStatus == 0 {
		rule.ValidStatus = 1
	}
	return r.UpsertRules(ctx, table, []model.RuleRecord{rule})
}

func (r *SQLiteRepository) RemoveByTextIDs(ctx context.Context, table string, ids []string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ?", table))
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, id := range ids {
			if _, err := stmt.ExecContext(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) SetRuleValidStatus(ctx context.Context, table string, ids []string, status int) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, fmt.Sprintf("UPDATE %s SET valid_status = ?, update_time = DATETIME(CURRENT_TIMESTAMP, 'localtime') WHERE id = ?", table))
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, id := range ids {
			if _, err := stmt.ExecContext(ctx, status, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) ListRuleTokens(ctx context.Context, table string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("SELECT token FROM %s GROUP BY token ORDER BY token ASC", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, err
		}
		result = append(result, token)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) QuerySonarrTitles(ctx context.Context, query model.PageQuery) (model.PageResponse[model.SonarrTitle], error) {
	where := []string{"1 = 1"}
	args := []any{}
	if query.Title != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+query.Title+"%")
	}
	if query.TVDBID != 0 {
		where = append(where, "tvdb_id = ?")
		args = append(args, query.TVDBID)
	}
	return queryPage(ctx, r.db, "sonarr_title", "id, series_id, tvdb_id, sno, COALESCE(main_title, ''), COALESCE(title, ''), COALESCE(clean_title, ''), season_number, monitored, valid_status", strings.Join(where, " AND "), "update_time DESC", query, args, scanSonarrTitle)
}

func (r *SQLiteRepository) QueryRadarrTitles(ctx context.Context, query model.PageQuery) (model.PageResponse[model.RadarrTitle], error) {
	where := []string{"1 = 1"}
	args := []any{}
	if query.Title != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+query.Title+"%")
	}
	if query.TMDBID != 0 {
		where = append(where, "tmdb_id = ?")
		args = append(args, query.TMDBID)
	}
	return queryPage(ctx, r.db, "radarr_title", "id, movie_id, tmdb_id, sno, COALESCE(main_title, ''), COALESCE(title, ''), COALESCE(clean_title, ''), year, monitored, valid_status", strings.Join(where, " AND "), "update_time DESC", query, args, scanRadarrTitle)
}

func (r *SQLiteRepository) QueryTmdbTitles(ctx context.Context, query model.PageQuery) (model.PageResponse[model.TmdbTitle], error) {
	where := []string{"1 = 1"}
	args := []any{}
	if query.Title != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+query.Title+"%")
	}
	if query.TVDBID != 0 {
		where = append(where, "tvdb_id = ?")
		args = append(args, query.TVDBID)
	}
	return queryPage(ctx, r.db, "tmdb_title", "id, tvdb_id, tmdb_id, language, title, valid_status", strings.Join(where, " AND "), "update_time DESC", query, args, scanTmdbTitle)
}

func (r *SQLiteRepository) RemoveByIntIDs(ctx context.Context, table string, ids []int) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ?", table))
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, id := range ids {
			if _, err := stmt.ExecContext(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) SaveTmdbTitle(ctx context.Context, title model.TmdbTitle) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO tmdb_title (id, tvdb_id, tmdb_id, language, title, valid_status, update_time)
		VALUES (NULLIF(?, 0), ?, ?, ?, ?, COALESCE(NULLIF(?, 0), 1), DATETIME(CURRENT_TIMESTAMP, 'localtime'))
		ON CONFLICT(id) DO UPDATE SET
		tvdb_id = excluded.tvdb_id,
		tmdb_id = excluded.tmdb_id,
		language = excluded.language,
		title = excluded.title,
		valid_status = excluded.valid_status,
		update_time = DATETIME(CURRENT_TIMESTAMP, 'localtime')`,
		title.ID, title.TVDBID, title.TMDBID, title.Language, title.Title, title.ValidStatus)
	return err
}

func (r *SQLiteRepository) QueryExamples(ctx context.Context, table string, query model.PageQuery) (model.PageResponse[model.Example], error) {
	where := []string{"1 = 1"}
	args := []any{}
	if query.OriginalText != "" {
		where = append(where, "original_text LIKE ?")
		args = append(args, "%"+query.OriginalText+"%")
	}
	if query.ValidStatus >= 0 {
		where = append(where, "valid_status = ?")
		args = append(args, query.ValidStatus)
	}
	return queryPage(ctx, r.db, table, "hash, original_text, COALESCE(format_text, ''), valid_status", strings.Join(where, " AND "), "update_time DESC", query, args, scanExample)
}

func (r *SQLiteRepository) SaveExamples(ctx context.Context, table string, examples []model.Example) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`INSERT INTO %s (hash, original_text, format_text, valid_status, update_time)
			VALUES (?, ?, ?, COALESCE(NULLIF(?, 0), 1), DATETIME(CURRENT_TIMESTAMP, 'localtime'))
			ON CONFLICT(hash) DO UPDATE SET
			original_text = excluded.original_text,
			format_text = excluded.format_text,
			valid_status = excluded.valid_status,
			update_time = DATETIME(CURRENT_TIMESTAMP, 'localtime')`, table))
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, example := range examples {
			if _, err := stmt.ExecContext(ctx, example.Hash, example.OriginalText, example.FormatText, example.ValidStatus); err != nil {
				return err
			}
		}
		return nil
	})
}

func queryPage[T any](ctx context.Context, db *sql.DB, table, columns, where, orderBy string, query model.PageQuery, args []any, scan func(*sql.Rows) (T, error)) (model.PageResponse[T], error) {
	current := query.Current
	if current <= 0 {
		current = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", table, where)
	if err := db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return model.PageResponse[T]{}, err
	}
	offset := (current - 1) * pageSize
	listSQL := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT ? OFFSET ?", columns, table, where, orderBy)
	listArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return model.PageResponse[T]{}, err
	}
	defer rows.Close()
	var list []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return model.PageResponse[T]{}, err
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return model.PageResponse[T]{}, err
	}
	return model.PageResponse[T]{
		Current:  current,
		PageSize: pageSize,
		Total:    total,
		List:     list,
	}, nil
}

func scanRuleRecord(rows *sql.Rows) (model.RuleRecord, error) {
	var item model.RuleRecord
	err := rows.Scan(&item.ID, &item.Token, &item.Priority, &item.Regex, &item.Replacement, &item.Offset, &item.Example, &item.Remark, &item.Author, &item.ValidStatus)
	return item, err
}

func scanSonarrTitle(rows *sql.Rows) (model.SonarrTitle, error) {
	var item model.SonarrTitle
	err := rows.Scan(&item.ID, &item.SeriesID, &item.TVDBID, &item.SNO, &item.MainTitle, &item.Title, &item.CleanTitle, &item.SeasonNumber, &item.Monitored, &item.ValidStatus)
	return item, err
}

func scanRadarrTitle(rows *sql.Rows) (model.RadarrTitle, error) {
	var item model.RadarrTitle
	err := rows.Scan(&item.ID, &item.MovieID, &item.TMDBID, &item.SNO, &item.MainTitle, &item.Title, &item.CleanTitle, &item.Year, &item.Monitored, &item.ValidStatus)
	return item, err
}

func scanTmdbTitle(rows *sql.Rows) (model.TmdbTitle, error) {
	var item model.TmdbTitle
	err := rows.Scan(&item.ID, &item.TVDBID, &item.TMDBID, &item.Language, &item.Title, &item.ValidStatus)
	return item, err
}

func scanExample(rows *sql.Rows) (model.Example, error) {
	var item model.Example
	err := rows.Scan(&item.Hash, &item.OriginalText, &item.FormatText, &item.ValidStatus)
	return item, err
}

func (r *SQLiteRepository) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

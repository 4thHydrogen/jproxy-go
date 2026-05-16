package repository

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"jproxy/core-proxy/internal/model"
)

type SQLiteCLIRepository struct {
	sqlitePath string
	dbPath     string
}

func NewSQLiteCLIRepository(sqlitePath, dbPath string) *SQLiteCLIRepository {
	return &SQLiteCLIRepository{sqlitePath: sqlitePath, dbPath: dbPath}
}

func (r *SQLiteCLIRepository) ConfigValue(key string) (string, error) {
	rows, err := r.query(fmt.Sprintf(
		`SELECT value FROM system_config WHERE "key" = '%s' AND valid_status = 1 LIMIT 1;`,
		escape(key),
	))
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("config %s not found or invalid", key)
	}
	return toString(rows[0]["value"]), nil
}

func (r *SQLiteCLIRepository) SonarrTitleByCleanTitle(cleanTitle string) (*model.SonarrTitle, error) {
	rows, err := r.query(fmt.Sprintf(
		`SELECT tvdb_id, sno, main_title, title, clean_title, season_number, monitored
		FROM sonarr_title
		WHERE valid_status = 1 AND clean_title = '%s'
		ORDER BY monitored DESC, LENGTH(title) DESC
		LIMIT 1;`, escape(cleanTitle)))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	return &model.SonarrTitle{
		TVDBID:       toInt(row["tvdb_id"]),
		SNO:          toInt(row["sno"]),
		MainTitle:    toString(row["main_title"]),
		Title:        toString(row["title"]),
		CleanTitle:   toString(row["clean_title"]),
		SeasonNumber: toInt(row["season_number"]),
		Monitored:    toInt(row["monitored"]),
	}, nil
}

func (r *SQLiteCLIRepository) RadarrTitleByCleanTitle(cleanTitle string) (*model.RadarrTitle, error) {
	rows, err := r.query(fmt.Sprintf(
		`SELECT tmdb_id, sno, main_title, title, clean_title, year, monitored
		FROM radarr_title
		WHERE valid_status = 1 AND clean_title = '%s'
		ORDER BY monitored DESC, LENGTH(title) DESC
		LIMIT 1;`, escape(cleanTitle)))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	return &model.RadarrTitle{
		TMDBID:     toInt(row["tmdb_id"]),
		SNO:        toInt(row["sno"]),
		MainTitle:  toString(row["main_title"]),
		Title:      toString(row["title"]),
		CleanTitle: toString(row["clean_title"]),
		Year:       toInt(row["year"]),
		Monitored:  toInt(row["monitored"]),
	}, nil
}

func (r *SQLiteCLIRepository) SonarrTMDBTitles(tvdbID int) ([]model.TmdbTitle, error) {
	rows, err := r.query(fmt.Sprintf(
		`SELECT tvdb_id, title FROM tmdb_title
		WHERE valid_status = 1 AND tvdb_id = %d
		GROUP BY title
		ORDER BY id ASC;`, tvdbID))
	if err != nil {
		return nil, err
	}
	result := make([]model.TmdbTitle, 0, len(rows))
	for _, row := range rows {
		result = append(result, model.TmdbTitle{
			TVDBID: toInt(row["tvdb_id"]),
			Title:  toString(row["title"]),
		})
	}
	return result, nil
}

func (r *SQLiteCLIRepository) SonarrCatalog() ([]model.SonarrTitle, error) {
	rows, err := r.query(`
		SELECT main_title, title, clean_title, season_number, monitored
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
		ORDER BY monitored DESC, LENGTH(title) DESC;`)
	if err != nil {
		return nil, err
	}
	result := make([]model.SonarrTitle, 0, len(rows))
	for _, row := range rows {
		result = append(result, model.SonarrTitle{
			MainTitle:    toString(row["main_title"]),
			Title:        toString(row["title"]),
			CleanTitle:   toString(row["clean_title"]),
			SeasonNumber: toInt(row["season_number"]),
			Monitored:    toInt(row["monitored"]),
		})
	}
	return result, nil
}

func (r *SQLiteCLIRepository) RadarrCatalog() ([]model.RadarrTitle, error) {
	rows, err := r.query(`
		SELECT tmdb_id, sno, main_title, title, clean_title, year, monitored
		FROM radarr_title
		WHERE valid_status = 1
		GROUP BY clean_title
		ORDER BY monitored DESC, LENGTH(title) DESC;`)
	if err != nil {
		return nil, err
	}
	result := make([]model.RadarrTitle, 0, len(rows))
	for _, row := range rows {
		result = append(result, model.RadarrTitle{
			TMDBID:     toInt(row["tmdb_id"]),
			SNO:        toInt(row["sno"]),
			MainTitle:  toString(row["main_title"]),
			Title:      toString(row["title"]),
			CleanTitle: toString(row["clean_title"]),
			Year:       toInt(row["year"]),
			Monitored:  toInt(row["monitored"]),
		})
	}
	return result, nil
}

func (r *SQLiteCLIRepository) SonarrRules(token string) ([]model.Rule, error) {
	return r.rules("sonarr_rule", token)
}

func (r *SQLiteCLIRepository) RadarrRules(token string) ([]model.Rule, error) {
	return r.rules("radarr_rule", token)
}

func (r *SQLiteCLIRepository) rules(table, token string) ([]model.Rule, error) {
	rows, err := r.query(fmt.Sprintf(
		`SELECT token, priority, regex, replacement, offset
		FROM %s
		WHERE valid_status = 1 AND token = '%s'
		ORDER BY priority ASC;`, table, escape(token)))
	if err != nil {
		return nil, err
	}
	result := make([]model.Rule, 0, len(rows))
	for _, row := range rows {
		result = append(result, model.Rule{
			Token:       toString(row["token"]),
			Priority:    toInt(row["priority"]),
			Regex:       toString(row["regex"]),
			Replacement: toString(row["replacement"]),
			Offset:      toInt(row["offset"]),
		})
	}
	return result, nil
}

func (r *SQLiteCLIRepository) query(sql string) ([]map[string]any, error) {
	cmd := exec.Command(r.sqlitePath, "-json", r.dbPath, sql)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sqlite3 query failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, fmt.Errorf("parse sqlite json: %w", err)
	}
	return rows, nil
}

func escape(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}

func toInt(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case float64:
		return int(typed)
	case string:
		number, _ := strconv.Atoi(typed)
		return number
	default:
		number, _ := strconv.Atoi(fmt.Sprint(value))
		return number
	}
}

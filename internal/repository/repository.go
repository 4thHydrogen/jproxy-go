package repository

import "jproxy/core-proxy/internal/model"

type Repository interface {
	ConfigValue(key string) (string, error)
	SonarrTitleByCleanTitle(cleanTitle string) (*model.SonarrTitle, error)
	RadarrTitleByCleanTitle(cleanTitle string) (*model.RadarrTitle, error)
	SonarrTMDBTitles(tvdbID int) ([]model.TmdbTitle, error)
	SonarrCatalog() ([]model.SonarrTitle, error)
	RadarrCatalog() ([]model.RadarrTitle, error)
	SonarrRules(token string) ([]model.Rule, error)
	RadarrRules(token string) ([]model.Rule, error)
}

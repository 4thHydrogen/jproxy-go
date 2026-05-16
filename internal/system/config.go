package system

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"jproxy/core-proxy/internal/model"
	"jproxy/core-proxy/internal/repository"
)

type ConfigService struct {
	repo   *repository.SQLiteRepository
	client *http.Client
}

func NewConfigService(repo *repository.SQLiteRepository, client *http.Client) *ConfigService {
	return &ConfigService{repo: repo, client: client}
}

func (s *ConfigService) List(ctx context.Context) ([]model.SystemConfig, error) {
	return s.repo.ListConfigs(ctx)
}

func (s *ConfigService) Update(ctx context.Context, configs []model.SystemConfig) error {
	for index := range configs {
		configs[index].ValidStatus = s.validate(ctx, configs[index], configs)
		if configs[index].Key == "transmissionUrl" {
			configs[index].Value = normalizeTransmissionURL(configs[index].Value)
		}
	}
	return s.repo.UpdateConfigs(ctx, configs)
}

func (s *ConfigService) validate(ctx context.Context, config model.SystemConfig, all []model.SystemConfig) int {
	switch config.Key {
	case "sonarrIndexerFormat":
		if strings.Contains(config.Value, "{title}") && strings.Contains(config.Value, "{season}") && strings.Contains(config.Value, "{episode}") {
			return 1
		}
		return 0
	case "radarrIndexerFormat":
		if strings.Contains(config.Value, "{title}") && strings.Contains(config.Value, "{year}") {
			return 1
		}
		return 0
	case "cleanTitleRegex":
		_, err := regexp.Compile(config.Value)
		if err == nil {
			return 1
		}
		return 0
	case "jackettUrl", "prowlarrUrl", "qbittorrentUrl", "transmissionUrl":
		if validHTTPURL(config.Value) {
			return 1
		}
		return 0
	case "sonarrUrl":
		return checkAPIHealth(ctx, s.client, config.Value, findConfig(all, "sonarrApikey"))
	case "sonarrApikey":
		return checkAPIHealth(ctx, s.client, findConfig(all, "sonarrUrl"), config.Value)
	case "radarrUrl":
		return checkAPIHealth(ctx, s.client, config.Value, findConfig(all, "radarrApikey"))
	case "radarrApikey":
		return checkAPIHealth(ctx, s.client, findConfig(all, "radarrUrl"), config.Value)
	case "tmdbUrl":
		return checkTMDB(ctx, s.client, config.Value, findConfig(all, "tmdbApikey"))
	case "tmdbApikey":
		return checkTMDB(ctx, s.client, findConfig(all, "tmdbUrl"), config.Value)
	default:
		return config.ValidStatus
	}
}

func validHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func normalizeTransmissionURL(raw string) string {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return value
	}
	if strings.HasSuffix(value, "/transmission/rpc") {
		return value
	}
	value = strings.ReplaceAll(value, "/transmission/web", "")
	return value + "/transmission/rpc"
}

func findConfig(configs []model.SystemConfig, key string) string {
	for _, config := range configs {
		if config.Key == key {
			return config.Value
		}
	}
	return ""
}

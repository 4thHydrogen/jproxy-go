package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"jproxy/core-proxy/internal/model"
)

type fixtureRepo struct {
	config        map[string]string
	sonarrLookup  map[string]model.SonarrTitle
	radarrLookup  map[string]model.RadarrTitle
	tmdbByTVDB    map[int][]model.TmdbTitle
	sonarrCatalog []model.SonarrTitle
	radarrCatalog []model.RadarrTitle
	sonarrRules   map[string][]model.Rule
	radarrRules   map[string][]model.Rule
}

func (f fixtureRepo) ConfigValue(key string) (string, error) { return f.config[key], nil }
func (f fixtureRepo) SonarrTitleByCleanTitle(cleanTitle string) (*model.SonarrTitle, error) {
	value, ok := f.sonarrLookup[cleanTitle]
	if !ok {
		return nil, nil
	}
	return &value, nil
}
func (f fixtureRepo) RadarrTitleByCleanTitle(cleanTitle string) (*model.RadarrTitle, error) {
	value, ok := f.radarrLookup[cleanTitle]
	if !ok {
		return nil, nil
	}
	return &value, nil
}
func (f fixtureRepo) SonarrTMDBTitles(tvdbID int) ([]model.TmdbTitle, error) {
	return f.tmdbByTVDB[tvdbID], nil
}
func (f fixtureRepo) SonarrCatalog() ([]model.SonarrTitle, error) { return f.sonarrCatalog, nil }
func (f fixtureRepo) RadarrCatalog() ([]model.RadarrTitle, error) { return f.radarrCatalog, nil }
func (f fixtureRepo) SonarrRules(token string) ([]model.Rule, error) {
	return f.sonarrRules[token], nil
}
func (f fixtureRepo) RadarrRules(token string) ([]model.Rule, error) {
	return f.radarrRules[token], nil
}

func TestSonarrSearchTitlesAddsTMDBFallbacks(t *testing.T) {
	service := mustService(t, fixtureRepo{
		config: map[string]string{"cleanTitleRegex": `[@"!?` + "`" + `_: \[\]\-\.']`},
		sonarrLookup: map[string]model.SonarrTitle{
			"three body problem": {TVDBID: 1, SNO: 0, MainTitle: "三体", Title: "三体"},
		},
		tmdbByTVDB: map[int][]model.TmdbTitle{
			1: {{TVDBID: 1, Title: "Three Body Problem"}, {TVDBID: 1, Title: "San Ti"}},
		},
	})
	titles, err := service.sonarrSearchTitles("Three Body Problem")
	if err != nil {
		t.Fatalf("search titles: %v", err)
	}
	want := []string{"Three Body Problem", "Three Body Problem", "San Ti", "Three Body Problem"}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("titles mismatch: got %v want %v", titles, want)
	}
}

func TestRadarrSearchTitlesAddsMainTitleFallback(t *testing.T) {
	service := mustService(t, fixtureRepo{
		config: map[string]string{"cleanTitleRegex": `[@"!?` + "`" + `_: \[\]\-\.']`},
		radarrLookup: map[string]model.RadarrTitle{
			"spirited away": {TMDBID: 2, SNO: 1, MainTitle: "千与千寻", Title: "Spirited Away", Year: 2001},
		},
	})
	titles, err := service.radarrSearchTitles("Spirited Away 2001")
	if err != nil {
		t.Fatalf("search titles: %v", err)
	}
	want := []string{"Spirited Away 2001", "Spirited Away", "千与千寻", "千与千寻"}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("titles mismatch: got %v want %v", titles, want)
	}
}

func TestFormatSonarrXMLRewritesTitles(t *testing.T) {
	service := mustService(t, fixtureRepo{
		config: map[string]string{
			"cleanTitleRegex":     `[@"!?` + "`" + `_: \[\]\-\.']`,
			"sonarrIndexerFormat": "{title} {season}{episode} {language}",
		},
		sonarrCatalog: []model.SonarrTitle{
			{MainTitle: "三体", Title: "Three Body Problem", CleanTitle: "three body problem", SeasonNumber: 1},
		},
		sonarrRules: map[string][]model.Rule{
			"title":    {{Token: "title", Regex: `.*{cleanTitle}.*`, Replacement: ""}},
			"episode":  {{Token: "episode", Regex: `(?i).*?(E\d+).*`, Replacement: "$1"}},
			"language": {{Token: "language", Regex: `(?i).*\b(CHS)\b.*`, Replacement: "$1"}},
			"season":   {},
		},
	})
	xml := `<?xml version="1.0"?><rss><channel><item><title>Three Body Problem S01E02 CHS</title></item></channel></rss>`
	got, err := service.formatSonarrXML(xml)
	if err != nil {
		t.Fatalf("format xml: %v", err)
	}
	if !strings.Contains(got, "<title>三体 E02 CHS</title>") {
		t.Fatalf("unexpected formatted xml: %s", got)
	}
}

func TestHandleMergesMultipleQueriesAndFormats(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		switch q {
		case "Three Body Problem":
			w.Write([]byte(`<?xml version="1.0"?><rss><channel><item><title>Three Body Problem S01E02 CHS</title></item></channel></rss>`))
		case "San Ti":
			w.Write([]byte(`<?xml version="1.0"?><rss><channel><item><title>San Ti S01E02 CHS</title></item></channel></rss>`))
		default:
			w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
		}
	}))
	defer upstream.Close()

	service := mustService(t, fixtureRepo{
		config: map[string]string{
			"cleanTitleRegex":     `[@"!?` + "`" + `_: \[\]\-\.']`,
			"sonarrIndexerFormat": "{title} {season}{episode} {language}",
			"jackettUrl":          upstream.URL,
		},
		sonarrLookup: map[string]model.SonarrTitle{
			"three body problem": {TVDBID: 1, SNO: 0, MainTitle: "三体", Title: "Three Body Problem"},
		},
		tmdbByTVDB: map[int][]model.TmdbTitle{
			1: {{TVDBID: 1, Title: "San Ti"}},
		},
		sonarrCatalog: []model.SonarrTitle{
			{MainTitle: "三体", Title: "Three Body Problem", CleanTitle: "three body problem", SeasonNumber: 1},
			{MainTitle: "三体", Title: "San Ti", CleanTitle: "san ti", SeasonNumber: 1},
		},
		sonarrRules: map[string][]model.Rule{
			"title":    {{Token: "title", Regex: `.*{cleanTitle}.*`, Replacement: ""}},
			"episode":  {{Token: "episode", Regex: `(?i).*?(E\d+).*`, Replacement: "$1"}},
			"language": {{Token: "language", Regex: `(?i).*\b(CHS)\b.*`, Replacement: "$1"}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/sonarr/jackett/api/v2.0/indexers/all/results/torznab/?q=Three+Body+Problem&t=search&offset=0&limit=2", nil)
	got, err := service.Handle(req, model.MediaSonarr, model.IndexerJackett)
	if err != nil {
		t.Fatalf("handle request: %v", err)
	}
	if strings.Count(got, "<item>") != 2 {
		t.Fatalf("expected merged items, got %s", got)
	}
	if !strings.Contains(got, "三体 E02 CHS") {
		t.Fatalf("expected formatted title, got %s", got)
	}
}

func TestFormatRadarrXMLRewritesTitleAndYear(t *testing.T) {
	service := mustService(t, fixtureRepo{
		config: map[string]string{
			"cleanTitleRegex":     `[@"!?` + "`" + `_: \[\]\-\.']`,
			"radarrIndexerFormat": "{title} {year} {language}",
		},
		radarrCatalog: []model.RadarrTitle{
			{MainTitle: "千与千寻", Title: "Spirited Away", CleanTitle: "spirited away", Year: 2001},
		},
		radarrRules: map[string][]model.Rule{
			"title":    {{Token: "title", Regex: `.*{cleanTitle}.*`, Replacement: ""}},
			"year":     {{Token: "year", Regex: `.*\b(2001)\b.*`, Replacement: "$1"}},
			"language": {{Token: "language", Regex: `(?i).*\b(CHS)\b.*`, Replacement: "$1"}},
		},
	})
	xml := `<?xml version="1.0"?><rss><channel><item><title>Spirited Away 2001 CHS</title></item></channel></rss>`
	got, err := service.formatRadarrXML(xml)
	if err != nil {
		t.Fatalf("format xml: %v", err)
	}
	if !strings.Contains(got, "<title>千与千寻 2001 CHS</title>") {
		t.Fatalf("unexpected formatted xml: %s", got)
	}
}

func TestRouteHandlerUsesCompatiblePrefixes(t *testing.T) {
	service := mustService(t, fixtureRepo{
		config: map[string]string{
			"jackettUrl":          "http://127.0.0.1:65535",
			"cleanTitleRegex":     ".",
			"sonarrIndexerFormat": "{title}",
		},
	})
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/sonarr/jackett/api/v2.0/indexers/all/results/torznab/?t=search", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code == http.StatusNotFound {
		t.Fatalf("route not registered")
	}
}

func TestGenerateOffsetKeyRemovesOffsetAndApiKey(t *testing.T) {
	key := generateOffsetKey("/sonarr/jackett/api", "q=abc&apikey=secret&offset=100&limit=50")
	if strings.Contains(key, "apikey") || strings.Contains(key, "offset=") {
		t.Fatalf("sensitive fields were not removed: %s", key)
	}
}

func mustService(t *testing.T, repo fixtureRepo) *Service {
	t.Helper()
	service, err := NewService(repo, "6")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.client = http.DefaultClient
	return service
}

func TestParseRequestCompatibility(t *testing.T) {
	values := url.Values{
		"q":      {"foo"},
		"t":      {"search"},
		"season": {"1"},
		"ep":     {"2"},
		"offset": {"10"},
		"limit":  {"50"},
	}
	got := parseRequest(values)
	if got.SearchKey != "foo" || got.SearchType != "search" || got.SeasonNumber != "1" || got.EpisodeNumber != "2" || got.Offset != 10 || got.Limit != 50 {
		t.Fatalf("unexpected request parse: %+v", got)
	}
}

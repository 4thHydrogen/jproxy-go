package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jproxy/core-proxy/internal/model"
)

type goldenCase struct {
	Name          string                       `json:"name"`
	Kind          string                       `json:"kind"`
	Indexer       string                       `json:"indexer"`
	Path          string                       `json:"path"`
	Query         string                       `json:"query"`
	UpstreamByQ   map[string]string            `json:"upstream_by_q"`
	Config        map[string]string            `json:"config"`
	SonarrLookup  map[string]model.SonarrTitle `json:"sonarr_lookup"`
	RadarrLookup  map[string]model.RadarrTitle `json:"radarr_lookup"`
	TmdbByTVDB    map[string][]model.TmdbTitle `json:"tmdb_by_tvdb"`
	SonarrCatalog []model.SonarrTitle          `json:"sonarr_catalog"`
	RadarrCatalog []model.RadarrTitle          `json:"radarr_catalog"`
	SonarrRules   map[string][]model.Rule      `json:"sonarr_rules"`
	RadarrRules   map[string][]model.Rule      `json:"radarr_rules"`
	ExpectedXML   string                       `json:"expected_xml"`
}

func TestGoldenCases(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "golden", "*.json"))
	if err != nil {
		t.Fatalf("glob golden cases: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var tc goldenCase
		if err := json.Unmarshal(data, &tc); err != nil {
			t.Fatalf("unmarshal %s: %v", file, err)
		}
		t.Run(tc.Name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				queryTitle := r.URL.Query().Get("q")
				if payload, ok := tc.UpstreamByQ[queryTitle]; ok {
					_, _ = w.Write([]byte(payload))
					return
				}
				_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
			}))
			defer upstream.Close()

			config := map[string]string{}
			for key, value := range tc.Config {
				config[key] = value
			}
			switch tc.Indexer {
			case "jackett":
				config["jackettUrl"] = upstream.URL
			case "prowlarr":
				config["prowlarrUrl"] = upstream.URL
			}

			tmdbByTVDB := map[int][]model.TmdbTitle{}
			for key, value := range tc.TmdbByTVDB {
				switch key {
				case "1":
					tmdbByTVDB[1] = value
				default:
					// golden fixtures can add more ids later
				}
			}

			service := mustService(t, fixtureRepo{
				config:        config,
				sonarrLookup:  tc.SonarrLookup,
				radarrLookup:  tc.RadarrLookup,
				tmdbByTVDB:    tmdbByTVDB,
				sonarrCatalog: tc.SonarrCatalog,
				radarrCatalog: tc.RadarrCatalog,
				sonarrRules:   tc.SonarrRules,
				radarrRules:   tc.RadarrRules,
			})

			req := httptest.NewRequest(http.MethodGet, tc.Path+"?"+tc.Query, nil)
			got, err := service.Handle(req, model.MediaKind(tc.Kind), model.IndexerKind(tc.Indexer))
			if err != nil {
				t.Fatalf("handle request: %v", err)
			}
			if normalizeXML(got) != normalizeXML(tc.ExpectedXML) {
				t.Fatalf("golden mismatch\nwant: %s\n got: %s", tc.ExpectedXML, got)
			}
		})
	}
}

func normalizeXML(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"jproxy/core-proxy/internal/model"
	"jproxy/core-proxy/internal/repository"
	"jproxy/core-proxy/internal/util"
)

const maxSyncResponseLength = 16 * 1024 * 1024

var errSyncResponseTooLarge = errors.New("sync response too large")

type SyncService struct {
	repo        *repository.SQLiteRepository
	client      *http.Client
	ruleBase    string
	runMu       sync.Mutex
	statusMu    sync.RWMutex
	statusByJob map[string]model.SyncJobStatus
}

func NewSyncService(repo *repository.SQLiteRepository, client *http.Client) *SyncService {
	if client == nil {
		client = defaultHTTPClient()
	}
	return &SyncService{
		repo:        repo,
		client:      client,
		statusByJob: map[string]model.SyncJobStatus{},
	}
}

func (s *SyncService) Status() []model.SyncJobStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	result := make([]model.SyncJobStatus, 0, len(s.statusByJob))
	for _, status := range s.statusByJob {
		result = append(result, status)
	}
	return result
}

func (s *SyncService) Run(ctx context.Context, job string) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	switch job {
	case "sonarr-title":
		return s.track(job, s.SyncSonarrTitles)(ctx)
	case "sonarr-rule":
		return s.track(job, s.SyncSonarrRules)(ctx)
	case "radarr-title":
		return s.track(job, s.SyncRadarrTitles)(ctx)
	case "radarr-rule":
		return s.track(job, s.SyncRadarrRules)(ctx)
	case "all":
		return s.track(job, func(ctx context.Context) error {
			if err := s.SyncSonarrTitles(ctx); err != nil {
				return err
			}
			if err := s.SyncSonarrRules(ctx); err != nil {
				return err
			}
			if err := s.SyncRadarrTitles(ctx); err != nil {
				return err
			}
			return s.SyncRadarrRules(ctx)
		})(ctx)
	default:
		return fmt.Errorf("unknown sync job: %s", job)
	}
}

func (s *SyncService) track(name string, fn func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		s.updateStatus(name, func(status model.SyncJobStatus) model.SyncJobStatus {
			status.Name = name
			status.Running = true
			status.LastStartedAt = time.Now().Format(time.RFC3339)
			status.LastError = ""
			return status
		})
		err := fn(ctx)
		s.updateStatus(name, func(status model.SyncJobStatus) model.SyncJobStatus {
			status.Name = name
			status.Running = false
			status.LastFinishedAt = time.Now().Format(time.RFC3339)
			if err != nil {
				status.LastError = err.Error()
			}
			return status
		})
		return err
	}
}

func (s *SyncService) updateStatus(name string, mutate func(model.SyncJobStatus) model.SyncJobStatus) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	status := s.statusByJob[name]
	s.statusByJob[name] = mutate(status)
}

func (s *SyncService) SyncSonarrTitles(ctx context.Context) error {
	sonarrURL, err := s.repo.ConfigValue("sonarrUrl")
	if err != nil {
		return err
	}
	apiKey, err := s.repo.ConfigValue("sonarrApikey")
	if err != nil {
		return err
	}
	cleanRegex, err := s.repo.ConfigValue("cleanTitleRegex")
	if err != nil {
		return err
	}
	body, err := s.get(ctx, strings.TrimRight(sonarrURL, "/")+"/api/v3/series?apikey="+apiKey)
	if err != nil {
		return err
	}
	var payload []struct {
		ID              int    `json:"id"`
		TVDBID          int    `json:"tvdbId"`
		Title           string `json:"title"`
		TitleSlug       string `json:"titleSlug"`
		Monitored       bool   `json:"monitored"`
		AlternateTitles []struct {
			Title             string `json:"title"`
			SceneSeasonNumber int    `json:"sceneSeasonNumber"`
		} `json:"alternateTitles"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	var titles []model.SonarrTitle
	for _, series := range payload {
		sno := 0
		main := model.SonarrTitle{
			ID:           generateTitleID(series.TVDBID, sno),
			SeriesID:     series.ID,
			TVDBID:       series.TVDBID,
			SNO:          sno,
			MainTitle:    series.Title,
			Title:        series.Title,
			CleanTitle:   util.CleanTitle(series.Title, cleanRegex),
			SeasonNumber: -1,
			Monitored:    boolCode(series.Monitored),
		}
		titles = append(titles, main)
		sno++
		slug := main
		slug.ID = generateTitleID(series.TVDBID, sno)
		slug.SNO = sno
		slug.CleanTitle = util.CleanTitle(series.TitleSlug, cleanRegex)
		titles = append(titles, slug)
		sno++
		for _, alt := range series.AlternateTitles {
			item := model.SonarrTitle{
				ID:           generateTitleID(series.TVDBID, sno),
				SeriesID:     series.ID,
				TVDBID:       series.TVDBID,
				SNO:          sno,
				MainTitle:    series.Title,
				Title:        alt.Title,
				CleanTitle:   util.CleanTitle(alt.Title, cleanRegex),
				SeasonNumber: alt.SceneSeasonNumber,
				Monitored:    boolCode(series.Monitored),
			}
			titles = append(titles, item)
			sno++
		}
	}
	if err := s.repo.UpsertSonarrTitles(ctx, titles); err != nil {
		return err
	}
	ids, err := s.repo.NeedSyncTmdbIDs(ctx)
	if err != nil {
		return err
	}
	return s.SyncTmdbTitles(ctx, ids)
}

func (s *SyncService) SyncTmdbTitles(ctx context.Context, tvdbIDs []int) error {
	if len(tvdbIDs) == 0 {
		return nil
	}
	tmdbURL, err := s.repo.ConfigValue("tmdbUrl")
	if err != nil {
		return err
	}
	apiKey, err := s.repo.ConfigValue("tmdbApikey")
	if err != nil {
		return err
	}
	lang1, err := s.repo.ConfigValue("sonarrLanguage1")
	if err != nil {
		return err
	}
	lang2, err := s.repo.ConfigValue("sonarrLanguage2")
	if err != nil {
		return err
	}
	var result []model.TmdbTitle
	for _, tvdbID := range tvdbIDs {
		for _, language := range []string{lang1, lang2} {
			findURL := fmt.Sprintf("%s/3/find/%d?api_key=%s&language=%s&external_source=tvdb_id", strings.TrimRight(tmdbURL, "/"), tvdbID, apiKey, language)
			body, err := s.get(ctx, findURL)
			if err != nil {
				return err
			}
			var payload struct {
				TVResults []struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"tv_results"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				return err
			}
			if len(payload.TVResults) == 0 {
				continue
			}
			result = append(result, model.TmdbTitle{
				TVDBID:   tvdbID,
				TMDBID:   payload.TVResults[0].ID,
				Language: language,
				Title:    payload.TVResults[0].Name,
			})
		}
	}
	return s.repo.InsertTmdbTitles(ctx, result)
}

func (s *SyncService) SyncRadarrTitles(ctx context.Context) error {
	radarrURL, err := s.repo.ConfigValue("radarrUrl")
	if err != nil {
		return err
	}
	apiKey, err := s.repo.ConfigValue("radarrApikey")
	if err != nil {
		return err
	}
	cleanRegex, err := s.repo.ConfigValue("cleanTitleRegex")
	if err != nil {
		return err
	}
	body, err := s.get(ctx, strings.TrimRight(radarrURL, "/")+"/api/v3/movie?apikey="+apiKey)
	if err != nil {
		return err
	}
	var payload []struct {
		ID              int    `json:"id"`
		TMDBID          int    `json:"tmdbId"`
		Year            int    `json:"year"`
		Title           string `json:"title"`
		Path            string `json:"path"`
		CleanTitle      string `json:"cleanTitle"`
		OriginalTitle   string `json:"originalTitle"`
		Monitored       bool   `json:"monitored"`
		AlternateTitles []struct {
			Title string `json:"title"`
		} `json:"alternateTitles"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	var titles []model.RadarrTitle
	for _, movie := range payload {
		sno := 0
		main := model.RadarrTitle{
			ID:         generateTitleID(movie.TMDBID, sno),
			MovieID:    movie.ID,
			TMDBID:     movie.TMDBID,
			SNO:        sno,
			MainTitle:  movie.Title,
			Title:      movie.Title,
			CleanTitle: util.CleanTitle(movie.Title, cleanRegex),
			Year:       movie.Year,
			Monitored:  boolCode(movie.Monitored),
		}
		titles = append(titles, main)
		sno++
		pathTitle := titleFromPath(movie.Path)
		english := main
		english.ID = generateTitleID(movie.TMDBID, sno)
		english.SNO = sno
		english.Title = pathTitle
		english.CleanTitle = util.CleanTitle(pathTitle, cleanRegex)
		titles = append(titles, english)
		sno++
		clean := main
		clean.ID = generateTitleID(movie.TMDBID, sno)
		clean.SNO = sno
		clean.Title = pathTitle
		clean.CleanTitle = movie.CleanTitle
		titles = append(titles, clean)
		sno++
		original := main
		original.ID = generateTitleID(movie.TMDBID, sno)
		original.SNO = sno
		original.Title = movie.OriginalTitle
		original.CleanTitle = util.CleanTitle(movie.OriginalTitle, cleanRegex)
		titles = append(titles, original)
		sno++
		for _, alt := range movie.AlternateTitles {
			item := model.RadarrTitle{
				ID:         generateTitleID(movie.TMDBID, sno),
				MovieID:    movie.ID,
				TMDBID:     movie.TMDBID,
				SNO:        sno,
				MainTitle:  movie.Title,
				Title:      alt.Title,
				CleanTitle: util.CleanTitle(alt.Title, cleanRegex),
				Year:       movie.Year,
				Monitored:  boolCode(movie.Monitored),
			}
			titles = append(titles, item)
			sno++
		}
	}
	return s.repo.UpsertRadarrTitles(ctx, titles)
}

func (s *SyncService) SyncSonarrRules(ctx context.Context) error {
	return s.syncRules(ctx, "sonarr_rule", "sonarr")
}

func (s *SyncService) SyncRadarrRules(ctx context.Context) error {
	return s.syncRules(ctx, "radarr_rule", "radarr")
}

func (s *SyncService) syncRules(ctx context.Context, table, media string) error {
	base, err := s.ruleLocation(ctx)
	if err != nil {
		return err
	}
	authors, err := s.ruleAuthors(ctx, base)
	if err != nil {
		return err
	}
	successes := 0
	var failures []error
	for _, author := range authors {
		body, err := s.get(ctx, fmt.Sprintf("%s/%s@%s.json", strings.TrimRight(base, "/"), media, strings.TrimSpace(author)))
		if err != nil {
			failures = append(failures, err)
			continue
		}
		var rules []model.RuleRecord
		if err := json.Unmarshal(body, &rules); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := validateRuleRegexes(rules); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := s.repo.UpsertRules(ctx, table, rules); err != nil {
			failures = append(failures, err)
			continue
		}
		successes++
	}
	if successes == 0 && len(failures) > 0 {
		return errors.Join(failures...)
	}
	return nil
}

func (s *SyncService) ruleLocation(ctx context.Context) (string, error) {
	if s.ruleBase != "" {
		return s.ruleBase, nil
	}
	return "https://raw.githubusercontent.com/LuckyPuppy514/jproxy/main/src/main/resources/rule", nil
}

func (s *SyncService) ruleAuthors(ctx context.Context, base string) ([]string, error) {
	authors, err := s.repo.ConfigValue("ruleSyncAuthors")
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(authors, "ALL") {
		body, err := s.get(ctx, strings.TrimRight(base, "/")+"/author.json")
		if err != nil {
			return nil, err
		}
		var values []string
		if err := json.Unmarshal(body, &values); err != nil {
			return nil, err
		}
		return values, nil
	}
	var result []string
	for _, author := range strings.Split(authors, ",") {
		trimmed := strings.TrimSpace(author)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}

func (s *SyncService) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request %s failed: %s", rawURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSyncResponseLength+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSyncResponseLength {
		return nil, errSyncResponseTooLarge
	}
	return body, nil
}

func validateRuleRegexes(rules []model.RuleRecord) error {
	for _, rule := range rules {
		if _, err := regexp.Compile(rule.Regex); err != nil {
			return fmt.Errorf("invalid regex for rule %s: %w", rule.ID, err)
		}
	}
	return nil
}

func generateTitleID(base, sno int) int {
	value := fmt.Sprintf("%d%d", base, sno)
	result := 0
	fmt.Sscanf(value, "%d", &result)
	return result
}

func boolCode(flag bool) int {
	if flag {
		return 1
	}
	return 0
}

func titleFromPath(path string) string {
	index := strings.LastIndex(path, "/")
	if index == -1 {
		index = 0
	} else {
		index++
	}
	value := path[index:]
	return regexp.MustCompile(` \(\d{4}\)$`).ReplaceAllString(value, "")
}

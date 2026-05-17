package app

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"jproxy/core-proxy/internal/model"
	"jproxy/core-proxy/internal/repository"
	"jproxy/core-proxy/internal/util"
)

const (
	maxResponseLength     = 4 * 1024 * 1024
	maxOffsetCacheEntries = 1024
)

type Service struct {
	repo          repository.Repository
	client        *http.Client
	minCount      int
	offsetMu      sync.Mutex
	offsetCache   map[string][]int
	offsetOrder   []string
	sonarrCatalog []model.SonarrTitle
	radarrCatalog []model.RadarrTitle
	catalogMu     sync.RWMutex
}

func NewService(repo repository.Repository, minCountRaw string) (*Service, error) {
	minCount, err := strconv.Atoi(strings.TrimSpace(minCountRaw))
	if err != nil {
		return nil, fmt.Errorf("invalid min count: %w", err)
	}
	return &Service{
		repo:        repo,
		client:      &http.Client{Timeout: 60 * time.Second},
		minCount:    minCount,
		offsetCache: map[string][]int{},
	}, nil
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	for _, route := range []string{
		"/sonarr/jackett/",
		"/sonarr/prowlarr/",
		"/radarr/jackett/",
		"/radarr/prowlarr/",
	} {
		mux.HandleFunc(route, s.routeHandler)
	}
}

func (s *Service) ResetCaches() {
	s.offsetMu.Lock()
	s.offsetCache = map[string][]int{}
	s.offsetOrder = nil
	s.offsetMu.Unlock()

	s.catalogMu.Lock()
	s.sonarrCatalog = nil
	s.radarrCatalog = nil
	s.catalogMu.Unlock()
}

func (s *Service) routeHandler(w http.ResponseWriter, r *http.Request) {
	kind, indexer, err := detectRoute(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	payload, err := s.Handle(r, kind, indexer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(payload))
}

func (s *Service) Handle(r *http.Request, kind model.MediaKind, indexer model.IndexerKind) (string, error) {
	req := parseRequest(r.URL.Query())
	searchKey := strings.TrimSpace(req.SearchKey)
	var xml string

	if searchKey != "" {
		var titles []string
		var err error
		switch kind {
		case model.MediaSonarr:
			titles, err = s.sonarrSearchTitles(util.RemoveEpisode(searchKey))
		case model.MediaRadarr:
			titles, err = s.radarrSearchTitles(searchKey)
		default:
			err = errors.New("unknown media kind")
		}
		if err != nil {
			return "", err
		}
		offsetKey := generateOffsetKey(r.URL.Path, r.URL.RawQuery)
		offsets := s.getOffsetList(offsetKey, len(titles))
		offset := req.Offset
		index := calculateCurrentIndex(offset, offsets)
		limit := req.Limit
		if limit <= 0 {
			limit = 100
		}
		for ; index < len(titles) && limit > 0; index++ {
			working := req
			if len(titles) > 2 && index == len(titles)-1 && kind == model.MediaSonarr && offsets[index-1] < s.minCount {
				if working.SeasonNumber != "" {
					working.SeasonNumber = ""
					working.EpisodeNumber = ""
				} else {
					working.SearchKey = util.RemoveSeasonEpisode(working.SearchKey)
				}
			}
			updateRequest(&working, index, titles, offsets)
			working.Offset = offset
			working.Limit = limit

			newXML, err := s.fetchUpstream(r, kind, indexer, working)
			if err != nil {
				return "", err
			}
			count := util.CountItems(newXML)
			if count > limit {
				count = limit
				newXML = util.RemoveOverflowItems(newXML, count)
				offset -= count
			}
			if count > 0 || xml == "" {
				xml = util.MergeXML(xml, newXML)
			}
			offset += count
			offsets[index] = offset
			s.saveOffsetList(offsetKey, offsets)
			limit -= count
		}
	} else {
		var err error
		xml, err = s.fetchUpstream(r, kind, indexer, req)
		if err != nil {
			return "", err
		}
	}

	if strings.Contains(xml, "<channel>") {
		switch kind {
		case model.MediaSonarr:
			return s.formatSonarrXML(xml)
		case model.MediaRadarr:
			return s.formatRadarrXML(xml)
		}
	}
	return xml, nil
}

func detectRoute(path string) (model.MediaKind, model.IndexerKind, error) {
	switch {
	case strings.HasPrefix(path, "/sonarr/jackett/"):
		return model.MediaSonarr, model.IndexerJackett, nil
	case strings.HasPrefix(path, "/sonarr/prowlarr/"):
		return model.MediaSonarr, model.IndexerProwlarr, nil
	case strings.HasPrefix(path, "/radarr/jackett/"):
		return model.MediaRadarr, model.IndexerJackett, nil
	case strings.HasPrefix(path, "/radarr/prowlarr/"):
		return model.MediaRadarr, model.IndexerProwlarr, nil
	default:
		return "", "", fmt.Errorf("unsupported route: %s", path)
	}
}

func parseRequest(values url.Values) model.QueryRequest {
	return model.QueryRequest{
		SearchKey:     values.Get("q"),
		SearchType:    values.Get("t"),
		SeasonNumber:  values.Get("season"),
		EpisodeNumber: values.Get("ep"),
		Offset:        atoi(values.Get("offset")),
		Limit:         atoi(values.Get("limit")),
	}
}

func generateOffsetKey(path, rawQuery string) string {
	query := regexp.MustCompile(`(offset=\d+|apikey=\w+)`).ReplaceAllString(rawQuery, "")
	return path + "?" + query
}

func generateCachePath(path string, kind model.MediaKind, indexer model.IndexerKind) string {
	prefix := "/" + string(kind) + "/" + string(indexer)
	return strings.TrimPrefix(path, prefix)
}

func calculateCurrentIndex(offset int, offsets []int) int {
	if offset == 0 {
		return 0
	}
	for index, value := range offsets {
		if value >= offset {
			return index
		}
	}
	if len(offsets) == 0 {
		return 0
	}
	return len(offsets) - 1
}

func updateRequest(req *model.QueryRequest, index int, titles []string, offsets []int) {
	if index == 0 || len(titles) == 0 {
		return
	}
	lastIndex := index - 1
	title := titles[index]
	searchKey := strings.Replace(req.SearchKey, titles[0], title, 1)
	if lastIndex > 0 {
		searchKey = strings.Replace(searchKey, titles[lastIndex], title, 1)
	}
	req.SearchKey = searchKey
	req.Offset -= offsets[lastIndex]
}

func (s *Service) fetchUpstream(r *http.Request, kind model.MediaKind, indexer model.IndexerKind, req model.QueryRequest) (string, error) {
	baseKey := "jackettUrl"
	if indexer == model.IndexerProwlarr {
		baseKey = "prowlarrUrl"
	}
	baseURL, err := s.repo.ConfigValue(baseKey)
	if err != nil {
		return "", err
	}
	query := cloneValues(r.URL.Query())
	query.Set("q", req.SearchKey)
	setIfNotEmpty(query, "t", req.SearchType)
	setIfNotEmpty(query, "season", req.SeasonNumber)
	setIfNotEmpty(query, "ep", req.EpisodeNumber)
	query.Set("offset", strconv.Itoa(req.Offset))
	query.Set("limit", strconv.Itoa(req.Limit))
	fullURL := strings.TrimRight(baseURL, "/") + generateCachePath(r.URL.Path, kind, indexer) + "?" + query.Encode()

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, fullURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.client.Do(upstreamReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseLength))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *Service) sonarrSearchTitles(title string) ([]string, error) {
	result := []string{title}
	regex, err := s.repo.ConfigValue("cleanTitleRegex")
	if err != nil {
		return nil, err
	}
	cleanTitle := util.CleanTitle(title, regex)
	entry, err := s.repo.SonarrTitleByCleanTitle(cleanTitle)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		trimmed := util.RemoveSeason(title)
		if trimmed != title {
			cleanTitle = util.CleanTitle(trimmed, regex)
			entry, err = s.repo.SonarrTitleByCleanTitle(cleanTitle)
			if err != nil {
				return nil, err
			}
			title = trimmed
		}
	}
	if entry == nil {
		return result, nil
	}
	result[0] = title
	if entry.SNO == 0 || entry.SNO == 1 {
		tmdbTitles, err := s.repo.SonarrTMDBTitles(entry.TVDBID)
		if err != nil {
			return nil, err
		}
		if len(tmdbTitles) > 0 {
			for _, item := range tmdbTitles {
				result = append(result, item.Title)
			}
			result = append(result, tmdbTitles[0].Title)
		}
	}
	return result, nil
}

func (s *Service) radarrSearchTitles(title string) ([]string, error) {
	result := []string{title}
	titleWithoutYear := util.RemoveYear(title)
	if titleWithoutYear != title {
		result = append(result, titleWithoutYear)
		title = titleWithoutYear
	}
	regex, err := s.repo.ConfigValue("cleanTitleRegex")
	if err != nil {
		return nil, err
	}
	cleanTitle := util.CleanTitle(title, regex)
	entry, err := s.repo.RadarrTitleByCleanTitle(cleanTitle)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return result, nil
	}
	if entry.SNO == 1 || entry.SNO == 2 {
		result = append(result, entry.MainTitle, entry.MainTitle)
	}
	return result, nil
}

func (s *Service) formatSonarrXML(xml string) (string, error) {
	format, err := s.repo.ConfigValue("sonarrIndexerFormat")
	if err != nil {
		return "", err
	}
	if !strings.Contains(format, "{title}") {
		return xml, nil
	}
	cleanRegex, err := s.repo.ConfigValue("cleanTitleRegex")
	if err != nil {
		return "", err
	}
	catalog, err := s.sonarrCatalogEntries()
	if err != nil {
		return "", err
	}
	tokenRules, err := s.loadTokenRules(model.MediaSonarr, format)
	if err != nil {
		return "", err
	}
	return rewriteItems(xml, func(title, description string) string {
		text := normalizeText(title, description)
		formatted := format
		formatted = s.applySonarrTitleRules(text, formatted, cleanRegex, tokenRules["title"], catalog)
		if strings.Contains(formatted, "{title}") {
			return title
		}
		formatted = applyGenericRules(text, formatted, tokenRules)
		if strings.Contains(formatted, "{episode}") {
			return title
		}
		return strings.TrimSpace(util.RemoveAllToken(formatted))
	}), nil
}

func (s *Service) formatRadarrXML(xml string) (string, error) {
	format, err := s.repo.ConfigValue("radarrIndexerFormat")
	if err != nil {
		return "", err
	}
	if !strings.Contains(format, "{title}") {
		return xml, nil
	}
	cleanRegex, err := s.repo.ConfigValue("cleanTitleRegex")
	if err != nil {
		return "", err
	}
	catalog, err := s.radarrCatalogEntries()
	if err != nil {
		return "", err
	}
	tokenRules, err := s.loadTokenRules(model.MediaRadarr, format)
	if err != nil {
		return "", err
	}
	return rewriteItems(xml, func(title, description string) string {
		text := normalizeText(title, description)
		formatted := format
		formatted = s.applyRadarrTitleRules(text, formatted, cleanRegex, tokenRules, catalog)
		if strings.Contains(formatted, "{title}") {
			return title
		}
		formatted = applyGenericRules(text, formatted, tokenRules)
		return strings.TrimSpace(util.RemoveAllToken(formatted))
	}), nil
}

func (s *Service) applySonarrTitleRules(text, format, cleanRegex string, rules []model.Rule, catalog []model.SonarrTitle) string {
	cleanText := util.CleanTitle(text, cleanRegex)
	for _, rule := range rules {
		if strings.Contains(rule.Regex, "{cleanTitle}") {
			for _, title := range catalog {
				candidate := title.CleanTitle
				if candidate == "" {
					candidate = util.CleanTitle(title.Title, cleanRegex)
				}
				replaced := strings.ReplaceAll(candidate, util.Placeholder, ".?")
				pattern := strings.ReplaceAll(rule.Regex, "{cleanTitle}", replaced)
				if matchRegex(pattern, cleanText) {
					if looksLikeEmbeddedEnglish(cleanText, replaced, rule.Regex) {
						continue
					}
					formatted := util.ReplaceToken("title", title.MainTitle, format)
					if title.SeasonNumber != -1 && title.SeasonNumber != 1 {
						formatted = util.ReplaceToken("season", "S"+strconv.Itoa(title.SeasonNumber), formatted)
					}
					return formatted
				}
			}
			continue
		}
		if value, ok := replaceByRegex(rule.Regex, text, rule.Replacement); ok {
			return util.ReplaceToken("title", value, format)
		}
	}
	return format
}

func (s *Service) applyRadarrTitleRules(text, format, cleanRegex string, tokenRules map[string][]model.Rule, catalog []model.RadarrTitle) string {
	titleRules := tokenRules["title"]
	yearRules := tokenRules["year"]
	cleanText := util.CleanTitle(text, cleanRegex)
	for _, rule := range titleRules {
		if strings.Contains(rule.Regex, "{cleanTitle}") {
			for _, title := range catalog {
				candidate := title.CleanTitle
				if candidate == "" {
					candidate = util.CleanTitle(title.Title, cleanRegex)
				}
				replaced := strings.ReplaceAll(candidate, util.Placeholder, ".?")
				pattern := strings.ReplaceAll(rule.Regex, "{cleanTitle}", replaced)
				if !matchRegex(pattern, cleanText) {
					continue
				}
				if looksLikeEmbeddedEnglish(cleanText, replaced, rule.Regex) {
					continue
				}
				titleYear := strconv.Itoa(title.Year)
				if !yearMatches(text, titleYear, yearRules) {
					continue
				}
				formatted := util.ReplaceToken("title", title.MainTitle, format)
				return util.ReplaceToken("year", titleYear, formatted)
			}
			continue
		}
		if value, ok := replaceByRegex(rule.Regex, text, rule.Replacement); ok {
			return util.ReplaceToken("title", value, format)
		}
	}
	return format
}

func applyGenericRules(text, format string, tokenRules map[string][]model.Rule) string {
	matches := regexp.MustCompile(util.TokenRegex).FindAllStringSubmatch(format, -1)
	for _, match := range matches {
		token := match[1]
		for _, rule := range tokenRules[token] {
			if value, ok := replaceByRegex(rule.Regex, text, rule.Replacement); ok {
				format = util.ReplaceTokenWithOffset(token, value, format, rule.Offset)
				break
			}
		}
	}
	return format
}

func normalizeText(title, description string) string {
	text := regexp.MustCompile(`\r\n|\r|\n`).ReplaceAllString(title, " ")
	if strings.TrimSpace(description) != "" {
		text += " " + util.PlaceholderSeparator + " " + description
	}
	return text
}

func (s *Service) loadTokenRules(kind model.MediaKind, format string) (map[string][]model.Rule, error) {
	result := map[string][]model.Rule{}
	seen := map[string]bool{}
	for _, match := range regexp.MustCompile(util.TokenRegex).FindAllStringSubmatch(format, -1) {
		token := match[1]
		if seen[token] {
			continue
		}
		seen[token] = true
		var rules []model.Rule
		var err error
		if kind == model.MediaSonarr {
			rules, err = s.repo.SonarrRules(token)
		} else {
			rules, err = s.repo.RadarrRules(token)
		}
		if err != nil {
			return nil, err
		}
		result[token] = rules
	}
	return result, nil
}

func (s *Service) sonarrCatalogEntries() ([]model.SonarrTitle, error) {
	s.catalogMu.RLock()
	if s.sonarrCatalog != nil {
		defer s.catalogMu.RUnlock()
		return s.sonarrCatalog, nil
	}
	s.catalogMu.RUnlock()

	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.sonarrCatalog != nil {
		return s.sonarrCatalog, nil
	}
	catalog, err := s.repo.SonarrCatalog()
	if err != nil {
		return nil, err
	}
	s.sonarrCatalog = catalog
	return catalog, nil
}

func (s *Service) radarrCatalogEntries() ([]model.RadarrTitle, error) {
	s.catalogMu.RLock()
	if s.radarrCatalog != nil {
		defer s.catalogMu.RUnlock()
		return s.radarrCatalog, nil
	}
	s.catalogMu.RUnlock()

	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.radarrCatalog != nil {
		return s.radarrCatalog, nil
	}
	catalog, err := s.repo.RadarrCatalog()
	if err != nil {
		return nil, err
	}
	s.radarrCatalog = catalog
	return catalog, nil
}

func (s *Service) getOffsetList(key string, size int) []int {
	s.offsetMu.Lock()
	defer s.offsetMu.Unlock()
	if value, ok := s.offsetCache[key]; ok && len(value) == size {
		return append([]int(nil), value...)
	}
	result := make([]int, size)
	s.rememberOffsetKeyLocked(key)
	s.offsetCache[key] = append([]int(nil), result...)
	return result
}

func (s *Service) saveOffsetList(key string, offsets []int) {
	s.offsetMu.Lock()
	defer s.offsetMu.Unlock()
	s.rememberOffsetKeyLocked(key)
	s.offsetCache[key] = append([]int(nil), offsets...)
}

func (s *Service) rememberOffsetKeyLocked(key string) {
	if _, ok := s.offsetCache[key]; ok {
		return
	}
	s.offsetOrder = append(s.offsetOrder, key)
	for len(s.offsetOrder) > maxOffsetCacheEntries {
		oldest := s.offsetOrder[0]
		s.offsetOrder = s.offsetOrder[1:]
		delete(s.offsetCache, oldest)
	}
}

func cloneValues(values url.Values) url.Values {
	result := url.Values{}
	for key, value := range values {
		result[key] = append([]string(nil), value...)
	}
	return result
}

func setIfNotEmpty(values url.Values, key, value string) {
	if value == "" {
		values.Del(key)
		return
	}
	values.Set(key, value)
}

func atoi(value string) int {
	number, _ := strconv.Atoi(value)
	return number
}

func replaceByRegex(pattern, text, replacement string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false
	}
	if !re.MatchString(text) {
		return "", false
	}
	return re.ReplaceAllString(text, replacement), true
}

func matchRegex(pattern, text string) bool {
	re, err := regexp.Compile("^(?:" + pattern + ")$")
	if err != nil {
		return false
	}
	return re.MatchString(text)
}

func looksLikeEmbeddedEnglish(cleanText, cleanTitle, basePattern string) bool {
	if !regexp.MustCompile(`[\.\?a-zA-Z]+`).MatchString(cleanTitle) {
		return false
	}
	prefix := strings.ReplaceAll(basePattern, "{cleanTitle}", "[a-zA-Z]+"+util.Placeholder+cleanTitle)
	suffix := strings.ReplaceAll(basePattern, "{cleanTitle}", cleanTitle+util.Placeholder+"[a-zA-Z]+")
	filtered := regexp.MustCompile(`( season \d+| episode \d+| ep \d+| aka | s\d+e\d+| s\d+| \d+)`).ReplaceAllString(cleanText, util.PlaceholderSeparator)
	return matchRegex(prefix, filtered) || matchRegex(suffix, filtered)
}

func yearMatches(text, titleYear string, rules []model.Rule) bool {
	for _, rule := range rules {
		if value, ok := replaceByRegex(rule.Regex, text, rule.Replacement); ok {
			return value == titleYear
		}
	}
	return true
}

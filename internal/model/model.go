package model

type MediaKind string

const (
	MediaSonarr MediaKind = "sonarr"
	MediaRadarr MediaKind = "radarr"
)

type IndexerKind string

const (
	IndexerJackett  IndexerKind = "jackett"
	IndexerProwlarr IndexerKind = "prowlarr"
)

type QueryRequest struct {
	SearchKey     string
	SearchType    string
	SeasonNumber  string
	EpisodeNumber string
	Offset        int
	Limit         int
}

type Rule struct {
	Token       string
	Priority    int
	Regex       string
	Replacement string
	Offset      int
}

type RuleRecord struct {
	ID          string `json:"id"`
	Token       string `json:"token"`
	Priority    int    `json:"priority"`
	Regex       string `json:"regex"`
	Replacement string `json:"replacement"`
	Offset      int    `json:"offset"`
	Example     string `json:"example"`
	Remark      string `json:"remark"`
	Author      string `json:"author"`
	ValidStatus int    `json:"validStatus"`
}

type SonarrTitle struct {
	ID           int    `json:"id"`
	SeriesID     int    `json:"seriesId"`
	TVDBID       int    `json:"tvdbId"`
	SNO          int    `json:"sno"`
	MainTitle    string `json:"mainTitle"`
	Title        string `json:"title"`
	CleanTitle   string `json:"cleanTitle"`
	SeasonNumber int    `json:"seasonNumber"`
	Monitored    int    `json:"monitored"`
	ValidStatus  int    `json:"validStatus"`
}

type RadarrTitle struct {
	ID          int    `json:"id"`
	MovieID     int    `json:"movieId"`
	TMDBID      int    `json:"tmdbId"`
	SNO         int    `json:"sno"`
	MainTitle   string `json:"mainTitle"`
	Title       string `json:"title"`
	CleanTitle  string `json:"cleanTitle"`
	Year        int    `json:"year"`
	Monitored   int    `json:"monitored"`
	ValidStatus int    `json:"validStatus"`
}

type TmdbTitle struct {
	ID          int    `json:"id"`
	TVDBID      int    `json:"tvdbId"`
	TMDBID      int    `json:"tmdbId"`
	Language    string `json:"language"`
	Title       string `json:"title"`
	CleanTitle  string `json:"cleanTitle,omitempty"`
	ValidStatus int    `json:"validStatus"`
}

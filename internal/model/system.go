package model

type SystemConfig struct {
	ID          int    `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	ValidStatus int    `json:"validStatus"`
}

type SyncJobStatus struct {
	Name           string `json:"name"`
	LastStartedAt  string `json:"lastStartedAt,omitempty"`
	LastFinishedAt string `json:"lastFinishedAt,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	Running        bool   `json:"running"`
}

type PageResponse[T any] struct {
	Current  int `json:"current"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
	List     []T `json:"list"`
}

type PageQuery struct {
	Current      int
	PageSize     int
	Title        string
	Token        string
	Remark       string
	OriginalText string
	ValidStatus  int
	TVDBID       int
	TMDBID       int
}

type Example struct {
	Hash         string `json:"hash"`
	OriginalText string `json:"originalText"`
	FormatText   string `json:"formatText"`
	ValidStatus  int    `json:"validStatus"`
}

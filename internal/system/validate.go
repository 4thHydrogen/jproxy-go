package system

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func checkAPIHealth(ctx context.Context, client *http.Client, rawURL, apiKey string) int {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(apiKey) == "" {
		return 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(rawURL, "/")+"/api/v3/health?apikey="+apiKey, nil)
	if err != nil {
		return 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 1
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return 0
	}
	return 1
}

func checkTMDB(ctx context.Context, client *http.Client, rawURL, apiKey string) int {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(apiKey) == "" {
		return 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(rawURL, "/")+"/3/movie/550?api_key="+apiKey, nil)
	if err != nil {
		return 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 1
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return 0
	}
	return 1
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

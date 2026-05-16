package system

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUIServiceServesIndexAndNoLoginEndpoints(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>legacy-ui</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	service := NewUIService(dir)
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/system/user/isLoginEnabled", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || resp.Body.String() != "false\n" {
		t.Fatalf("unexpected isLoginEnabled response: %d %q", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/system/user/update", nil)
	resp = httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected user update response: %d %q", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
	resp = httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || resp.Body.String() != "<html>legacy-ui</html>" {
		t.Fatalf("unexpected static fallback response: %d %q", resp.Code, resp.Body.String())
	}
}

package system

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"jproxy/core-proxy/internal/model"
	"jproxy/core-proxy/internal/repository"
)

type testCacheResetter struct{}

func (testCacheResetter) ResetCaches() {}

func TestRuleImportPreservesDisabledRules(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "jproxy.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	defer repo.Close()
	if err := repo.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	service := NewAdminService(repo, testCacheResetter{}, nil)
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	rules := []model.RuleRecord{{
		ID:          "disabled-rule",
		Token:       "title",
		Priority:    10,
		Regex:       "foo",
		Replacement: "bar",
		ValidStatus: 0,
	}}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "rules.json")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if err := json.NewEncoder(part).Encode(rules); err != nil {
		t.Fatalf("encode rules: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sonarr/rule/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected import response: %d %q", resp.Code, resp.Body.String())
	}

	page, err := repo.QueryRules(context.Background(), "sonarr_rule", model.PageQuery{Current: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("query rules: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected one imported rule, got %d", page.Total)
	}
	if page.List[0].ValidStatus != 0 {
		t.Fatalf("expected disabled rule to stay disabled, got %d", page.List[0].ValidStatus)
	}
}

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"sessiondesk/internal/sessions"
)

func TestSessionAPIContracts(t *testing.T) {
	service, err := sessions.New(sessions.Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(service, fstest.MapFS{"index.html": {Data: []byte("Session Desk")}})
	for _, path := range []string{"/api/state", "/api/settings", "/api/transcript?key=" + view.Sessions[0].Key, "/api/command?key=" + view.Sessions[0].Key} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("session responses must not be cached by the browser")
		}
	}
	for _, path := range []string{"/api/transcript?key=/etc/passwd", "/api/transcript?cursor=-1&key=" + view.Sessions[0].Key, "/api/command?key=unknown"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid session target accepted: %s", path)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/export?key="+view.Sessions[0].Key, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "## assistant") || !strings.Contains(response.Body.String(), "工作目录") {
		t.Fatalf("export: %d %s", response.Code, response.Body.String())
	}
}

func TestSettingsAndResumeValidateBoundary(t *testing.T) {
	service, err := sessions.New(sessions.Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := Handler(service, fstest.MapFS{})
	for _, input := range []struct{ path, contentType, body string }{
		{"/api/settings", "text/plain", `{"sources":[]}`},
		{"/api/settings", "application/json", `{"sources":[{"id":"unknown","claude_dir":"/tmp"}]}`},
		{"/api/settings", "application/json", `{"sources":[{"id":"windows:local","claude_dir":"relative/path"}]}`},
		{"/api/resume", "application/json", `{"key":"unknown","command":"untrusted"}`},
		{"/api/resume", "application/json", `{"key":"unknown"}`},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, input.path, strings.NewReader(input.body))
		request.Header.Set("Content-Type", input.contentType)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("accepted invalid boundary input: %+v", input)
		}
	}
}

func TestMutationStateAndBatchRestore(t *testing.T) {
	service, err := sessions.New(sessions.Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(service, fstest.MapFS{})
	request := func(path string, body any) []byte {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		method := http.MethodPost
		if body == nil {
			method = http.MethodGet
		}
		in := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		if body != nil {
			in.Header.Set("Content-Type", "application/json")
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, in)
		if out.Code != http.StatusOK {
			t.Fatalf("%s: %s", path, out.Body.String())
		}
		return out.Body.Bytes()
	}
	var plan sessions.DeletePlan
	if err := json.Unmarshal(request("/api/delete/preview", map[string]any{"keys": []string{view.Sessions[0].Key}}), &plan); err != nil {
		t.Fatal(err)
	}
	var result sessions.MutationResult
	if err := json.Unmarshal(request("/api/delete", map[string]any{"keys": plan.Keys, "revisions": plan.Revisions}), &result); err != nil || len(result.Failed) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	var next sessions.Snapshot
	if err := json.Unmarshal(request("/api/state", nil), &next); err != nil || len(next.Sessions) != 16-len(plan.Keys) {
		t.Fatalf("stale state: %d %v", len(next.Sessions), err)
	}
	var trash struct {
		Entries []sessions.TrashEntry `json:"entries"`
	}
	if err := json.Unmarshal(request("/api/trash", nil), &trash); err != nil || len(trash.Entries) != 1 {
		t.Fatalf("trash: %+v %v", trash, err)
	}
	if err := json.Unmarshal(request("/api/trash/restore", map[string]any{"keys": []string{trash.Entries[0].Key}}), &result); err != nil || len(result.Done) != 1 || len(result.Failed) != 0 {
		t.Fatalf("restore: %+v %v", result, err)
	}
	if err := json.Unmarshal(request("/api/state", nil), &next); err != nil || len(next.Sessions) != 16 {
		t.Fatalf("restored state: %d %v", len(next.Sessions), err)
	}
}

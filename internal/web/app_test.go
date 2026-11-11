package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"sessiondesk/internal/buildinfo"
	"sessiondesk/internal/prefs"
	"sessiondesk/internal/sessions"
	"sessiondesk/internal/update"
)

func appHandler(t *testing.T, options Options) http.Handler {
	t.Helper()
	service, err := sessions.New(sessions.Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(service, fstest.MapFS{}, options)
}

func call(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestAppInfoAndPreferences(t *testing.T) {
	store := prefs.Open(filepath.Join(t.TempDir(), "preferences.json"))
	handler := appHandler(t, Options{Build: buildinfo.Info{Version: "1.2.3"}, Preferences: store})
	var info struct {
		Version     string `json:"version"`
		UpdateCheck bool   `json:"update_check"`
	}
	json.NewDecoder(call(handler, "GET", "/api/app", "").Body).Decode(&info)
	if info.Version != "1.2.3" || !info.UpdateCheck {
		t.Fatalf("unexpected app info %+v", info)
	}
	if response := call(handler, "POST", "/api/app/preferences", `{"update_check":false}`); response.Code != 200 {
		t.Fatalf("saving preferences failed: %s", response.Body)
	}
	if store.Get().AutoUpdateCheck() {
		t.Fatal("update check preference was not saved")
	}
	var result update.Result
	json.NewDecoder(call(handler, "GET", "/api/update", "").Body).Decode(&result)
	if result.Status != update.StatusDisabled {
		t.Fatalf("automatic check should be disabled, got %+v", result)
	}
}

func TestUpdateCheckUsesChecker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v9.0.0","html_url":"https://github.com/o/r/releases/tag/v9.0.0"}`))
	}))
	defer server.Close()
	checker := update.NewChecker("o/r", "1.0.0")
	checker.BaseURL = server.URL
	handler := appHandler(t, Options{Build: buildinfo.Info{Version: "1.0.0"}, Updates: checker, Demo: true})
	var result update.Result
	json.NewDecoder(call(handler, "GET", "/api/update", "").Body).Decode(&result)
	if result.Status != update.StatusDisabled {
		t.Fatalf("demo mode must not check automatically, got %+v", result)
	}
	json.NewDecoder(call(handler, "GET", "/api/update?force=1", "").Body).Decode(&result)
	if result.Status != update.StatusAvailable || result.Latest != "9.0.0" {
		t.Fatalf("manual check should report update, got %+v", result)
	}
}

func TestOpenURLValidation(t *testing.T) {
	var opened []string
	handler := appHandler(t, Options{OpenURL: func(link string) error { opened = append(opened, link); return nil }})
	for _, bad := range []string{`{"url":"file:///C:/Windows/system32/calc.exe"}`, `{"url":"javascript:alert(1)"}`, `{"url":"ms-settings:"}`} {
		if response := call(handler, "POST", "/api/open-url", bad); response.Code != 400 {
			t.Fatalf("%s should be rejected", bad)
		}
	}
	if response := call(handler, "POST", "/api/open-url", `{"url":"https://github.com/owner/name/releases"}`); response.Code != 200 {
		t.Fatalf("https link should open: %s", response.Body)
	}
	if len(opened) != 1 {
		t.Fatalf("expected one opened link, got %v", opened)
	}
	if response := call(appHandler(t, Options{}), "POST", "/api/open-url", `{"url":"https://example.com"}`); response.Code != 400 {
		t.Fatal("browser preview mode should refuse to open links")
	}
}

func TestUpdateInstallNeedsInstaller(t *testing.T) {
	handler := appHandler(t, Options{Build: buildinfo.Info{Version: "1.2.3"}})
	if response := call(handler, "POST", "/api/update/install", `{}`); response.Code == 200 {
		t.Fatal("install must be refused without an installer")
	}
	var state update.InstallState
	json.NewDecoder(call(handler, "GET", "/api/update/install", "").Body).Decode(&state)
	if state.Phase != update.PhaseIdle {
		t.Fatalf("unexpected state %+v", state)
	}
}

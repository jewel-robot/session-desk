package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"0.3.0", "0.2.2", true},
		{"v0.3.0", "0.3.0", false},
		{"0.2.10", "0.2.9", true},
		{"1.0.0", "1.0.0-beta.1", true},
		{"1.0.0-beta.2", "1.0.0-beta.1", true},
		{"1.0.0-beta.1", "1.0.0", false},
		{"0.1.0", "0.2.0", false},
		{"0.3.0", "dev", false},
		{"nightly", "0.2.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.candidate, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

func serve(t *testing.T, status int, body string) *Checker {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	checker := NewChecker("owner/repo", "0.3.0")
	checker.BaseURL = server.URL
	return checker
}

func TestCheckAvailablePrefersInstaller(t *testing.T) {
	checker := serve(t, 200, `{"tag_name":"v0.4.0","html_url":"https://github.com/owner/repo/releases/tag/v0.4.0","body":"notes","assets":[
		{"name":"session-desk-windows-amd64-v0.4.0.zip","browser_download_url":"https://example.test/zip"},
		{"name":"session-desk-0.4.0-setup.exe","browser_download_url":"https://example.test/setup"}]}`)
	result := checker.Check(context.Background(), true)
	if result.Status != StatusAvailable || result.Latest != "0.4.0" || result.DownloadURL != "https://example.test/setup" {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestCheckLatestAndNoRelease(t *testing.T) {
	if result := serve(t, 200, `{"tag_name":"0.3.0","html_url":"u"}`).Check(context.Background(), true); result.Status != StatusLatest {
		t.Fatalf("want latest, got %+v", result)
	}
	if result := serve(t, 404, `{"message":"Not Found"}`).Check(context.Background(), true); result.Status != StatusNone {
		t.Fatalf("want none, got %+v", result)
	}
	if result := serve(t, 500, `oops`).Check(context.Background(), true); result.Status != StatusError || result.Error == "" {
		t.Fatalf("want error, got %+v", result)
	}
}

func TestCheckOfflineAndCache(t *testing.T) {
	checker := NewChecker("owner/repo", "0.3.0")
	checker.BaseURL = "http://127.0.0.1:1"
	if result := checker.Check(context.Background(), false); result.Status != StatusError {
		t.Fatalf("want error when offline, got %+v", result)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"tag_name":"0.3.0"}`))
	}))
	defer server.Close()
	checker.BaseURL = server.URL
	checker.Check(context.Background(), false)
	checker.Check(context.Background(), false)
	if calls != 1 {
		t.Fatalf("cached check should call once, got %d", calls)
	}
	checker.Check(context.Background(), true)
	if calls != 2 {
		t.Fatalf("forced check should bypass cache, got %d calls", calls)
	}
}

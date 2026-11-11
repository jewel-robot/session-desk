package update

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func installer(t *testing.T, body []byte) (*Installer, Result, *[]string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	t.Cleanup(server.Close)
	launched := &[]string{}
	i := NewInstaller("owner/repo")
	i.AllowedPrefix = server.URL + "/owner/repo/releases/download/"
	i.Dir = t.TempDir()
	i.Launch = func(path string) error { *launched = append(*launched, path); return nil }
	sum := sha256.Sum256(body)
	result := Result{Status: StatusAvailable, AssetName: "session-desk-9.9.9-setup.exe", DownloadURL: i.AllowedPrefix + "v9.9.9/session-desk-9.9.9-setup.exe", Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(body))}
	return i, result, launched
}

func wait(t *testing.T, i *Installer) InstallState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if state := i.State(); state.Phase == PhaseLaunched || state.Phase == PhaseError {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("install did not finish")
	return InstallState{}
}

func TestInstallVerifiesDigestBeforeLaunch(t *testing.T) {
	i, result, launched := installer(t, []byte("installer"))
	done := make(chan struct{})
	i.Done = func() { close(done) }
	if err := i.Start(result); err != nil {
		t.Fatal(err)
	}
	state := wait(t, i)
	<-done
	if state.Phase != PhaseLaunched || state.Received != 9 || len(*launched) != 1 {
		t.Fatalf("unexpected state %+v, launched %v", state, *launched)
	}
	if data, err := os.ReadFile((*launched)[0]); err != nil || string(data) != "installer" {
		t.Fatalf("installer not saved: %v", err)
	}
}

func TestInstallRejectsMismatchAndBadReleases(t *testing.T) {
	i, result, launched := installer(t, []byte("tampered"))
	result.Digest = "sha256:" + hex.EncodeToString(make([]byte, 32))
	if err := i.Start(result); err != nil {
		t.Fatal(err)
	}
	if state := wait(t, i); state.Phase != PhaseError || len(*launched) != 0 {
		t.Fatalf("mismatch must not launch: %+v", state)
	}
	if entries, _ := os.ReadDir(i.Dir); len(entries) != 0 {
		t.Fatalf("mismatched download left behind: %v", entries)
	}
	for name, change := range map[string]func(*Result){
		"no digest":  func(r *Result) { r.Digest = "" },
		"zip":        func(r *Result) { r.AssetName = "session-desk.zip" },
		"traversal":  func(r *Result) { r.AssetName = "../x-setup.exe" },
		"other host": func(r *Result) { r.DownloadURL = "https://example.test/session-desk-9.9.9-setup.exe" },
		"not newer":  func(r *Result) { r.Status = StatusLatest },
	} {
		bad := result
		change(&bad)
		if err := i.Start(bad); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	if filepath.Base(result.AssetName) != result.AssetName {
		t.Fatal("asset name should be a plain file name")
	}
}

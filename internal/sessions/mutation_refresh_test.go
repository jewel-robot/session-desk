package sessions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeletePreviewRefreshesOnlySelectedStore(t *testing.T) {
	ctx := context.Background()
	s := demoService(t)
	var selected, other Session
	for _, session := range s.View().Sessions {
		if session.Provider != "claude" {
			continue
		}
		if session.SourceID == s.fixed[0].ID {
			selected = session
		} else {
			other = session
		}
	}
	raw, err := os.ReadFile(other.Path)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, other.Path, string(raw)+"{\"type\":\"custom-title\",\"customTitle\":\"changed outside selection\"}\n")
	writeFixture(t, filepath.Join(s.fixed[0].OpenCodeDir, "opencode.db"), "broken unrelated database")
	plan, err := s.PrepareDelete(ctx, []string{selected.Key})
	if err != nil || len(plan.Keys) != 1 {
		t.Fatalf("preview: %+v %v", plan, err)
	}
	unchanged, _, err := s.Find(other.Key)
	if err != nil || unchanged.Title != other.Title {
		t.Fatalf("unrelated source scanned: %+v %v", unchanged, err)
	}
	if len(s.View().Warnings) != 0 {
		t.Fatalf("unrelated provider scanned: %v", s.View().Warnings)
	}
	if len(s.View().Sessions) != 16 {
		t.Fatal("unrelated sessions dropped")
	}
}

func TestMutationPublishesStateAndOfflineCache(t *testing.T) {
	ctx := context.Background()
	s := demoService(t)
	var target Session
	for _, session := range s.View().Sessions {
		if session.Provider == "claude" {
			target = session
			break
		}
	}
	plan, err := s.PrepareDelete(ctx, []string{target.Key})
	if err != nil {
		t.Fatal(err)
	}
	result := s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Failed) > 0 || len(s.View().Sessions) != 15 {
		t.Fatalf("state not updated: %+v %d", result, len(s.View().Sessions))
	}
	if _, _, err := s.Find(target.Key); err == nil {
		t.Fatal("deleted target remains addressable")
	}
	restarted, err := New(Options{ConfigPath: s.options.ConfigPath})
	if err != nil {
		t.Fatal(err)
	}
	restarted.fixed = append([]Source{}, s.fixed...)
	for i := range restarted.fixed {
		restarted.fixed[i].Running = false
	}
	view, err := restarted.Refresh(ctx)
	if err != nil || len(view.Sessions) != 15 {
		t.Fatalf("offline cache resurrected deletion: %d %v", len(view.Sessions), err)
	}
	entries, err := s.Trash(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("%+v %v", entries, err)
	}
	result = s.RestoreMany(ctx, []string{entries[0].Key, "unknown|claude|invalid"})
	if len(result.Done) != 1 || len(result.Failed) != 1 || len(s.View().Sessions) != 16 {
		t.Fatalf("restored state: %+v %d", result, len(s.View().Sessions))
	}
	restored, _, err := s.Find(target.Key)
	if err != nil || restored.Title != target.Title {
		t.Fatalf("restored lookup: %+v %v", restored, err)
	}
}

func TestWorkerTrashAndPurgeSkipSessionScanning(t *testing.T) {
	root := t.TempDir()
	source := Source{ID: "wsl:test", Name: "test", Kind: "local", Home: root, OpenCodeDir: filepath.Join(root, "opencode"), Running: true}
	writeFixture(t, filepath.Join(source.OpenCodeDir, "opencode.db"), "invalid SQLite file")
	cachePath := filepath.Join(root, ".local", "share", "session-desk", "worker", "session-cache.json")
	writeFixture(t, cachePath, "{}")
	response, err := HandleWorker(context.Background(), WorkerRequest{Action: "trash", Source: source})
	if err != nil || len(response.Trash) != 0 {
		t.Fatalf("trash: %+v %v", response, err)
	}
	raw, err := os.ReadFile(cachePath)
	if err != nil || string(raw) != "{}" {
		t.Fatal("trash scanned and rewrote session cache")
	}
	id := strings.Repeat("a", 32)
	entry := TrashEntry{ID: id, Key: source.ID + "|opencode|" + id, SourceID: source.ID, Provider: "opencode", Status: "trashed", Sessions: []Session{}}
	if err := writeJSON(filepath.Join(source.OpenCodeDir, ".session-desk-trash", id, "manifest.json"), entry); err != nil {
		t.Fatal(err)
	}
	response, err = HandleWorker(context.Background(), WorkerRequest{Action: "purge", Source: source, Keys: []string{entry.Key}})
	if err != nil || len(response.Result.Done) != 1 || len(response.Result.Failed) != 0 {
		t.Fatalf("purge: %+v %v", response.Result, err)
	}
	raw, err = os.ReadFile(cachePath)
	if err != nil || string(raw) != "{}" {
		t.Fatal("purge rewrote session cache")
	}
}

func TestColdWorkerScopedPreviewPreservesOtherCachedTitles(t *testing.T) {
	ctx := context.Background()
	s := demoService(t)
	source := s.fixed[0]
	// Reuse the real worker cache location with an isolated home.
	source.Home = t.TempDir()
	key := "windows:local|opencode|ses_demoWindows"
	if _, err := HandleWorker(ctx, WorkerRequest{Action: "list", Source: source}); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(source.Home, ".local", "share", "session-desk", "worker", "session-cache.json")
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	response, err := HandleWorker(ctx, WorkerRequest{Action: "preview", Source: source, Keys: []string{key}})
	if err != nil || len(response.Plan.Keys) != 2 {
		t.Fatalf("preview: %+v %v", response, err)
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var original, current map[string]cachedSession
	if err := json.Unmarshal(before, &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &current); err != nil {
		t.Fatal(err)
	}
	for cacheKey, cached := range original {
		if cached.Session.Provider == "opencode" {
			continue
		}
		if current[cacheKey].Session.Title != cached.Session.Title {
			t.Fatalf("cached title changed: %s", cacheKey)
		}
	}
}

func TestRestoreRejectsMetadataPathsOutsideStore(t *testing.T) {
	ctx := context.Background()
	s := demoService(t)
	var target Session
	for _, session := range s.View().Sessions {
		if session.Provider == "claude" {
			target = session
			break
		}
	}
	plan, err := s.PrepareDelete(ctx, []string{target.Key})
	if err != nil {
		t.Fatal(err)
	}
	result := s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Failed) > 0 {
		t.Fatal(result)
	}
	entries, err := s.Trash(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("%+v %v", entries, err)
	}
	entry := entries[0]
	path := filepath.Join(entry.Root, ".session-desk-trash", entry.ID, "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest TrashEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Sessions[0].Path = filepath.Join(t.TempDir(), "outside.jsonl")
	if err := writeJSON(path, manifest); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(ctx, entry.Key); err == nil {
		t.Fatal("out-of-store metadata accepted")
	}
	if len(s.View().Sessions) != 15 {
		t.Fatal("invalid metadata published")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("backup lost after validation error")
	}
}

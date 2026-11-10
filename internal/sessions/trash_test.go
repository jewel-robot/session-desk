package sessions

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTrashRoundTripProviders(t *testing.T) {
	for _, provider := range Providers {
		t.Run(provider, func(t *testing.T) {
			ctx := context.Background()
			s := demoService(t)
			var session Session
			for _, item := range s.View().Sessions {
				if item.Provider == provider && item.SourceID == "windows:local" && item.ParentID == "" {
					session = item
					break
				}
			}
			before, err := s.Transcript(ctx, session.Key, 0)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := s.PrepareDelete(ctx, []string{session.Key})
			if err != nil {
				t.Fatal(err)
			}
			if provider == "opencode" && (len(plan.Keys) != 2 || plan.Related != 1) {
				t.Fatalf("child expansion: %+v", plan)
			}
			result := s.Delete(ctx, plan.Keys, plan.Revisions)
			if len(result.Failed) > 0 || len(result.Done) != len(plan.Keys) {
				t.Fatalf("delete: %+v", result)
			}
			view, err := s.Refresh(ctx)
			if err != nil || len(view.Sessions) != 16-len(plan.Keys) {
				t.Fatalf("deleted list: %d %v", len(view.Sessions), err)
			}
			entries, err := s.Trash(ctx)
			if err != nil || len(entries) != 1 || entries[0].Status != "trashed" {
				t.Fatalf("trash: %+v %v", entries, err)
			}
			if len(entries[0].Databases) != 0 || len(entries[0].AgyCache) != 0 {
				t.Fatal("private backup leaked")
			}
			if err := s.Restore(ctx, entries[0].Key); err != nil {
				t.Fatal(err)
			}
			view, err = s.Refresh(ctx)
			if err != nil || len(view.Sessions) != 16 {
				t.Fatalf("restored list: %d %v", len(view.Sessions), err)
			}
			after, err := s.Transcript(ctx, session.Key, 0)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("transcript changed: %+v %+v %v", before, after, err)
			}
			entries, err = s.Trash(ctx)
			if err != nil || len(entries) != 0 {
				t.Fatalf("restored entry retained: %+v %v", entries, err)
			}
		})
	}
}

func TestDatabaseBackupPreservesTypesDependenciesAndNewRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db := fixtureDB(t, path, `PRAGMA foreign_keys=ON;
 CREATE TABLE session (id TEXT PRIMARY KEY);
 CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT REFERENCES session(id) ON DELETE CASCADE, big INTEGER, real_value REAL, blob_value BLOB, empty_blob BLOB, null_value TEXT);
 CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT REFERENCES message(id) ON DELETE CASCADE, data TEXT);
 INSERT INTO session VALUES ('ses_first'),('ses_keep');
 INSERT INTO message VALUES ('msg_first','ses_first',9007199254740993,1.25,x'00ff0100',x'',NULL);
 INSERT INTO part VALUES ('prt_first','msg_first','原消息');`)
	change, err := captureDatabase(ctx, path, "opencode", []string{"ses_first"})
	if err != nil {
		t.Fatal(err)
	}
	if len(change.backup.Tables) != 3 {
		t.Fatalf("dependent rows: %+v", change.backup)
	}
	raw, err := json.Marshal(change.backup)
	if err != nil {
		t.Fatal(err)
	}
	var backup DatabaseBackup
	if err := json.Unmarshal(raw, &backup); err != nil {
		t.Fatal(err)
	}
	if err := change.deleteRows(ctx); err != nil {
		t.Fatal(err)
	}
	if err := commitChanges([]*databaseChange{change}); err != nil {
		t.Fatal(err)
	}
	closeChanges([]*databaseChange{change})
	if _, err := db.Exec(`INSERT INTO session VALUES ('ses_new'); INSERT INTO message VALUES ('msg_new','ses_new',1,2,x'01',x'',NULL)`); err != nil {
		t.Fatal(err)
	}
	restored, err := restoreDatabase(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitChanges([]*databaseChange{restored}); err != nil {
		t.Fatal(err)
	}
	closeChanges([]*databaseChange{restored})
	var big int64
	var realValue float64
	var blob, empty []byte
	var null any
	if err := db.QueryRow(`SELECT big,real_value,blob_value,empty_blob,null_value FROM message WHERE id='msg_first'`).Scan(&big, &realValue, &blob, &empty, &null); err != nil {
		t.Fatal(err)
	}
	if big != 9007199254740993 || realValue != 1.25 || !slices.Equal(blob, []byte{0, 255, 1, 0}) || len(empty) != 0 || null != nil {
		t.Fatalf("typed values: %d %f %v %v %v", big, realValue, blob, empty, null)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM session`).Scan(&count)
	if count != 3 {
		t.Fatalf("new or unrelated row lost: %d", count)
	}
	db.QueryRow(`SELECT count(*) FROM part`).Scan(&count)
	if count != 1 {
		t.Fatal("dependent part lost")
	}
}

func TestDatabaseRestoreConflictAndUnconfirmedChild(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db := fixtureDB(t, path, `PRAGMA foreign_keys=ON;
 CREATE TABLE session (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES session(id) ON DELETE CASCADE, title TEXT);
 INSERT INTO session VALUES ('ses_parent',NULL,'原内容'),('ses_child','ses_parent','子会话');`)
	if change, err := captureDatabase(ctx, path, "opencode", []string{"ses_parent"}); err == nil {
		closeChanges([]*databaseChange{change})
		t.Fatal("unconfirmed child accepted")
	}
	change, err := captureDatabase(ctx, path, "opencode", []string{"ses_parent", "ses_child"})
	if err != nil {
		t.Fatal(err)
	}
	backup := change.backup
	if err := change.deleteRows(ctx); err != nil {
		t.Fatal(err)
	}
	if err := commitChanges([]*databaseChange{change}); err != nil {
		t.Fatal(err)
	}
	closeChanges([]*databaseChange{change})
	if _, err := db.Exec(`INSERT INTO session VALUES ('ses_parent',NULL,'新内容')`); err != nil {
		t.Fatal(err)
	}
	if restored, err := restoreDatabase(ctx, backup); err == nil {
		closeChanges([]*databaseChange{restored})
		t.Fatal("conflicting record overwritten")
	}
	var title string
	db.QueryRow(`SELECT title FROM session WHERE id='ses_parent'`).Scan(&title)
	if title != "新内容" {
		t.Fatal(title)
	}
}

func TestCodexCatalogLocalRowsAndRevision(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "codex.db")
	db := fixtureDB(t, path, `CREATE TABLE local_thread_catalog_hosts (host_id TEXT PRIMARY KEY, host_kind TEXT);
 CREATE TABLE local_thread_catalog (host_id TEXT, thread_id TEXT, title TEXT, PRIMARY KEY(host_id,thread_id));
 CREATE TABLE local_thread_catalog_metadata (id INTEGER PRIMARY KEY, catalog_revision INTEGER);
 INSERT INTO local_thread_catalog_hosts VALUES ('this','local'),('remote','remote');
 INSERT INTO local_thread_catalog VALUES ('this','thread','local'),('remote','thread','remote');
 INSERT INTO local_thread_catalog_metadata VALUES (1,10);`)
	change, err := captureDatabase(ctx, path, "codex", []string{"thread"})
	if err != nil {
		t.Fatal(err)
	}
	backup := change.backup
	if err := change.deleteRows(ctx); err != nil {
		t.Fatal(err)
	}
	if err := commitChanges([]*databaseChange{change}); err != nil {
		t.Fatal(err)
	}
	closeChanges([]*databaseChange{change})
	var count, revision int
	db.QueryRow(`SELECT count(*) FROM local_thread_catalog`).Scan(&count)
	db.QueryRow(`SELECT catalog_revision FROM local_thread_catalog_metadata`).Scan(&revision)
	if count != 1 || revision != 11 {
		t.Fatalf("delete: count %d revision %d", count, revision)
	}
	restored, err := restoreDatabase(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitChanges([]*databaseChange{restored}); err != nil {
		t.Fatal(err)
	}
	closeChanges([]*databaseChange{restored})
	db.QueryRow(`SELECT count(*) FROM local_thread_catalog`).Scan(&count)
	db.QueryRow(`SELECT catalog_revision FROM local_thread_catalog_metadata`).Scan(&revision)
	if count != 2 || revision != 12 {
		t.Fatalf("restore: count %d revision %d", count, revision)
	}
}

func TestDeleteRejectsActiveAndStaleConfirmation(t *testing.T) {
	ctx := context.Background()
	s := demoService(t)
	var session Session
	for _, item := range s.View().Sessions {
		if item.Provider == "claude" {
			session = item
			break
		}
	}
	plan, err := s.PrepareDelete(ctx, []string{session.Key})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(session.Path)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, session.Path, string(data)+"{\"type\":\"custom-title\",\"customTitle\":\"更改后的标题\"}\n")
	result := s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Done) != 0 || len(result.Failed) == 0 {
		t.Fatalf("active file deleted: %+v", result)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(session.Path, old, old); err != nil {
		t.Fatal(err)
	}
	result = s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Done) != 0 || len(result.Failed) == 0 || !strings.Contains(result.Failed[0].Error, "已变化") {
		t.Fatalf("stale confirmation accepted: %+v", result)
	}
	if _, err := os.Stat(session.Path); err != nil {
		t.Fatal(err)
	}
}

func TestFileRestoreConflictPurgeAndPending(t *testing.T) {
	ctx := context.Background()
	s := demoService(t)
	var session Session
	for _, item := range s.View().Sessions {
		if item.Provider == "claude" {
			session = item
			break
		}
	}
	plan, err := s.PrepareDelete(ctx, []string{session.Key})
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
	writeFixture(t, session.Path, "replacement")
	if err := s.Restore(ctx, entry.Key); err == nil {
		t.Fatal("conflicting file replaced")
	}
	data, _ := os.ReadFile(session.Path)
	if string(data) != "replacement" {
		t.Fatal("conflicting data changed")
	}
	manifestPath := filepath.Join(entry.Root, ".session-desk-trash", entry.ID, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest TrashEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Status = "pending"
	if err := writeJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	if err := s.Purge(ctx, entry.Key); err == nil {
		t.Fatal("pending backup purged")
	}
	manifest.Status = "trashed"
	if err := writeJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	result = s.PurgeMany(ctx, []string{entry.Key, "invalid"})
	if len(result.Done) != 1 || len(result.Failed) != 1 {
		t.Fatal(result)
	}
	data, _ = os.ReadFile(session.Path)
	if string(data) != "replacement" {
		t.Fatal("purge modified live data")
	}
}

func TestOwnedPathsAndTrashSymlink(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"../escape", filepath.Join(root, "absolute")} {
		if _, err := ownedPath(root, relative); err == nil {
			t.Fatalf("accepted %s", relative)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".session-desk-trash")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if release, err := lockStore(root, "claude"); err == nil {
		release()
		t.Fatal("trash symlink accepted")
	}
	files, err := os.ReadDir(outside)
	if err != nil || len(files) != 0 {
		t.Fatal("outside store modified")
	}
}

func TestAntigravityCacheRoundTripAndMalformedDelete(t *testing.T) {
	field := func(number uint64, value []byte) []byte {
		out := binary.AppendUvarint(nil, number<<3|2)
		out = binary.AppendUvarint(out, uint64(len(value)))
		return append(out, value...)
	}
	first := field(1, field(1, []byte(testID)))
	otherID := "33333333-4444-4555-8666-777777777777"
	second := field(1, field(1, []byte(otherID)))
	unknown := field(9, []byte("future"))
	original := append(append(slices.Clone(first), second...), unknown...)
	remaining, removed, err := cutAgyCache(original, map[string]bool{testID: true})
	if err != nil || !slices.Equal(removed, first) || !slices.Equal(remaining, append(slices.Clone(second), unknown...)) {
		t.Fatalf("cache cut: %v", err)
	}
	restored, err := restoreAgyCache(remaining, removed)
	if err != nil || !slices.Equal(restored, append(slices.Clone(remaining), first...)) {
		t.Fatal(err)
	}
	s := demoService(t)
	ctx := context.Background()
	var session Session
	var source Source
	for _, item := range s.View().Sessions {
		if item.Provider == "agy" {
			session = item
			break
		}
	}
	_, source, err = s.Find(session.Key)
	if err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(source.AgyDir, "jetbox_summaries_proto.pb")
	if err := os.WriteFile(cachePath, []byte{10, 255}, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := s.PrepareDelete(ctx, []string{session.Key})
	if err != nil {
		t.Fatal(err)
	}
	result := s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Done) != 0 || len(result.Failed) == 0 {
		t.Fatal("malformed protobuf deleted")
	}
	view, err := s.Refresh(ctx)
	if err != nil || len(view.Sessions) != 16 {
		t.Fatalf("data changed: %d %v", len(view.Sessions), err)
	}
}

func TestOfflineDeletionAndLateChild(t *testing.T) {
	s := demoService(t)
	ctx := context.Background()
	key := "windows:local|opencode|ses_demoWindows"
	plan, err := s.PrepareDelete(ctx, []string{key})
	if err != nil {
		t.Fatal(err)
	}
	db := fixtureDB(t, filepath.Join(s.fixed[0].OpenCodeDir, "opencode.db"), "")
	at := time.Now().Add(-time.Hour).UnixMilli()
	if _, err := db.Exec(`INSERT INTO session_v2 VALUES ('ses_late','ses_demoWindows','新子会话','/other',?,?)`, at, at); err != nil {
		t.Fatal(err)
	}
	result := s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Done) != 0 || len(result.Failed) == 0 || !strings.Contains(result.Failed[0].Error, "关联子会话") {
		t.Fatal(result)
	}
	s.fixed[0].Running = false
	if _, err := s.PrepareDelete(ctx, []string{key}); err == nil {
		t.Fatal("offline deletion allowed")
	}
}

func TestCodexArchivedRelatedSessions(t *testing.T) {
	ctx := context.Background()
	s := demoService(t)
	source := s.fixed[0]
	var parent Session
	for _, session := range s.View().Sessions {
		if session.Provider == "codex" && session.SourceID == source.ID {
			parent = session
			break
		}
	}
	childID := "44444444-5555-4666-8777-888888888888"
	path := filepath.Join(source.CodexDir, "archived_sessions", "rollout-"+childID+".jsonl")
	writeFixture(t, path, `{"type":"session_meta","timestamp":"2020-01-01T00:00:00Z","payload":{"id":"`+childID+`","forked_from_id":"`+parent.ID+`","cwd":"/other/project"}}`+"\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	plan, err := s.PrepareDelete(ctx, []string{parent.Key})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Keys) != 2 || plan.Related != 1 {
		t.Fatalf("archived child missing: %+v", plan)
	}
	child, _, err := s.Find(source.ID + "|codex|" + childID)
	if err != nil || !child.Archived {
		t.Fatalf("archived metadata: %+v %v", child, err)
	}
	result := s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Failed) > 0 {
		t.Fatal(result)
	}
	entries, err := s.Trash(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("%+v %v", entries, err)
	}
	if err := s.Restore(ctx, entries[0].Key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyJSONDeleteRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := Source{ID: "local", Name: "local", Home: root, OpenCodeDir: filepath.Join(root, "opencode"), Running: true}
	storage := filepath.Join(source.OpenCodeDir, "storage")
	files := map[string]string{
		filepath.Join("session", "project", "ses_test.json"):  `{"id":"ses_test","title":"旧版会话","directory":"/work/project","time":{"created":1,"updated":2}}`,
		filepath.Join("message", "ses_test", "msg_test.json"): `{"id":"msg_test","role":"user"}`,
		filepath.Join("part", "msg_test", "prt_test.json"):    `{"type":"text","text":"旧版正文"}`,
	}
	old := time.Now().Add(-time.Hour)
	for file, raw := range files {
		path := filepath.Join(storage, file)
		writeFixture(t, path, raw)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(Options{ConfigPath: filepath.Join(root, "settings.json")})
	if err != nil {
		t.Fatal(err)
	}
	s.fixed = []Source{source}
	view, err := s.Refresh(ctx)
	if err != nil || len(view.Sessions) != 1 {
		t.Fatalf("%+v %v", view, err)
	}
	plan, err := s.PrepareDelete(ctx, []string{view.Sessions[0].Key})
	if err != nil {
		t.Fatal(err)
	}
	result := s.Delete(ctx, plan.Keys, plan.Revisions)
	if len(result.Failed) > 0 {
		t.Fatal(result)
	}
	for file := range files {
		if _, err := os.Stat(filepath.Join(storage, file)); !os.IsNotExist(err) {
			t.Fatalf("live file remains: %s %v", file, err)
		}
	}
	entries, err := s.Trash(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("%+v %v", entries, err)
	}
	if err := s.Restore(ctx, entries[0].Key); err != nil {
		t.Fatal(err)
	}
	for file, raw := range files {
		data, err := os.ReadFile(filepath.Join(storage, file))
		if err != nil || string(data) != raw {
			t.Fatalf("file changed: %s %v", file, err)
		}
	}
}

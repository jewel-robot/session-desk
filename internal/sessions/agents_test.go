package sessions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureDB(t *testing.T, path, schema string) *sql.DB {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenCodeFormatsAndMigration(t *testing.T) {
	for _, format := range []string{"v1", "v2", "partial", "completed"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			db := fixtureDB(t, filepath.Join(root, "opencode.db"), `
   CREATE TABLE session (id TEXT PRIMARY KEY, title TEXT, directory TEXT, parent_id TEXT, time_created INTEGER, time_updated INTEGER, time_archived INTEGER);
   CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT REFERENCES session(id) ON DELETE CASCADE, data TEXT);
   CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT REFERENCES message(id) ON DELETE CASCADE, data TEXT);
   CREATE TABLE session_v2 (id TEXT PRIMARY KEY, title TEXT, directory TEXT, parent_id TEXT, time_created INTEGER, time_updated INTEGER);
   CREATE TABLE session_message (id TEXT PRIMARY KEY, session_id TEXT REFERENCES session_v2(id) ON DELETE CASCADE, seq INTEGER, type TEXT, data TEXT);
   CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT);`)
			if format == "v1" {
				db.Exec(`DROP TABLE session_message; DROP TABLE session_v2`)
			}
			at := time.Now().Add(-time.Hour).UnixMilli()
			for _, id := range []string{"ses_shared", "ses_old"} {
				if _, err := db.Exec(`INSERT INTO session VALUES (?, '旧版会话', '/work/test', NULL, ?, ?, 0)`, id, at, at); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO message VALUES (?, ?, ?)`, "msg_"+id, id, `{"role":"assistant","modelID":"model-v1","tokens":{"input":100,"output":20,"reasoning":5,"cache":{"read":50,"write":10}}}`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO part VALUES (?, ?, ?)`, "prt_"+id, "msg_"+id, `{"type":"text","text":"v1 消息正文"}`); err != nil {
					t.Fatal(err)
				}
			}
			if format != "v1" {
				if _, err := db.Exec(`INSERT INTO session_v2 VALUES ('ses_shared','新版会话','/work/test',NULL,?,?)`, at, at); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO session_message VALUES ('msg_new','ses_shared',0,'user','{"text":"v2 提问"}'), ('msg_reply','ses_shared',1,'assistant','{"model":{"id":"model-v2"},"content":[{"type":"reasoning","text":"分析"},{"type":"text","text":"v2 消息正文"},{"type":"tool","tool":"read","state":{"input":{"path":"a"},"output":"读取完成"}}],"tokens":{"input":200,"output":30}}')`); err != nil {
					t.Fatal(err)
				}
			}
			if format == "partial" || format == "completed" {
				phase := "copying"
				if format == "completed" {
					phase = "completed"
				}
				if _, err := db.Exec(`INSERT INTO kv VALUES ('migration.v1-v2',?)`, `{"phase":"`+phase+`"}`); err != nil {
					t.Fatal(err)
				}
			}
			if format == "v2" {
				db.Exec(`DELETE FROM part; DELETE FROM message; DELETE FROM session`)
			}
			found, err := loadOpenCode(context.Background(), Source{ID: "local", OpenCodeDir: root})
			if err != nil {
				t.Fatal(err)
			}
			expected := 2
			if format == "v2" || format == "completed" {
				expected = 1
			}
			if len(found) != expected {
				t.Fatalf("sessions: %+v", found)
			}
			for _, session := range found {
				page, err := providerTranscript(context.Background(), session, 0)
				if err != nil || len(page.Parts) == 0 {
					t.Fatalf("transcript: %+v, %v", page, err)
				}
				if session.ID == "ses_shared" && format != "v1" {
					if session.Store != "opencode-v2" || session.Model != "model-v2" || session.Tokens.Input != 200 || len(page.Parts) != 5 {
						t.Fatalf("v2: %+v %+v", session, page)
					}
				} else if session.Tokens.Input != 160 || session.Tokens.Output != 25 {
					t.Fatalf("v1 usage: %+v", session)
				}
			}
		})
	}
}

func TestOpenCodeLegacyAndPathValidation(t *testing.T) {
	root := t.TempDir()
	storage := filepath.Join(root, "storage")
	summary := filepath.Join(storage, "session", "project", "ses_legacy.json")
	writeFixture(t, summary, `{"id":"ses_legacy","directory":"/work/legacy","time":{"created":1,"updated":2}}`)
	writeFixture(t, filepath.Join(storage, "message", "ses_legacy", "msg_1.json"), `{"id":"msg_1","role":"user","time":{"created":1}}`)
	writeFixture(t, filepath.Join(storage, "part", "msg_1", "prt_1.json"), `{"type":"text","text":"旧版提问"}`)
	message := filepath.Join(storage, "message", "ses_legacy", "msg_2.json")
	writeFixture(t, message, `{"id":"msg_2","role":"assistant","modelID":"legacy-model","time":{"created":2},"tokens":{"input":10,"output":20,"cache":{"read":5}}}`)
	writeFixture(t, filepath.Join(storage, "part", "msg_2", "prt_2.json"), `{"type":"text","text":"旧版回复"}`)
	sessions, err := loadOpenCode(context.Background(), Source{ID: "local", OpenCodeDir: root})
	if err != nil || len(sessions) != 1 {
		t.Fatalf("%+v %v", sessions, err)
	}
	session := sessions[0]
	if session.Title != "旧版提问" || session.Messages != 2 || session.Model != "legacy-model" || session.Tokens.Input != 15 || len(session.Paths) != 5 {
		t.Fatalf("%+v", session)
	}
	page, err := providerTranscript(context.Background(), session, 0)
	if err != nil || len(page.Parts) != 2 {
		t.Fatalf("%+v %v", page, err)
	}
	writeFixture(t, message, `{"id":"../../escape","role":"assistant"}`)
	if _, err := loadOpenCode(context.Background(), Source{ID: "local", OpenCodeDir: root}); err == nil {
		t.Fatal("invalid message path accepted")
	}
	writeFixture(t, summary, `{"id":"../../escape"}`)
	if _, err := loadOpenCode(context.Background(), Source{ID: "local", OpenCodeDir: root}); err == nil {
		t.Fatal("invalid session path accepted")
	}
}

func demoService(t *testing.T) *Service {
	t.Helper()
	s, err := New(Options{Demo: true, ConfigPath: filepath.Join(t.TempDir(), "settings.json")})
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.Refresh(context.Background())
	if err != nil || len(view.Warnings) != 0 || len(view.Sessions) != 16 {
		t.Fatalf("demo: %d, %v, %v", len(view.Sessions), view.Warnings, err)
	}
	return s
}

func TestAntigravitySummaryChunksAndWorkspace(t *testing.T) {
	s := demoService(t)
	for _, session := range s.View().Sessions {
		if session.Provider != "agy" {
			continue
		}
		page, err := s.Transcript(context.Background(), session.Key, 0)
		if err != nil || page.Parts == nil {
			t.Fatalf("%+v %v", page, err)
		}
		if session.Store == "agy-summary" {
			if session.Cwd != `C:\Work\personal-site` || len(page.Parts) != 0 {
				t.Fatalf("summary: %+v %+v", session, page)
			}
		} else {
			if session.Cwd != "/home/demo/projects/robot-navigation" || session.Messages != 2 || session.Tokens.Input != 3300 || len(page.Parts) != 3 || strings.Contains(fmt.Sprint(page.Parts), "正在整理") {
				t.Fatalf("chunks: %+v %+v", session, page)
			}
		}
	}
	if cwd := workspacePath("file:///home/demo/a%20b", Source{Kind: "wsl"}); cwd != "/home/demo/a b" {
		t.Fatal(cwd)
	}
	if cwd := workspacePath("file://server/share/a", Source{Kind: "windows"}); cwd != `\\server\share\a` {
		t.Fatal(cwd)
	}
}

func TestNewProviderResumePlans(t *testing.T) {
	for _, kind := range []string{"windows", "wsl"} {
		source := Source{Kind: kind, Distro: "Ubuntu-20.04", Home: "/home/demo", OpenCodeDir: "/tmp/open code", AgyDir: "/tmp/agy"}
		for _, provider := range []string{"opencode", "agy"} {
			id := testID
			command := "--conversation " + id
			if provider == "opencode" {
				id = "ses_test"
				command = "--session " + id
			}
			plan, err := resumePlan(Session{ID: id, Provider: provider, Cwd: "/work/test"}, source)
			if err != nil || !strings.Contains(plan.Command, provider) || !strings.Contains(plan.Command, command) {
				t.Fatalf("%+v %v", plan, err)
			}
			if provider == "opencode" && (!strings.Contains(plan.Command, "OPENCODE_DB") || !strings.Contains(plan.Command, "XDG_DATA_HOME")) {
				t.Fatal(plan.Command)
			}
		}
	}
}

func TestSummaryOnlyTranscriptJSON(t *testing.T) {
	page, err := providerTranscript(context.Background(), Session{Provider: "agy"}, 0)
	raw, _ := json.Marshal(page)
	if err != nil || !strings.Contains(string(raw), `"parts":[]`) {
		t.Fatalf("%s %v", raw, err)
	}
}

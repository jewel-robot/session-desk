package sessions

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

const testID = "11111111-2222-4333-8444-555555555555"

func writeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeSummaryAndContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), testID+".jsonl")
	writeFixture(t, path, `{"type":"user","sessionId":"`+testID+`","cwd":"C:\\Work\\中文项目","timestamp":"2026-10-29T12:00:00Z","message":{"content":"添加项目分组\n并保留环境"}}
{"type":"assistant","message":{"id":"msg1","model":"claude-test","content":[{"type":"thinking","thinking":"先确认边界"}],"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":50}}}
{"type":"assistant","message":{"id":"msg1","model":"claude-test","content":[{"type":"text","text":"处理完成"}],"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":50}}}
{"type":"assistant","message":{"id":"msg1","content":[{"type":"tool_use","name":"Read","input":{"path":"x.go"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"读取成功"}]}]}}
{"type":"custom-title","customTitle":"自定义标题"}
`)
	info, _ := os.Stat(path)
	s, err := readSummary(context.Background(), Source{ID: "windows:local"}, "claude", path, testID, info)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "自定义标题" || s.Messages != 2 || s.Project != "中文项目" || s.Model != "claude-test" {
		t.Fatalf("unexpected summary: %+v", s)
	}
	if s.Tokens.Input != 150 || s.Tokens.Output != 20 || s.Tokens.Cached != 50 {
		t.Fatalf("repeated reply usage counted twice or lost: %+v", s.Tokens)
	}
	transcript, err := readTranscript(context.Background(), s, 0)
	if err != nil || len(transcript.Parts) != 5 {
		t.Fatalf("transcript: %+v, %v", transcript, err)
	}
	if transcript.Parts[4].Kind != "tool_result" || transcript.Parts[4].Text != "读取成功" {
		t.Fatalf("tool result: %+v", transcript.Parts[4])
	}
}

func TestCodexEventAndResponseDeduplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-"+testID+".jsonl")
	writeFixture(t, path, `{"type":"session_meta","timestamp":"2026-10-29T12:00:00Z","payload":{"id":"`+testID+`","cwd":"/home/test/project","git":{"branch":"main"}}}
{"type":"turn_context","payload":{"model":"test-model"}}
{"type":"event_msg","payload":{"type":"user_message","message":"解释解析流程"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"解释解析流程"}]}}
{"type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"先检查记录格式"}]}}
{"type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"pwd\"}"}}
{"type":"response_item","payload":{"type":"function_call_output","output":"/home/test/project"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"这是解析流程"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"这是解析流程"}]}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"output_tokens":200,"cached_input_tokens":800}}}}
`)
	info, _ := os.Stat(path)
	s, err := readSummary(context.Background(), Source{ID: "wsl:Ubuntu"}, "codex", path, "rollout-name", info)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != testID || s.Title != "解释解析流程" || s.Messages != 2 || s.Tokens.Input != 1000 || !s.ResponseMessages {
		t.Fatalf("summary: %+v", s)
	}
	transcript, err := readTranscript(context.Background(), s, 0)
	if err != nil || len(transcript.Parts) != 5 {
		t.Fatalf("duplicate events in transcript: %+v, %v", transcript, err)
	}
}

func TestTranscriptPaginationCRLFAndIncompleteTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	var lines strings.Builder
	for i := 0; i < 205; i++ {
		data, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]string{"content": "第" + strings.Repeat("一", i+1) + "条消息"}})
		lines.Write(data)
		lines.WriteString("\r\n")
	}
	lines.WriteString(`{"type":"user","message":`)
	writeFixture(t, path, lines.String())
	s := Session{Provider: "claude", Path: path}
	var cursor int64
	var count, pages int
	for {
		page, err := readTranscript(context.Background(), s, cursor)
		if err != nil {
			t.Fatal(err)
		}
		count += len(page.Parts)
		pages++
		if !page.HasMore {
			break
		}
		if page.NextCursor <= cursor || pages > 5 {
			t.Fatal("pagination did not advance")
		}
		cursor = page.NextCursor
	}
	if count != 205 {
		t.Fatalf("lost records across CRLF pages: %d", count)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{\"content\":\"尾部完成\"}}\n")
	f.Close()
	page, err := readTranscript(context.Background(), s, cursor)
	if err != nil || len(page.Parts) != 1 || page.Parts[0].Text != "尾部完成" || page.HasMore {
		t.Fatalf("completed tail was not retained: %+v, %v", page, err)
	}
}

func TestSourceIdentityCacheAndReadOnlyScan(t *testing.T) {
	root := t.TempDir()
	service, err := New(Options{ConfigPath: filepath.Join(root, "settings.json")})
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"windows:local", "wsl:Ubuntu"} {
		home := filepath.Join(root, id[:3])
		source := Source{ID: id, Name: id, Home: home, ClaudeDir: filepath.Join(home, ".claude"), CodexDir: filepath.Join(home, ".codex"), Running: true}
		path := filepath.Join(source.ClaudeDir, "projects", "project", testID+".jsonl")
		writeFixture(t, path, `{"type":"user","sessionId":"`+testID+`","cwd":"/same/project","message":{"content":"会话 `+string(rune('A'+i))+`"}}`+"\n")
		service.fixed = append(service.fixed, source)
	}
	before, _ := os.ReadFile(filepath.Join(service.fixed[0].ClaudeDir, "projects", "project", testID+".jsonl"))
	view, err := service.Refresh(context.Background())
	if err != nil || len(view.Sessions) != 2 || view.Sessions[0].Key == view.Sessions[1].Key {
		t.Fatalf("cross-source collision: %+v, %v", view, err)
	}
	after, _ := os.ReadFile(filepath.Join(service.fixed[0].ClaudeDir, "projects", "project", testID+".jsonl"))
	if string(before) != string(after) {
		t.Fatal("browsing changed source data")
	}
	service.fixed[1].Running = false
	if err := os.RemoveAll(service.fixed[1].Home); err != nil {
		t.Fatal(err)
	}
	view, err = service.Refresh(context.Background())
	if err != nil || len(view.Sessions) != 2 {
		t.Fatalf("stopped source lost cached sessions: %+v %v", view, err)
	}
	for _, session := range view.Sessions {
		if session.SourceID == "wsl:Ubuntu" {
			if _, err := service.Transcript(context.Background(), session.Key, 0); err == nil {
				t.Fatal("opened stopped source")
			}
		}
	}
}

func TestSearchMatchesBodyAndMetadata(t *testing.T) {
	service, err := New(Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := service.Search(context.Background(), "发生变化的文件", "codex", "wsl:Ubuntu-20.04")
	if len(result.Hits) != 3 || result.Hits[0].Excerpt == "" || len(result.Errors) > 0 {
		t.Fatalf("body search: %+v", result)
	}
	result = service.Search(context.Background(), "personal-site", "", "windows:local")
	if len(result.Hits) != 4 {
		t.Fatalf("metadata search: %+v", result)
	}
}

func TestDuplicateRolloutCacheRetainsEachFile(t *testing.T) {
	root := t.TempDir()
	service, err := New(Options{ConfigPath: filepath.Join(root, "settings.json")})
	if err != nil {
		t.Fatal(err)
	}
	source := Source{ID: "local", Name: "local", ClaudeDir: filepath.Join(root, ".claude"), CodexDir: filepath.Join(root, ".codex"), Running: true}
	service.fixed = []Source{source}
	for _, day := range []string{"28", "29"} {
		path := filepath.Join(source.CodexDir, "sessions", "2026", "10", day, "rollout.jsonl")
		writeFixture(t, path, `{"type":"session_meta","timestamp":"2026-10-`+day+`T12:00:00Z","payload":{"id":"`+testID+`","cwd":"/work/project"}}`+"\n")
	}
	view, err := service.Refresh(context.Background())
	if err != nil || len(view.Sessions) != 1 || filepath.Base(filepath.Dir(view.Sessions[0].Path)) != "29" {
		t.Fatalf("latest rollout not selected: %+v %v", view, err)
	}
	if err := os.Remove(view.Sessions[0].Path); err != nil {
		t.Fatal(err)
	}
	view, err = service.Refresh(context.Background())
	if err != nil || len(view.Sessions) != 1 || filepath.Base(filepath.Dir(view.Sessions[0].Path)) != "28" {
		t.Fatalf("cache kept a deleted file instead of the remaining rollout: %+v %v", view, err)
	}
}

func TestTerminalPrefersPowerShellCoreAndFallsBackToWindowsPowerShell(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if arguments := terminalArguments("Write-Output test"); arguments[5] != "powershell.exe" {
		t.Fatalf("expected Windows PowerShell fallback: %q", arguments)
	}
	pwsh := filepath.Join(bin, "pwsh.exe")
	if err := os.WriteFile(pwsh, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	arguments := terminalArguments("Write-Output test")
	if arguments[5] != pwsh || strings.Contains(strings.Join(arguments, " "), "-NoProfile") {
		t.Fatalf("expected PowerShell Core with the user's profile: %q", arguments)
	}
}

func TestResumePlanQuotesPathsAndUsesEncodedTerminalArguments(t *testing.T) {
	s := Session{ID: testID, Provider: "codex", Cwd: `C:\Work\it's; $(whoami) 中文`, Project: "unsafe;title"}
	plan, err := resumePlan(s, Source{Kind: "windows"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Command, "it''s;") || strings.Contains(strings.Join(plan.Arguments, " "), s.Cwd) {
		t.Fatalf("unquoted path: %+v", plan)
	}
	encoded, err := base64.StdEncoding.DecodeString(plan.Arguments[len(plan.Arguments)-1])
	if err != nil || decodeWSL(encoded) != plan.Command {
		t.Fatalf("encoded command corrupted: %v", err)
	}
	s.Cwd = "/home/test/it's; $(touch x)/中文"
	plan, err = resumePlan(s, Source{Kind: "wsl", Distro: "Ubuntu-20.04"})
	if err != nil || !strings.Contains(plan.Command, "--distribution 'Ubuntu-20.04'") || !strings.Contains(plan.Command, "it''s;") {
		t.Fatalf("WSL command: %+v, %v", plan, err)
	}
	s.ID = "id; malicious"
	if _, err := resumePlan(s, Source{Kind: "wsl", Distro: "Ubuntu"}); err == nil {
		t.Fatal("invalid session ID accepted")
	}
}

func TestResumeUsesConfiguredStore(t *testing.T) {
	session := Session{ID: testID, Provider: "codex", Cwd: `C:\Work\project`}
	plan, err := resumePlan(session, Source{Kind: "windows", CodexDir: `C:\Stores\it's code`})
	if err != nil || !strings.Contains(plan.Command, "$env:CODEX_HOME = 'C:\\Stores\\it''s code'") {
		t.Fatalf("Windows store was not applied: %+v, %v", plan, err)
	}
	session.Cwd = "/home/test/project"
	plan, err = resumePlan(session, Source{Kind: "wsl", Distro: "Ubuntu", CodexDir: "/home/test/it's code"})
	if err != nil || !strings.Contains(plan.Command, "SESSION_DESK_DATA_HOME=/home/test/it''s code") || !strings.Contains(plan.Command, "CODEX_HOME=${SESSION_DESK_DATA_HOME}") {
		t.Fatalf("WSL store was not applied: %+v, %v", plan, err)
	}
}

func TestWSLListUTF16AndUTF8(t *testing.T) {
	text := "Ubuntu-20.04\r\n中文环境\r\ndocker-desktop\r\n"
	units := utf16.Encode([]rune(text))
	data := []byte{0xff, 0xfe}
	for _, unit := range units {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}
	for _, encoded := range [][]byte{data, data[2:], []byte(text), utf16LE("中文环境\r\nUbuntu\r\n")} {
		names := distroNames(encoded)
		if len(names) != 2 || names[0] != "中文环境" && names[1] != "中文环境" {
			t.Fatalf("bad WSL list: %q", names)
		}
	}
}

package sessions

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Demo files use the same scanners and parsers as real sessions.
func makeDemo(root string) ([]Source, error) {
	sources := []Source{
		{ID: "windows:local", Name: "Windows", Kind: "windows", Home: `C:\Users\demo`, Running: true},
		{ID: "wsl:Ubuntu-20.04", Name: "Ubuntu-20.04", Kind: "wsl", Distro: "Ubuntu-20.04", Home: "/home/demo", Running: true},
	}
	for i := range sources {
		sources[i].ClaudeDir = filepath.Join(root, fmt.Sprint(i), ".claude")
		sources[i].CodexDir = filepath.Join(root, fmt.Sprint(i), ".codex")
		sources[i].OpenCodeDir = filepath.Join(root, fmt.Sprint(i), "opencode")
		sources[i].AgyDir = filepath.Join(root, fmt.Sprint(i), "antigravity-cli")
	}
	items := []struct {
		source   int
		provider string
		project  string
		cwd      string
		title    string
		minutes  int
	}{
		{0, "claude", "session-desk", `C:\Work\session-desk`, "为会话列表添加项目分组与环境筛选", 8},
		{0, "codex", "session-desk", `C:\Work\session-desk`, "实现 Windows Terminal 会话续接", 42},
		{0, "claude", "session-desk", `C:\Work\session-desk`, "梳理桌面应用的数据源配置", 130},
		{1, "codex", "atlas-api", "/home/demo/projects/atlas-api", "优化会话索引的增量读取性能", 25},
		{1, "claude", "atlas-api", "/home/demo/projects/atlas-api", "检查 API 分页与错误处理", 68},
		{1, "codex", "atlas-api", "/home/demo/projects/atlas-api", "为项目列表补充集成测试", 280},
		{0, "claude", "personal-site", `C:\Work\personal-site`, "调整文章页面的排版和深色主题", 1520},
		{0, "codex", "personal-site", `C:\Work\personal-site`, "修复移动端导航的键盘焦点", 1700},
		{1, "claude", "robot-navigation", "/home/demo/projects/robot-navigation", "分析轨迹规划中的速度约束", 2950},
		{1, "codex", "robot-navigation", "/home/demo/projects/robot-navigation", "整理规划模块的配置参数", 3200},
	}
	answer := "已经把项目目录和会话来源整理成独立的数据模型。\n\n### 实现方式\n\n- 按环境和工作目录组织会话，保留各自的阅读位置。\n- 会话摘要先显示，展开后按需读取正文。\n- 刷新只重新解析发生变化的文件。\n\n```go\nkey := source.ID + \"|\" + session.Provider + \"|\" + session.ID\n```\n\n验证通过后，可以继续接入 Windows Terminal。"
	for i, item := range items {
		source := sources[item.source]
		id := fmt.Sprintf("11111111-2222-4333-8444-%012d", i+1)
		now := time.Now().Add(-time.Duration(item.minutes) * time.Minute)
		timestamp := now.Format(time.RFC3339Nano)
		var records []any
		var path string
		if item.provider == "claude" {
			path = filepath.Join(source.ClaudeDir, "projects", item.project, id+".jsonl")
			records = []any{
				map[string]any{"type": "user", "sessionId": id, "cwd": item.cwd, "gitBranch": "main", "timestamp": timestamp, "message": map[string]any{"role": "user", "content": item.title}},
				map[string]any{"type": "assistant", "timestamp": timestamp, "message": map[string]any{"id": "msg_1", "role": "assistant", "model": "claude-sonnet-4-6", "content": []any{map[string]any{"type": "thinking", "thinking": "先确认数据源和会话的边界，再实现项目分组与正文的按需加载。"}, map[string]any{"type": "tool_use", "name": "Read", "input": map[string]string{"file_path": "internal/sessions/service.go"}}}, "usage": map[string]int{"input_tokens": 12840 + i*300, "output_tokens": 2100 + i*70, "cache_read_input_tokens": 6400}}},
				map[string]any{"type": "user", "timestamp": timestamp, "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "content": "文件读取完成，已定位到项目分组与数据源处理逻辑。"}}}},
				map[string]any{"type": "assistant", "timestamp": timestamp, "message": map[string]any{"id": "msg_2", "role": "assistant", "model": "claude-sonnet-4-6", "content": []any{map[string]any{"type": "text", "text": answer}}, "usage": map[string]int{"input_tokens": 2800, "output_tokens": 940}}},
			}
		} else {
			path = filepath.Join(source.CodexDir, "sessions", "2026", "10", "29", "rollout-"+id+".jsonl")
			records = []any{
				map[string]any{"type": "session_meta", "timestamp": timestamp, "payload": map[string]any{"id": id, "cwd": item.cwd, "git": map[string]string{"branch": "feat/session-browser"}}},
				map[string]any{"type": "turn_context", "timestamp": timestamp, "payload": map[string]string{"model": "gpt-5.3-codex", "cwd": item.cwd}},
				map[string]any{"type": "event_msg", "timestamp": timestamp, "payload": map[string]string{"type": "user_message", "message": item.title}},
				map[string]any{"type": "response_item", "timestamp": timestamp, "payload": map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": item.title}}}},
				map[string]any{"type": "response_item", "timestamp": timestamp, "payload": map[string]any{"type": "reasoning", "summary": []any{map[string]string{"type": "summary_text", "text": "检查文件格式，确认会话来源，再补充分页边界的验证。"}}}},
				map[string]any{"type": "response_item", "timestamp": timestamp, "payload": map[string]any{"type": "function_call", "name": "exec_command", "arguments": `{"cmd":"go test ./internal/sessions"}`}},
				map[string]any{"type": "response_item", "timestamp": timestamp, "payload": map[string]any{"type": "function_call_output", "output": "ok sessiondesk/internal/sessions 0.024s"}},
				map[string]any{"type": "response_item", "timestamp": timestamp, "payload": map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": answer}}}},
				map[string]any{"type": "event_msg", "timestamp": timestamp, "payload": map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": map[string]int{"input_tokens": 18200 + i*600, "output_tokens": 3500, "cached_input_tokens": 9300}}}},
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		encoder := json.NewEncoder(f)
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				f.Close()
				return nil, err
			}
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}

	for i, source := range sources {
		if err := os.MkdirAll(source.OpenCodeDir, 0o700); err != nil {
			return nil, err
		}
		db, err := sql.Open("sqlite", filepath.Join(source.OpenCodeDir, "opencode.db"))
		if err != nil {
			return nil, err
		}
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS session_v2 (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES session_v2(id), title TEXT, directory TEXT, time_created INTEGER, time_updated INTEGER);
   CREATE TABLE IF NOT EXISTS session_message (id TEXT PRIMARY KEY, session_id TEXT REFERENCES session_v2(id) ON DELETE CASCADE, seq INTEGER, type TEXT, data TEXT);`)
		if err != nil {
			db.Close()
			return nil, err
		}
		parentID, childID := "ses_demoWindows", "ses_demoChild"
		parentCwd, childCwd := `C:\Work\session-desk`, `C:\Work\personal-site`
		if i == 1 {
			parentID, childID = "ses_demoWSL", "ses_demoWorker"
			parentCwd, childCwd = "/home/demo/projects/atlas-api", "/home/demo/projects/robot-navigation"
		}
		for n, id := range []string{parentID, childID} {
			title, cwd := "接入 OpenCode 会话与消息数据库", parentCwd
			var parent any
			if n == 1 {
				title, cwd, parent = "验证 OpenCode 关联子会话", childCwd, parentID
			}
			at := time.Now().Add(-time.Duration(95+n*20) * time.Minute).UnixMilli()
			if _, err := db.Exec(`INSERT OR REPLACE INTO session_v2 VALUES (?, ?, ?, ?, ?, ?)`, id, parent, title, cwd, at, at); err != nil {
				db.Close()
				return nil, err
			}
			for seq, role := range []string{"user", "assistant"} {
				message := map[string]any{"text": title}
				if role == "assistant" {
					message = map[string]any{"model": map[string]string{"id": "claude-sonnet-4-6"}, "content": []any{map[string]string{"type": "reasoning", "text": "核对会话和消息之间的关联。"}, map[string]string{"type": "text", "text": "已读取 OpenCode 数据库，消息按原始顺序呈现。"}}, "tokens": map[string]any{"input": 1200, "output": 320, "cache": map[string]int{"read": 500}}}
				}
				raw, err := json.Marshal(message)
				if err != nil {
					db.Close()
					return nil, err
				}
				if _, err := db.Exec(`INSERT OR REPLACE INTO session_message VALUES (?, ?, ?, ?, ?)`, fmt.Sprintf("msg_%s%d", id, seq), id, seq, role, string(raw)); err != nil {
					db.Close()
					return nil, err
				}
			}
		}
		if err := db.Close(); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(source.AgyDir, 0o700); err != nil {
			return nil, err
		}
		db, err = sql.Open("sqlite", filepath.Join(source.AgyDir, "conversation_summaries.db"))
		if err != nil {
			return nil, err
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS conversation_summaries (conversation_id TEXT PRIMARY KEY, title TEXT, workspace_uris TEXT, last_modified_time TEXT, parent_conversation_id TEXT)`); err != nil {
			db.Close()
			return nil, err
		}
		id := fmt.Sprintf("22222222-3333-4444-8555-%012d", i+1)
		title, workspace := "整理 Antigravity CLI 项目摘要", "file:///C:/Work/personal-site"
		if i == 1 {
			title, workspace = "分析导航模块的会话日志", "file:///home/demo/projects/robot-navigation"
		}
		uris, _ := json.Marshal([]string{workspace})
		timestamp := time.Now().Add(-3 * time.Hour).Format(time.RFC3339Nano)
		if _, err := db.Exec(`INSERT OR REPLACE INTO conversation_summaries VALUES (?, ?, ?, ?, NULL)`, id, title, string(uris), timestamp); err != nil {
			db.Close()
			return nil, err
		}
		if err := db.Close(); err != nil {
			return nil, err
		}
		// Keep the original protobuf map format used by Antigravity's summary cache.
		key := append(binary.AppendUvarint(nil, 10), byte(len(id)))
		key = append(key, id...)
		cache := append(binary.AppendUvarint(nil, 10), byte(len(key)))
		cache = append(cache, key...)
		if err := os.WriteFile(filepath.Join(source.AgyDir, "jetbox_summaries_proto.pb"), cache, 0o600); err != nil {
			return nil, err
		}
		if i == 1 {
			logDir := filepath.Join(source.AgyDir, "brain", id, ".system_generated", "logs", "chunks", "transcript_full")
			if err := os.MkdirAll(logDir, 0o700); err != nil {
				return nil, err
			}
			steps := [][]map[string]any{
				{{"step_index": 0, "source": "USER_EXPLICIT", "type": "USER_INPUT", "created_at": timestamp, "content": "<USER_REQUEST>分析导航模块的会话日志</USER_REQUEST>"}, {"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "created_at": timestamp, "content": "正在整理"}},
				{{"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "created_at": timestamp, "content": "日志已按步骤归并，重复步骤已更新。", "thinking": "按 step_index 合并分片中的重复记录。", "input_tokens": 2500, "cache_read_tokens": 800, "output_tokens": 450}},
			}
			for n, chunk := range steps {
				var data []byte
				for _, step := range chunk {
					raw, err := json.Marshal(step)
					if err != nil {
						return nil, err
					}
					data = append(append(data, raw...), '\n')
				}
				if err := os.WriteFile(filepath.Join(logDir, fmt.Sprintf("%04d.jsonl", n)), data, 0o600); err != nil {
					return nil, err
				}
			}
		}
	}
	old := time.Now().Add(-6 * time.Hour)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		return os.Chtimes(path, old, old)
	}); err != nil {
		return nil, err
	}
	return sources, nil
}

package sessions

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type record struct {
	Raw         json.RawMessage `json:"-"`
	Type        string          `json:"type"`
	Timestamp   string          `json:"timestamp"`
	SessionID   string          `json:"sessionId"`
	Cwd         string          `json:"cwd"`
	Branch      string          `json:"gitBranch"`
	CustomTitle string          `json:"customTitle"`
	AITitle     string          `json:"aiTitle"`
	Summary     string          `json:"summary"`
	ID          string          `json:"id"`
	ThreadName  string          `json:"thread_name"`
	Message     json.RawMessage `json:"message"`
	Payload     json.RawMessage `json:"payload"`
}

type message struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		Input  int64 `json:"input_tokens"`
		Output int64 `json:"output_tokens"`
		Cached int64 `json:"cache_read_input_tokens"`
		Write  int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

type block struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
}

type codexPayload struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	SessionID  string          `json:"session_id"`
	Cwd        string          `json:"cwd"`
	Model      string          `json:"model"`
	Role       string          `json:"role"`
	Message    string          `json:"message"`
	Text       string          `json:"text"`
	Name       string          `json:"name"`
	Arguments  string          `json:"arguments"`
	Input      json.RawMessage `json:"input"`
	Output     json.RawMessage `json:"output"`
	Content    json.RawMessage `json:"content"`
	Summary    []block         `json:"summary"`
	ParentID   string          `json:"parent_thread_id"`
	ForkedFrom string          `json:"forked_from_id"`
	Git        struct {
		Branch string `json:"branch"`
	} `json:"git"`
	Info struct {
		Total struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
			Cached int64 `json:"cached_input_tokens"`
		} `json:"total_token_usage"`
	} `json:"info"`
}

// readRecords retains an unfinished final line so a growing JSONL can be read again.
func readRecords(ctx context.Context, path string, start int64, visit func(record, int64) bool) (int64, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return start, 0, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return start, 0, err
	}
	if start < 0 || start > stat.Size() {
		return start, 0, errors.New("会话文件已变化，请重新打开")
	}
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return start, 0, err
	}
	reader := bufio.NewReaderSize(f, 64*1024)
	offset, invalid := start, 0
	for {
		if err := ctx.Err(); err != nil {
			return offset, invalid, err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 32*1024*1024 {
			return offset, invalid, errors.New("单条会话记录超过 32 MiB")
		}
		if len(line) == 0 {
			if readErr == io.EOF {
				return offset, invalid, nil
			}
			return offset, invalid, readErr
		}
		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			if readErr == io.EOF {
				return offset, invalid, nil
			}
			invalid++
			offset += int64(len(line))
			continue
		}
		offset += int64(len(line))
		r.Raw = line
		if !visit(r, offset) {
			return offset, invalid, nil
		}
		if readErr == io.EOF {
			return offset, invalid, nil
		}
		if readErr != nil {
			return offset, invalid, readErr
		}
	}
}

func textContent(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" || b.Type == "input_text" || b.Type == "output_text" || b.Type == "summary_text" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func contextText(text string) bool {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"<environment_context>", "<permissions instructions>", "<skills_instructions>", "<INSTRUCTIONS>", "# AGENTS.md instructions"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func readSummary(ctx context.Context, source Source, provider, path, id string, stat os.FileInfo) (Session, error) {
	s := Session{ID: id, SourceID: source.ID, Provider: provider, Path: path, Size: stat.Size(), UpdatedAt: stat.ModTime()}
	s.Revision = fmt.Sprintf("%d:%d", stat.Size(), stat.ModTime().UnixNano())
	var first, eventFirst, custom string
	var eventUsers, eventAssistants, responseUsers, responseAssistants int
	usage := map[string]Tokens{}
	seen := map[string]bool{}
	_, invalid, err := readRecords(ctx, path, 0, func(r record, offset int64) bool {
		if ts, err := time.Parse(time.RFC3339Nano, r.Timestamp); err == nil {
			if s.StartedAt.IsZero() {
				s.StartedAt = ts
			}
			s.UpdatedAt = ts
		}
		if s.Cwd == "" && r.Cwd != "" {
			s.Cwd = r.Cwd
		}
		if r.Branch != "" {
			s.Branch = r.Branch
		}
		if provider == "claude" {
			if r.SessionID != "" {
				s.ID = r.SessionID
			}
			if r.Type == "custom-title" && r.CustomTitle != "" {
				custom = r.CustomTitle
			}
			if r.Type == "ai-title" && r.AITitle != "" {
				s.Title = r.AITitle
			}
			if r.Type != "user" && r.Type != "assistant" {
				return true
			}
			var m message
			if json.Unmarshal(r.Message, &m) != nil {
				return true
			}
			text := strings.TrimSpace(textContent(m.Content))
			if r.Type == "user" && text != "" && !contextText(text) {
				s.Messages++
				if first == "" {
					first = text
				}
			}
			if r.Type == "assistant" {
				key := m.ID
				if key == "" {
					key = fmt.Sprint(offset)
				}
				if !seen[key] {
					s.Messages++
					seen[key] = true
				}
				if m.Usage != nil {
					usage[key] = Tokens{Input: m.Usage.Input + m.Usage.Write + m.Usage.Cached, Output: m.Usage.Output, Cached: m.Usage.Cached}
				}
				if m.Model != "" {
					s.Model = m.Model
				}
			}
			return true
		}
		var p codexPayload
		if json.Unmarshal(r.Payload, &p) != nil {
			return true
		}
		switch r.Type {
		case "session_meta":
			if p.ID != "" {
				s.ID = p.ID
			} else if p.SessionID != "" {
				s.ID = p.SessionID
			}
			s.Cwd, s.Branch = p.Cwd, p.Git.Branch
			s.ParentID = p.ParentID
			if s.ParentID == "" {
				s.ParentID = p.ForkedFrom
			}
		case "turn_context":
			if p.Cwd != "" {
				s.Cwd = p.Cwd
			}
			if p.Model != "" {
				s.Model = p.Model
			}
		case "event_msg":
			switch p.Type {
			case "user_message":
				if text := strings.TrimSpace(p.Message); text != "" && !contextText(text) {
					eventUsers++
					if eventFirst == "" {
						eventFirst = text
					}
				}
			case "agent_message":
				eventAssistants++
			case "token_count":
				s.Tokens = Tokens{Input: p.Info.Total.Input, Output: p.Info.Total.Output, Cached: p.Info.Total.Cached}
			}
		case "response_item":
			if p.Type == "message" {
				text := strings.TrimSpace(textContent(p.Content))
				if p.Role == "user" && text != "" && !contextText(text) {
					responseUsers++
					if first == "" {
						first = text
					}
				}
				if p.Role == "assistant" {
					responseAssistants++
				}
				s.ResponseMessages = true
			}
		}
		return true
	})
	if err != nil {
		return s, err
	}
	if invalid > 0 {
		s.Warning = fmt.Sprintf("已跳过 %d 条无法解析的记录", invalid)
	}
	if provider == "codex" {
		if eventUsers > 0 {
			responseUsers, first = eventUsers, eventFirst
		}
		if eventAssistants > 0 {
			responseAssistants = eventAssistants
		}
		s.Messages = responseUsers + responseAssistants
	} else {
		for _, t := range usage {
			s.Tokens.Input += t.Input
			s.Tokens.Output += t.Output
			s.Tokens.Cached += t.Cached
		}
	}
	if custom != "" {
		s.Title = custom
	}
	if s.Title == "" {
		s.Title = first
	}
	s.Title = strings.Join(strings.Fields(s.Title), " ")
	if title := []rune(s.Title); len(title) > 160 {
		s.Title = string(title[:160]) + "…"
	}
	if s.Title == "" {
		s.Title = "未命名会话"
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = s.UpdatedAt
	}
	s.Key = s.SourceID + "|" + s.Provider + "|" + s.ID
	s.Project = projectName(s.Cwd)
	return s, nil
}

func recordParts(r record, s Session) []Part {
	var parts []Part
	if s.Provider == "claude" {
		if r.Type != "user" && r.Type != "assistant" {
			return parts
		}
		var m message
		if json.Unmarshal(r.Message, &m) != nil {
			return parts
		}
		var text string
		if json.Unmarshal(m.Content, &text) == nil {
			kind := "text"
			if contextText(text) {
				kind = "context"
			}
			return []Part{{Role: r.Type, Kind: kind, Text: text, Model: m.Model}}
		}
		var blocks []block
		if json.Unmarshal(m.Content, &blocks) != nil {
			return parts
		}
		for _, b := range blocks {
			p := Part{Role: r.Type, Kind: b.Type, Name: b.Name, Model: m.Model}
			switch b.Type {
			case "text":
				p.Text = b.Text
				if contextText(p.Text) {
					p.Kind = "context"
				}
			case "thinking":
				p.Text = b.Thinking
			case "tool_use":
				p.Text = prettyJSON(b.Input)
			case "tool_result":
				p.Role = "tool"
				p.Text = textContent(b.Content)
				if p.Text == "" && len(b.Content) > 0 {
					p.Text = prettyJSON(b.Content)
				}
			case "image":
				p.Text = "图片"
			default:
				continue
			}
			if strings.TrimSpace(p.Text) != "" {
				parts = append(parts, p)
			}
		}
		return parts
	}
	var p codexPayload
	if json.Unmarshal(r.Payload, &p) != nil {
		return parts
	}
	if r.Type == "event_msg" && !s.ResponseMessages {
		if p.Type == "user_message" {
			return []Part{{Role: "user", Kind: "text", Text: p.Message}}
		}
		if p.Type == "agent_message" {
			return []Part{{Role: "assistant", Kind: "text", Text: p.Message}}
		}
	}
	if r.Type != "response_item" {
		return parts
	}
	switch p.Type {
	case "message":
		text := textContent(p.Content)
		if text != "" {
			kind := "text"
			if p.Role == "developer" || p.Role == "system" || contextText(text) {
				kind = "context"
			}
			parts = append(parts, Part{Role: p.Role, Kind: kind, Text: text})
		}
	case "reasoning":
		for _, b := range p.Summary {
			if b.Text != "" {
				parts = append(parts, Part{Role: "assistant", Kind: "thinking", Text: b.Text})
			}
		}
	case "function_call", "custom_tool_call":
		text := p.Arguments
		if text == "" {
			text = textContent(p.Input)
			if text == "" {
				text = prettyJSON(p.Input)
			}
		} else if json.Valid([]byte(text)) {
			text = prettyJSON([]byte(text))
		}
		parts = append(parts, Part{Role: "assistant", Kind: "tool_use", Name: p.Name, Text: text})
	case "function_call_output", "custom_tool_call_output":
		text := textContent(p.Output)
		if text == "" {
			text = prettyJSON(p.Output)
		}
		parts = append(parts, Part{Role: "tool", Kind: "tool_result", Text: text})
	}
	return parts
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	text, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(text)
}

func readTranscript(ctx context.Context, s Session, cursor int64) (Transcript, error) {
	out := Transcript{Parts: []Part{}}
	var size int
	next, _, err := readRecords(ctx, s.Path, cursor, func(r record, _ int64) bool {
		for _, p := range recordParts(r, s) {
			out.Parts = append(out.Parts, p)
			size += len(p.Text)
		}
		return len(out.Parts) < 100 && size < 1024*1024
	})
	if err != nil {
		return out, err
	}
	stat, err := os.Stat(s.Path)
	if err != nil {
		return out, err
	}
	out.NextCursor, out.HasMore = next, next > cursor && next < stat.Size()
	return out, nil
}

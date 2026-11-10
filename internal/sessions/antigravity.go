package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type agyStep struct {
	Index     *int   `json:"step_index"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
	Content   string `json:"content"`
	Thinking  string `json:"thinking"`
	Input     int64  `json:"input_tokens"`
	Cached    int64  `json:"cache_read_tokens"`
	Output    int64  `json:"output_tokens"`
	ToolCalls []struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"tool_calls"`
}

func agyRequest(text string) string {
	_, rest, found := strings.Cut(text, "<USER_REQUEST>")
	if !found {
		return strings.TrimSpace(text)
	}
	request, _, _ := strings.Cut(rest, "</USER_REQUEST>")
	return strings.TrimSpace(request)
}

func workspacePath(uri string, source Source) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if source.Kind == "windows" {
		if u.Host != "" && u.Host != "localhost" {
			return `\\` + u.Host + strings.ReplaceAll(p, "/", `\`)
		}
		if len(p) > 2 && p[0] == '/' && p[2] == ':' {
			p = strings.ToUpper(p[1:2]) + p[2:]
		}
		return strings.ReplaceAll(p, "/", `\`)
	}
	if u.Host != "" && u.Host != "localhost" {
		return ""
	}
	return p
}

func parseAgyTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

func agyTranscriptPaths(root, id string) ([]string, error) {
	logs := filepath.Join(root, "brain", id, ".system_generated", "logs")
	for _, base := range []string{"transcript_full", "transcript"} {
		path := filepath.Join(logs, base+".jsonl")
		if stat, err := os.Stat(path); err == nil && stat.Mode().IsRegular() {
			return []string{path}, nil
		} else if err != nil && !missingStore(err) {
			return nil, err
		}
		chunks, err := filepath.Glob(filepath.Join(logs, "chunks", base, "*.jsonl"))
		if err != nil {
			return nil, err
		}
		if len(chunks) > 0 {
			return chunks, nil
		}
	}
	return nil, nil
}

func loadAntigravity(ctx context.Context, source Source) ([]Session, error) {
	root := source.filePath(source.AgyDir)
	byID := map[string]Session{}
	database := filepath.Join(root, "conversation_summaries.db")
	if _, err := os.Stat(database); err == nil {
		db, err := openDatabase(ctx, database, false)
		if err != nil {
			return nil, err
		}
		tables, err := databaseTables(ctx, db)
		if err != nil {
			db.Close()
			return nil, err
		}
		cols := tables["conversation_summaries"]
		if !slices.Contains(cols, "conversation_id") {
			db.Close()
			return nil, unsupportedStore("Antigravity")
		}
		query := "SELECT conversation_id," + columnExpr(cols, "title", "''") + "," + columnExpr(cols, "workspace_uris", "'[]'") + "," + columnExpr(cols, "last_modified_time", "''") + "," + columnExpr(cols, "parent_conversation_id", "''") + " FROM conversation_summaries"
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			db.Close()
			return nil, err
		}
		for rows.Next() {
			var s Session
			var uris, last string
			if err := rows.Scan(&s.ID, &s.Title, &uris, &last, &s.ParentID); err != nil {
				rows.Close()
				db.Close()
				return nil, err
			}
			var workspaces []string
			if err := json.Unmarshal([]byte(uris), &workspaces); err != nil {
				rows.Close()
				db.Close()
				return nil, fmt.Errorf("Antigravity 工作目录记录：%w", err)
			}
			for _, uri := range workspaces {
				if s.Cwd = workspacePath(uri, source); s.Cwd != "" {
					break
				}
			}
			s.UpdatedAt = parseAgyTime(last)
			byID[s.ID] = s
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			db.Close()
			return nil, err
		}
		rows.Close()
		db.Close()
	} else if !missingStore(err) {
		return nil, err
	}
	for _, folder := range []string{"brain", "conversations"} {
		entries, err := os.ReadDir(filepath.Join(root, folder))
		if missingStore(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			id := entry.Name()
			if folder == "conversations" {
				if !strings.HasSuffix(id, ".db") {
					continue
				}
				id = strings.TrimSuffix(id, ".db")
			}
			if !sessionIDPattern.MatchString(id) {
				continue
			}
			if _, ok := byID[id]; !ok {
				byID[id] = Session{ID: id}
			}
		}
	}
	var out []Session
	for id, s := range byID {
		if !sessionIDPattern.MatchString(id) {
			continue
		}
		s.SourceID, s.Provider, s.Store = source.ID, "agy", "agy"
		s.Key = source.ID + "|agy|" + id
		paths, err := agyTranscriptPaths(root, id)
		if err != nil {
			return nil, err
		}
		s.Paths = paths
		s.Path = filepath.Join(root, "conversations", id+".db")
		if len(paths) > 0 {
			s.Path = paths[0]
		} else {
			s.Store = "agy-summary"
		}
		steps, err := readAgySteps(ctx, paths)
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		metadata, _ := json.Marshal(s)
		hash.Write(metadata)
		for _, step := range steps {
			raw, _ := json.Marshal(step)
			hash.Write(raw)
			at := parseAgyTime(step.CreatedAt)
			if !at.IsZero() {
				if s.StartedAt.IsZero() {
					s.StartedAt = at
				}
				if at.After(s.UpdatedAt) {
					s.UpdatedAt = at
				}
			}
			if step.Source == "USER_EXPLICIT" && step.Type == "USER_INPUT" {
				s.Messages++
				if s.Title == "" {
					s.Title = strings.Join(strings.Fields(agyRequest(step.Content)), " ")
				}
			}
			if step.Source == "MODEL" && step.Type == "PLANNER_RESPONSE" {
				s.Tokens.Input += step.Input + step.Cached
				s.Tokens.Cached += step.Cached
				s.Tokens.Output += step.Output
				if step.Content != "" {
					s.Messages++
				}
			}
		}
		for _, path := range paths {
			stat, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			s.Size += stat.Size()
			fmt.Fprintf(hash, "%s:%d:%d", path, stat.Size(), stat.ModTime().UnixNano())
		}
		s.Revision = hex.EncodeToString(hash.Sum(nil))
		if s.Title == "" {
			s.Title = "未命名会话"
		}
		if s.StartedAt.IsZero() {
			s.StartedAt = s.UpdatedAt
		}
		s.Project = projectName(s.Cwd)
		out = append(out, s)
	}
	return out, nil
}

func readAgySteps(ctx context.Context, paths []string) ([]agyStep, error) {
	var order []int
	steps := map[int]agyStep{}
	for _, path := range paths {
		_, invalid, err := readRecords(ctx, path, 0, func(r record, _ int64) bool {
			var step agyStep
			if json.Unmarshal(r.Raw, &step) != nil || step.Index == nil {
				return true
			}
			if _, ok := steps[*step.Index]; !ok {
				order = append(order, *step.Index)
			}
			steps[*step.Index] = step
			return true
		})
		if err != nil {
			return nil, err
		}
		if invalid > 0 {
			return nil, fmt.Errorf("Antigravity 正文含 %d 条无法解析的记录", invalid)
		}
	}
	var out []agyStep
	for _, index := range order {
		out = append(out, steps[index])
	}
	return out, nil
}

func agyParts(steps []agyStep) []Part {
	var out []Part
	for _, step := range steps {
		switch {
		case step.Source == "USER_EXPLICIT" && step.Type == "USER_INPUT":
			out = append(out, Part{Role: "user", Kind: "text", Text: agyRequest(step.Content)})
		case step.Source == "MODEL" && step.Type == "PLANNER_RESPONSE":
			if step.Thinking != "" {
				out = append(out, Part{Role: "assistant", Kind: "thinking", Text: step.Thinking})
			}
			if step.Content != "" {
				out = append(out, Part{Role: "assistant", Kind: "text", Text: step.Content})
			}
			for _, call := range step.ToolCalls {
				out = append(out, Part{Role: "assistant", Kind: "tool_use", Name: call.Name, Text: prettyJSON(call.Args)})
			}
		case step.Source == "MODEL" && step.Content != "":
			text := step.Content
			for _, prefix := range []string{"Created At:", "Completed At:"} {
				if strings.HasPrefix(text, prefix) {
					_, text, _ = strings.Cut(text, "\n")
				}
			}
			out = append(out, Part{Role: "tool", Kind: "tool_result", Text: strings.TrimSpace(text)})
		}
	}
	return out
}

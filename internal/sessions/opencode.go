package sessions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

type openCodeMessage struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	ModelID string `json:"modelID"`
	Model   struct {
		ID string `json:"id"`
	} `json:"model"`
	Text    string            `json:"text"`
	Content []json.RawMessage `json:"content"`
	Tokens  struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	Time struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
}

type openCodePart struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Name      string `json:"name"`
	Tool      string `json:"tool"`
	Synthetic bool   `json:"synthetic"`
	Ignored   bool   `json:"ignored"`
	State     struct {
		Input   json.RawMessage `json:"input"`
		Output  string          `json:"output"`
		Content []block         `json:"content"`
	} `json:"state"`
}

func openCodeParts(role, model string, raw json.RawMessage) ([]Part, error) {
	var p openCodePart
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.Synthetic || p.Ignored {
		return nil, nil
	}
	base := Part{Role: role, Model: model}
	switch p.Type {
	case "text":
		base.Kind, base.Text = "text", p.Text
		if strings.TrimSpace(base.Text) != "" {
			return []Part{base}, nil
		}
	case "reasoning":
		base.Kind, base.Text = "thinking", p.Text
		return []Part{base}, nil
	case "tool":
		base.Kind, base.Name, base.Text = "tool_use", p.Tool, prettyJSON(p.State.Input)
		if base.Name == "" {
			base.Name = p.Name
		}
		parts := []Part{base}
		if p.State.Output != "" {
			parts = append(parts, Part{Role: "tool", Kind: "tool_result", Name: base.Name, Text: p.State.Output})
		}
		for _, item := range p.State.Content {
			if item.Type == "text" {
				parts = append(parts, Part{Role: "tool", Kind: "tool_result", Name: base.Name, Text: item.Text})
			}
		}
		return parts, nil
	}
	return nil, nil
}

func loadOpenCode(ctx context.Context, source Source) ([]Session, error) {
	root := source.filePath(source.OpenCodeDir)
	database := filepath.Join(root, "opencode.db")
	if _, err := os.Stat(database); missingStore(err) {
		return loadOpenCodeJSON(ctx, source)
	} else if err != nil {
		return nil, err
	}
	db, err := openDatabase(ctx, database, false)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tables, err := databaseTables(ctx, db)
	if err != nil {
		return nil, err
	}
	var formats []string
	if len(tables["session_v2"]) > 0 {
		formats = append(formats, "session_v2")
	}
	if len(tables["session"]) > 0 {
		migrated := false
		if len(formats) > 0 && len(tables["kv"]) > 0 {
			var raw string
			if err := db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key='migration.v1-v2'`).Scan(&raw); err == nil {
				var state struct {
					Phase string `json:"phase"`
				}
				if err := json.Unmarshal([]byte(raw), &state); err != nil {
					return nil, err
				}
				migrated = state.Phase == "completed"
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		if !migrated {
			formats = append(formats, "session")
		}
	}
	if len(formats) == 0 {
		return nil, unsupportedStore("OpenCode")
	}
	var out []Session
	seen := map[string]bool{}
	for _, table := range formats {
		columns := tables[table]
		for _, required := range []string{"id", "directory", "time_created", "time_updated"} {
			if !slices.Contains(columns, required) {
				return nil, unsupportedStore("OpenCode")
			}
		}
		query := "SELECT id," + columnExpr(columns, "title", "''") + ",directory," + columnExpr(columns, "parent_id", "''") + ",time_created,time_updated," + columnExpr(columns, "time_archived", "0") + " FROM " + quoteSQL(table)
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		var batch []Session
		for rows.Next() {
			var s Session
			var created, updated, archived int64
			if err := rows.Scan(&s.ID, &s.Title, &s.Cwd, &s.ParentID, &created, &updated, &archived); err != nil {
				rows.Close()
				return nil, err
			}
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			s.SourceID, s.Provider, s.Path, s.Store = source.ID, "opencode", database, "opencode-v1"
			if table == "session_v2" {
				s.Store = "opencode-v2"
			}
			s.StartedAt, s.UpdatedAt, s.Archived = time.UnixMilli(created), time.UnixMilli(updated), archived > 0
			batch = append(batch, s)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		for _, s := range batch {
			parts, messages, tokens, model, revision, err := openCodeConversation(ctx, db, s)
			if err != nil {
				return nil, err
			}
			s.Messages, s.Tokens, s.Model = messages, tokens, model
			for _, part := range parts {
				s.Size += int64(len(part.Text))
				if s.Title == "" && part.Role == "user" && part.Kind == "text" {
					s.Title = strings.Join(strings.Fields(part.Text), " ")
				}
			}
			if s.Title == "" {
				s.Title = "未命名会话"
			}
			s.Project, s.Key = projectName(s.Cwd), source.ID+"|opencode|"+s.ID
			s.Revision = revision + fmt.Sprintf(":%s:%d", s.Title, s.UpdatedAt.UnixMilli())
			out = append(out, s)
		}
	}
	return out, nil
}

func openCodeConversation(ctx context.Context, db *sql.DB, s Session) ([]Part, int, Tokens, string, string, error) {
	var parts []Part
	var tokens Tokens
	var count int
	var model string
	hash := sha256.New()
	var query string
	if s.Store == "opencode-v2" {
		query = `SELECT id,type,data FROM session_message WHERE session_id=? ORDER BY seq,id`
	} else {
		query = `SELECT id,'' AS type,data FROM message WHERE session_id=? ORDER BY id`
	}
	rows, err := db.QueryContext(ctx, query, s.ID)
	if err != nil {
		return nil, 0, tokens, "", "", err
	}
	type rawMessage struct {
		ID, Role string
		Raw      []byte
	}
	var messages []rawMessage
	for rows.Next() {
		var m rawMessage
		if err := rows.Scan(&m.ID, &m.Role, &m.Raw); err != nil {
			rows.Close()
			return nil, 0, tokens, "", "", err
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, tokens, "", "", err
	}
	rows.Close()
	for _, row := range messages {
		hash.Write(row.Raw)
		var m openCodeMessage
		if err := json.Unmarshal(row.Raw, &m); err != nil {
			return nil, 0, tokens, "", "", fmt.Errorf("OpenCode 消息 %s：%w", row.ID, err)
		}
		if row.Role != "" {
			m.Role = row.Role
		}
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		count++
		messageModel := m.ModelID
		if messageModel == "" {
			messageModel = m.Model.ID
		}
		if m.Role == "assistant" {
			if messageModel != "" {
				model = messageModel
			}
			tokens.Input += m.Tokens.Input + m.Tokens.Cache.Read + m.Tokens.Cache.Write
			tokens.Output += m.Tokens.Output + m.Tokens.Reasoning
			tokens.Cached += m.Tokens.Cache.Read
		}
		if s.Store == "opencode-v2" {
			if m.Role == "user" {
				if m.Text != "" {
					parts = append(parts, Part{Role: "user", Kind: "text", Text: m.Text})
				}
				continue
			}
			for _, raw := range m.Content {
				found, err := openCodeParts(m.Role, messageModel, raw)
				if err != nil {
					return nil, 0, tokens, "", "", err
				}
				parts = append(parts, found...)
			}
			continue
		}
		r, err := db.QueryContext(ctx, `SELECT data FROM part WHERE message_id=? ORDER BY id`, row.ID)
		if err != nil {
			return nil, 0, tokens, "", "", err
		}
		for r.Next() {
			var raw []byte
			if err := r.Scan(&raw); err != nil {
				r.Close()
				return nil, 0, tokens, "", "", err
			}
			hash.Write(raw)
			found, err := openCodeParts(m.Role, messageModel, raw)
			if err != nil {
				r.Close()
				return nil, 0, tokens, "", "", err
			}
			parts = append(parts, found...)
		}
		if err := r.Err(); err != nil {
			r.Close()
			return nil, 0, tokens, "", "", err
		}
		r.Close()
	}
	return parts, count, tokens, model, hex.EncodeToString(hash.Sum(nil)), nil
}

type openCodeLegacySession struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentID"`
	Directory string `json:"directory"`
	Title     string `json:"title"`
	Time      struct {
		Created int64 `json:"created"`
		Updated int64 `json:"updated"`
	} `json:"time"`
}

func loadOpenCodeJSON(ctx context.Context, source Source) ([]Session, error) {
	root := filepath.Join(source.filePath(source.OpenCodeDir), "storage")
	paths, err := filepath.Glob(filepath.Join(root, "session", "*", "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var m openCodeLegacySession
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		if !openCodeIDPattern.MatchString(m.ID) {
			return nil, fmt.Errorf("OpenCode 会话 ID 无效")
		}
		s := Session{ID: m.ID, SourceID: source.ID, Provider: "opencode", Title: m.Title, Cwd: m.Directory, ParentID: m.ParentID, Path: path, Store: "opencode-json", StartedAt: time.UnixMilli(m.Time.Created), UpdatedAt: time.UnixMilli(m.Time.Updated)}
		conversation, err := openCodeJSONConversation(ctx, root, s.ID)
		if err != nil {
			return nil, err
		}
		s.Paths = append([]string{path}, conversation.Files...)
		metaHash := sha256.Sum256(raw)
		s.Revision = conversation.Revision + hex.EncodeToString(metaHash[:])
		s.Tokens, s.Model, s.Messages = conversation.Tokens, conversation.Model, conversation.Messages
		s.Project = projectName(s.Cwd)
		s.Key = source.ID + "|opencode|" + s.ID
		for _, p := range conversation.Parts {
			s.Size += int64(len(p.Text))
			if p.Kind == "text" {
				if s.Title == "" && p.Role == "user" {
					s.Title = strings.Join(strings.Fields(p.Text), " ")
				}
			}
		}
		if s.Title == "" {
			s.Title = "未命名会话"
		}
		out = append(out, s)
	}
	return out, nil
}

var openCodeMessageIDPattern = regexp.MustCompile(`^msg_[a-zA-Z0-9]+$`)

type legacyConversation struct {
	Parts    []Part
	Files    []string
	Tokens   Tokens
	Model    string
	Messages int
	Revision string
}

func openCodeJSONConversation(ctx context.Context, root, id string) (legacyConversation, error) {
	var out legacyConversation
	if !openCodeIDPattern.MatchString(id) {
		return out, fmt.Errorf("OpenCode 会话 ID 无效")
	}
	paths, err := filepath.Glob(filepath.Join(root, "message", id, "*.json"))
	if err != nil {
		return out, err
	}
	type entry struct {
		Message openCodeMessage
		Path    string
		Raw     []byte
	}
	var messages []entry
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return out, err
		}
		var m openCodeMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return out, err
		}
		if !openCodeMessageIDPattern.MatchString(m.ID) {
			return out, fmt.Errorf("OpenCode 消息 ID 无效")
		}
		messages = append(messages, entry{m, path, raw})
	}
	slices.SortFunc(messages, func(a, b entry) int {
		if a.Message.Time.Created < b.Message.Time.Created {
			return -1
		}
		if a.Message.Time.Created > b.Message.Time.Created {
			return 1
		}
		return strings.Compare(a.Message.ID, b.Message.ID)
	})
	hash := sha256.New()
	for _, m := range messages {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		out.Files = append(out.Files, m.Path)
		hash.Write(m.Raw)
		model := m.Message.ModelID
		if model == "" {
			model = m.Message.Model.ID
		}
		if m.Message.Role == "assistant" {
			out.Model = model
			out.Tokens.Input += m.Message.Tokens.Input + m.Message.Tokens.Cache.Read + m.Message.Tokens.Cache.Write
			out.Tokens.Output += m.Message.Tokens.Output + m.Message.Tokens.Reasoning
			out.Tokens.Cached += m.Message.Tokens.Cache.Read
		}
		if m.Message.Role == "assistant" || m.Message.Role == "user" {
			out.Messages++
		}
		paths, err := filepath.Glob(filepath.Join(root, "part", m.Message.ID, "*.json"))
		if err != nil {
			return out, err
		}
		for _, path := range paths {
			raw, err := os.ReadFile(path)
			if err != nil {
				return out, err
			}
			hash.Write(raw)
			out.Files = append(out.Files, path)
			found, err := openCodeParts(m.Message.Role, model, raw)
			if err != nil {
				return out, err
			}
			out.Parts = append(out.Parts, found...)
		}
	}
	out.Revision = hex.EncodeToString(hash.Sum(nil))
	return out, nil
}

func openCodeTranscript(ctx context.Context, s Session) ([]Part, error) {
	if s.Store == "opencode-json" {
		root := filepath.Dir(filepath.Dir(filepath.Dir(s.Path)))
		conversation, err := openCodeJSONConversation(ctx, root, s.ID)
		return conversation.Parts, err
	}
	db, err := openDatabase(ctx, s.Path, false)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	parts, _, _, _, _, err := openCodeConversation(ctx, db, s)
	return parts, err
}

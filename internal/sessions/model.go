package sessions

import (
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Source struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Distro      string `json:"distro,omitempty"`
	Home        string `json:"home"`
	ClaudeDir   string `json:"claude_dir"`
	CodexDir    string `json:"codex_dir"`
	OpenCodeDir string `json:"opencode_dir"`
	AgyDir      string `json:"agy_dir"`
	Running     bool   `json:"running"`
	Error       string `json:"error,omitempty"`
}

func (s Source) filePath(path string) string {
	if runtime.GOOS == "windows" && s.Kind == "wsl" && strings.HasPrefix(path, "/") {
		return `\\wsl.localhost\` + s.Distro + strings.ReplaceAll(path, "/", `\`)
	}
	return filepath.Clean(path)
}

type Tokens struct {
	// Input includes cache reads and writes for both providers.
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Cached int64 `json:"cached"`
}

type Session struct {
	Key              string    `json:"key"`
	ID               string    `json:"id"`
	SourceID         string    `json:"source_id"`
	Provider         string    `json:"provider"`
	Title            string    `json:"title"`
	Cwd              string    `json:"cwd"`
	Project          string    `json:"project"`
	Model            string    `json:"model"`
	Branch           string    `json:"branch,omitempty"`
	ParentID         string    `json:"parent_id,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Messages         int       `json:"messages"`
	Size             int64     `json:"size"`
	Revision         string    `json:"revision"`
	Tokens           Tokens    `json:"tokens"`
	Path             string    `json:"path"`
	ResponseMessages bool      `json:"response_messages,omitempty"`
	Warning          string    `json:"warning,omitempty"`
	Store            string    `json:"store,omitempty"`
	Paths            []string  `json:"paths,omitempty"`
	Archived         bool      `json:"archived,omitempty"`
}

type Part struct {
	Role  string `json:"role"`
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
	Text  string `json:"text"`
	Model string `json:"model,omitempty"`
}

type Transcript struct {
	Parts      []Part `json:"parts"`
	NextCursor int64  `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

type Snapshot struct {
	Sources   []Source  `json:"sources"`
	Sessions  []Session `json:"sessions"`
	UpdatedAt time.Time `json:"updated_at"`
	Warnings  []string  `json:"warnings"`
	Demo      bool      `json:"demo"`
	Platform  string    `json:"platform"`
	CanResume bool      `json:"can_resume"`
}

type SourceOverride struct {
	ID          string `json:"id"`
	ClaudeDir   string `json:"claude_dir"`
	CodexDir    string `json:"codex_dir"`
	OpenCodeDir string `json:"opencode_dir"`
	AgyDir      string `json:"agy_dir"`
}

var Providers = []string{"claude", "codex", "opencode", "agy"}

func (s Source) dataDir(provider string) string {
	switch provider {
	case "claude":
		return s.ClaudeDir
	case "codex":
		return s.CodexDir
	case "opencode":
		return s.OpenCodeDir
	case "agy":
		return s.AgyDir
	}
	return ""
}

type Settings struct {
	Sources []SourceOverride `json:"sources"`
}

type SearchHit struct {
	Key     string `json:"key"`
	Excerpt string `json:"excerpt"`
}

func projectName(cwd string) string {
	clean := strings.TrimRight(strings.ReplaceAll(cwd, `\`, "/"), "/")
	if clean == "" {
		return "未关联项目"
	}
	return clean[strings.LastIndex(clean, "/")+1:]
}

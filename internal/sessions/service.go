package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type Options struct {
	ConfigPath string
	Home       string
	Demo       bool
}

type cachedSession struct {
	Size    int64   `json:"size"`
	Mod     int64   `json:"mod"`
	Session Session `json:"session"`
}

type Service struct {
	mu          sync.RWMutex
	refreshMu   sync.Mutex
	options     Options
	settings    Settings
	view        Snapshot
	byKey       map[string]Session
	cache       map[string]cachedSession
	fixed       []Source
	workerPath  string
	workerPaths map[string]string
}

func New(options Options) (*Service, error) {
	if options.ConfigPath == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		options.ConfigPath = filepath.Join(dir, "SessionDesk", "settings.json")
	}
	s := &Service{options: options, workerPaths: map[string]string{}, byKey: map[string]Session{}, cache: map[string]cachedSession{}, settings: Settings{Sources: []SourceOverride{}}}
	s.view = Snapshot{Sources: []Source{}, Sessions: []Session{}, Warnings: []string{}, Demo: options.Demo, Platform: runtime.GOOS}
	if runtime.GOOS == "windows" && !options.Demo {
		_, terminalErr := exec.LookPath("wt.exe")
		s.view.CanResume = terminalErr == nil
	}
	data, err := os.ReadFile(options.ConfigPath)
	if err == nil {
		if err := json.Unmarshal(data, &s.settings); err != nil {
			return nil, fmt.Errorf("读取设置 %s：%w", options.ConfigPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if data, err := os.ReadFile(s.cachePath()); err == nil {
		if err := json.Unmarshal(data, &s.cache); err != nil {
			s.cache = map[string]cachedSession{}
			s.view.Warnings = append(s.view.Warnings, "会话缓存已失效，将重新扫描")
		}
	}
	if data, err := os.ReadFile(s.sourcesPath()); err == nil {
		if err := json.Unmarshal(data, &s.view.Sources); err != nil {
			s.view.Sources = []Source{}
		}
	}
	if options.Demo {
		sources, err := makeDemo(filepath.Join(filepath.Dir(options.ConfigPath), "demo"))
		if err != nil {
			return nil, err
		}
		s.fixed = sources
	}
	return s, nil
}

func (s *Service) cachePath() string {
	return filepath.Join(filepath.Dir(s.options.ConfigPath), "session-cache.json")
}

func (s *Service) sourcesPath() string {
	return filepath.Join(filepath.Dir(s.options.ConfigPath), "source-cache.json")
}

func (s *Service) View() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.view
}

func (s *Service) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Service) SaveSettings(settings Settings) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	known := map[string]Source{}
	for _, source := range s.View().Sources {
		known[source.ID] = source
	}
	seen := map[string]bool{}
	for _, override := range settings.Sources {
		source, ok := known[override.ID]
		if !ok || seen[override.ID] {
			return errors.New("数据源无效，请刷新后重试")
		}
		seen[override.ID] = true
		for _, dir := range []string{override.ClaudeDir, override.CodexDir, override.OpenCodeDir, override.AgyDir} {
			if dir == "" {
				continue
			}
			absolute := filepath.IsAbs(dir)
			if source.Kind == "wsl" {
				absolute = strings.HasPrefix(dir, "/")
			}
			if strings.ContainsAny(dir, "\x00\r\n") || !absolute {
				return errors.New("数据目录需要填写绝对路径")
			}
		}
	}
	if err := writeJSON(s.options.ConfigPath, settings); err != nil {
		return err
	}
	s.mu.Lock()
	s.settings = settings
	s.mu.Unlock()
	return nil
}

func (s *Service) Refresh(ctx context.Context) (Snapshot, error) {
	return s.refresh(ctx, nil)
}

// A scoped refresh keeps unrelated stores and their cached summaries intact.
func (s *Service) refresh(ctx context.Context, scope map[string]map[string]bool) (Snapshot, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	previous := s.View()
	sources := slices.Clone(s.fixed)
	if scope != nil && sources == nil {
		sources = slices.Clone(previous.Sources)
	}
	var discoveryErr error
	if sources == nil {
		sources, discoveryErr = discoverSources(ctx, s.options.Home, previous.Sources)
	}
	settings := s.Settings()
	for i := range sources {
		for _, override := range settings.Sources {
			if override.ID == sources[i].ID {
				if override.ClaudeDir != "" {
					sources[i].ClaudeDir = override.ClaudeDir
				}
				if override.CodexDir != "" {
					sources[i].CodexDir = override.CodexDir
				}
				if override.OpenCodeDir != "" {
					sources[i].OpenCodeDir = override.OpenCodeDir
				}
				if override.AgyDir != "" {
					sources[i].AgyDir = override.AgyDir
				}
			}
		}
	}
	view := Snapshot{Sources: sources, Sessions: []Session{}, Warnings: []string{}, Demo: s.options.Demo, Platform: runtime.GOOS, CanResume: previous.CanResume}
	if discoveryErr != nil {
		view.Warnings = append(view.Warnings, discoveryErr.Error())
	}
	nextCache := map[string]cachedSession{}
	byKey := map[string]Session{}
	if scope != nil {
		view.Warnings = slices.Clone(previous.Warnings)
		for key, cached := range s.cache {
			if !scope[cached.Session.SourceID][cached.Session.Provider] {
				nextCache[key] = cached
			}
		}
		for _, session := range previous.Sessions {
			if !scope[session.SourceID][session.Provider] {
				byKey[session.Key] = session
			}
		}
	}
	for _, source := range sources {
		if scope != nil && scope[source.ID] == nil {
			continue
		}
		if source.Error != "" {
			view.Warnings = append(view.Warnings, source.Name+"："+source.Error)
		}
		if !source.Running || source.Error != "" {
			for key, cached := range s.cache {
				if cached.Session.SourceID == source.ID && (scope == nil || scope[source.ID][cached.Session.Provider]) {
					root := source.dataDir(cached.Session.Provider)
					relative, err := filepath.Rel(source.filePath(root), source.filePath(cached.Session.Path))
					if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
						continue
					}
					nextCache[key] = cached
					byKey[cached.Session.Key] = cached.Session
				}
			}
			continue
		}
		if s.remoteSource(source) {
			response, err := s.callWorker(ctx, WorkerRequest{Action: "list", Source: source})
			if err != nil {
				view.Warnings = append(view.Warnings, source.Name+"："+err.Error())
				continue
			}
			for _, session := range response.Snapshot.Sessions {
				byKey[session.Key] = session
				nextCache[source.ID+"|remote|"+session.Key] = cachedSession{Session: session}
			}
			view.Warnings = append(view.Warnings, response.Snapshot.Warnings...)
			continue
		}
		for _, provider := range Providers {
			if scope != nil && !scope[source.ID][provider] {
				continue
			}
			root := source.dataDir(provider)
			if root == "" {
				continue
			}
			root = source.filePath(root)
			if provider == "opencode" || provider == "agy" {
				var found []Session
				var err error
				if provider == "opencode" {
					found, err = loadOpenCode(ctx, source)
				} else {
					found, err = loadAntigravity(ctx, source)
				}
				if err != nil {
					if scope != nil {
						return previous, err
					}
					view.Warnings = append(view.Warnings, source.Name+" / "+provider+"："+err.Error())
				}
				for _, session := range found {
					byKey[session.Key] = session
					nextCache[source.ID+"|"+provider+"|"+session.ID] = cachedSession{Session: session}
				}
				continue
			}
			folders := []string{filepath.Join(root, "projects")}
			if provider == "codex" {
				folders = []string{filepath.Join(root, "sessions"), filepath.Join(root, "archived_sessions")}
			}
			for _, folder := range folders {
				walkErr := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
					if errors.Is(err, os.ErrNotExist) && path == folder {
						return nil
					}
					if err != nil {
						if scope != nil {
							return err
						}
						view.Warnings = append(view.Warnings, source.Name+"："+err.Error())
						return nil
					}
					if err := ctx.Err(); err != nil {
						return err
					}
					if entry.IsDir() {
						if provider == "claude" && path != folder {
							rel, _ := filepath.Rel(folder, path)
							if strings.ContainsRune(rel, filepath.Separator) {
								return filepath.SkipDir
							}
						}
						return nil
					}
					if entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".jsonl") {
						return nil
					}
					stat, err := entry.Info()
					if err != nil {
						view.Warnings = append(view.Warnings, err.Error())
						return nil
					}
					cacheKey := source.ID + "|" + provider + "|" + path
					cached, ok := s.cache[cacheKey]
					if !ok || cached.Session.Path != path || cached.Size != stat.Size() || cached.Mod != stat.ModTime().UnixNano() {
						id := strings.TrimSuffix(entry.Name(), ".jsonl")
						session, err := readSummary(ctx, source, provider, path, id, stat)
						if err != nil {
							if scope != nil {
								return err
							}
							view.Warnings = append(view.Warnings, source.Name+"："+err.Error())
							return nil
						}
						cached = cachedSession{Size: stat.Size(), Mod: stat.ModTime().UnixNano(), Session: session}
					}
					cached.Session.Archived = provider == "codex" && filepath.Base(folder) == "archived_sessions"
					nextCache[cacheKey] = cached
					old, exists := byKey[cached.Session.Key]
					if !exists || old.UpdatedAt.Before(cached.Session.UpdatedAt) {
						byKey[cached.Session.Key] = cached.Session
					}
					return nil
				})
				if walkErr != nil {
					return previous, walkErr
				}
			}
			if provider == "codex" {
				if err := applyCodexTitles(ctx, filepath.Join(root, "session_index.jsonl"), source.ID, byKey); err != nil {
					view.Warnings = append(view.Warnings, "Codex 标题索引："+err.Error())
				}
			}
		}
	}
	for _, session := range byKey {
		view.Sessions = append(view.Sessions, session)
	}
	slices.SortFunc(view.Sessions, func(a, b Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	view.UpdatedAt = time.Now()
	s.cache = nextCache
	for key, cached := range nextCache {
		if session, exists := byKey[cached.Session.Key]; exists {
			cached.Session.Title = session.Title
		}
		nextCache[key] = cached
	}
	if err := writeJSON(s.cachePath(), nextCache); err != nil {
		view.Warnings = append(view.Warnings, "缓存写入失败："+err.Error())
	}
	if err := writeJSON(s.sourcesPath(), sources); err != nil {
		view.Warnings = append(view.Warnings, "数据源缓存写入失败："+err.Error())
	}
	s.mu.Lock()
	s.view, s.byKey = view, byKey
	s.mu.Unlock()
	return view, nil
}

func applyCodexTitles(ctx context.Context, path, sourceID string, sessions map[string]Session) error {
	_, invalid, err := readRecords(ctx, path, 0, func(entry record, _ int64) bool {
		key := sourceID + "|codex|" + entry.ID
		if session, ok := sessions[key]; ok && entry.ThreadName != "" {
			session.Title = strings.Join(strings.Fields(entry.ThreadName), " ")
			sessions[key] = session
		}
		return true
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("已跳过 %d 条无法解析的记录", invalid)
	}
	return nil
}

func (s *Service) Find(key string) (Session, Source, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.byKey[key]
	if !ok {
		return Session{}, Source{}, errors.New("未找到会话，请刷新列表")
	}
	for _, source := range s.view.Sources {
		if source.ID == session.SourceID {
			return session, source, nil
		}
	}
	return Session{}, Source{}, errors.New("会话数据源已变化，请刷新列表")
}

func (s *Service) Transcript(ctx context.Context, key string, cursor int64) (Transcript, error) {
	session, source, err := s.Find(key)
	if err != nil {
		return Transcript{}, err
	}
	running, err := s.sourceRunning(ctx, source)
	if err != nil {
		return Transcript{}, err
	}
	if !running {
		return Transcript{}, errors.New("请先启动对应的 WSL 发行版，再刷新会话")
	}
	if s.remoteSource(source) {
		response, err := s.callWorker(ctx, WorkerRequest{Action: "transcript", Source: source, Key: key, Cursor: cursor})
		if err != nil {
			return Transcript{}, err
		}
		return *response.Transcript, nil
	}
	return providerTranscript(ctx, session, cursor)
}

type SearchResult struct {
	Hits    []SearchHit `json:"hits"`
	Limited bool        `json:"limited"`
	Errors  []string    `json:"errors"`
}

func (s *Service) Search(ctx context.Context, query, provider, sourceID string) SearchResult {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out := SearchResult{Hits: []SearchHit{}, Errors: []string{}}
	query = strings.ToLower(strings.Join(strings.Fields(query), " "))
	if query == "" {
		return out
	}
	view := s.View()
	running := map[string]bool{}
	remote := map[string]bool{}
	for _, source := range view.Sources {
		if sourceID != "" && source.ID != sourceID {
			continue
		}
		up, err := s.sourceRunning(ctx, source)
		if err != nil {
			out.Errors = append(out.Errors, source.Name+"："+err.Error())
		}
		running[source.ID] = up
		if up && s.remoteSource(source) {
			remote[source.ID] = true
			response, err := s.callWorker(ctx, WorkerRequest{Action: "search", Source: source, Query: query, Provider: provider})
			if err != nil {
				out.Errors = append(out.Errors, err.Error())
				continue
			}
			out.Hits = append(out.Hits, response.Search.Hits...)
			out.Limited = out.Limited || response.Search.Limited
			out.Errors = append(out.Errors, response.Search.Errors...)
		}
	}
	for _, session := range view.Sessions {
		if remote[session.SourceID] {
			continue
		}
		if provider != "" && provider != session.Provider || sourceID != "" && sourceID != session.SourceID {
			continue
		}
		if strings.Contains(strings.ToLower(session.Title+" "+session.Cwd+" "+session.ID+" "+session.Model), query) {
			out.Hits = append(out.Hits, SearchHit{Key: session.Key})
			continue
		}
		if !running[session.SourceID] {
			continue
		}
		_, err := scanParts(ctx, session, func(part Part) bool {
			lower := strings.ToLower(part.Text)
			at := strings.Index(lower, query)
			if at < 0 {
				return true
			}
			text := []rune(part.Text)
			before := len([]rune(lower[:at]))
			start, end := max(0, before-30), min(len(text), before+len([]rune(query))+70)
			out.Hits = append(out.Hits, SearchHit{Key: session.Key, Excerpt: strings.Join(strings.Fields(string(text[start:end])), " ")})
			return false
		})
		if ctx.Err() != nil {
			out.Limited = true
			break
		}
		if err != nil {
			out.Errors = append(out.Errors, session.Title+"："+err.Error())
		}
		if len(out.Hits) >= 200 {
			out.Limited = true
			break
		}
	}
	return out
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".session-desk-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// The caller holds refreshMu while publishing a completed mutation or a remote preview.
func (s *Service) updateSessions(removed []string, added []Session) {
	if len(removed) == 0 && len(added) == 0 {
		return
	}
	changed := map[string]bool{}
	for _, key := range removed {
		changed[key] = true
	}
	for _, session := range added {
		changed[session.Key] = true
	}
	view := s.View()
	byKey := map[string]Session{}
	for _, session := range view.Sessions {
		if !changed[session.Key] {
			byKey[session.Key] = session
		}
	}
	for _, session := range added {
		byKey[session.Key] = session
	}
	view.Sessions = []Session{}
	for _, session := range byKey {
		view.Sessions = append(view.Sessions, session)
	}
	slices.SortFunc(view.Sessions, func(a, b Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	view.UpdatedAt = time.Now()
	for key, cached := range s.cache {
		if changed[cached.Session.Key] {
			delete(s.cache, key)
		}
	}
	for _, session := range added {
		s.cache[session.SourceID+"|updated|"+session.Key] = cachedSession{Session: session}
	}
	if err := writeJSON(s.cachePath(), s.cache); err != nil {
		view.Warnings = append(slices.Clone(view.Warnings), "缓存写入失败："+err.Error())
	}
	s.mu.Lock()
	s.view, s.byKey = view, byKey
	s.mu.Unlock()
}

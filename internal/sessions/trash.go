package sessions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type DeletePlan struct {
	Keys      []string          `json:"keys"`
	Targets   []Session         `json:"targets"`
	Revisions map[string]string `json:"revisions"`
	Related   int               `json:"related"`
}

type MutationFailure struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}
type MutationResult struct {
	Done   []string          `json:"done"`
	Failed []MutationFailure `json:"failed"`
}
type TrashFile struct {
	Original string `json:"original"`
	Saved    string `json:"saved"`
}
type TrashEntry struct {
	ID        string           `json:"id"`
	Key       string           `json:"key"`
	SourceID  string           `json:"source_id"`
	Provider  string           `json:"provider"`
	Root      string           `json:"root"`
	Title     string           `json:"title"`
	DeletedAt time.Time        `json:"deleted_at"`
	Sessions  []Session        `json:"sessions"`
	Files     []TrashFile      `json:"files"`
	Databases []DatabaseBackup `json:"databases,omitempty"`
	AgyCache  []byte           `json:"agy_cache,omitempty"`
	Status    string           `json:"status"`
	Offline   bool             `json:"offline"`
}

func ownedPath(root, relative string) (string, error) {
	if !filepath.IsLocal(relative) {
		return "", errors.New("会话文件路径越界")
	}
	current := root
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		stat, err := os.Lstat(current)
		if missingStore(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if stat.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("会话路径含符号链接，请使用真实数据目录")
		}
	}
	return current, nil
}

func storeRoot(source Source, provider string) (string, error) {
	root := source.dataDir(provider)
	if root == "" {
		return "", errors.New("数据源目录为空")
	}
	return filepath.EvalSymlinks(source.filePath(root))
}

func lockStore(root, provider string) (func(), error) {
	dir, err := ownedPath(root, ".session-desk-trash")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	var handles []*os.File
	release := func() {
		for i := len(handles) - 1; i >= 0; i-- {
			handles[i].Close()
		}
	}
	lock, err := os.OpenFile(filepath.Join(dir, "operation.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	handles = append(handles, lock)
	if err := lockFile(lock); err != nil {
		release()
		return nil, errors.New("此数据源有其他回收操作正在进行")
	}
	if provider == "agy" {
		paths, err := filepath.Glob(filepath.Join(root, "presence", "*.lock"))
		if err != nil {
			release()
			return nil, err
		}
		for _, path := range paths {
			file, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				release()
				return nil, err
			}
			handles = append(handles, file)
			if err := lockFile(file); err != nil {
				release()
				return nil, errors.New("请先退出使用此目录的 Antigravity CLI，再删除或恢复会话")
			}
		}
	}
	return release, nil
}

func (s *Service) PrepareDelete(ctx context.Context, keys []string) (DeletePlan, error) {
	if len(keys) == 0 {
		return DeletePlan{}, errors.New("请选择要删除的会话")
	}
	view := s.View()
	sources := map[string]Source{}
	for _, source := range view.Sources {
		sources[source.ID] = source
	}
	local := map[string]bool{}
	remote := map[string][]string{}
	scope := map[string]map[string]bool{}
	requested := map[string]bool{}
	for _, key := range keys {
		parts := strings.Split(key, "|")
		if len(parts) != 3 || !validSessionID(parts[1], parts[2]) {
			return DeletePlan{}, errors.New("会话 ID 无效")
		}
		source, exists := sources[parts[0]]
		if !exists {
			return DeletePlan{}, errors.New("会话数据源已变化，请刷新列表")
		}
		if requested[key] {
			continue
		}
		requested[key] = true
		if s.remoteSource(source) {
			remote[source.ID] = append(remote[source.ID], key)
		} else {
			local[key] = true
			if scope[source.ID] == nil {
				scope[source.ID] = map[string]bool{}
			}
			scope[source.ID][parts[1]] = true
		}
	}
	for sourceID := range remote {
		source := sources[sourceID]
		running, err := s.sourceRunning(ctx, source)
		if err != nil {
			return DeletePlan{}, err
		}
		if !running || source.Error != "" {
			return DeletePlan{}, errors.New("请先启动会话所属的环境并刷新数据源")
		}
	}
	if len(scope) > 0 {
		var err error
		view, err = s.refresh(ctx, scope)
		if err != nil {
			return DeletePlan{}, err
		}
		for _, source := range view.Sources {
			if scope[source.ID] != nil && (!source.Running || source.Error != "") {
				return DeletePlan{}, errors.New("请先启动会话所属的环境并刷新数据源")
			}
		}
		for key := range local {
			if _, _, err := s.Find(key); err != nil {
				return DeletePlan{}, err
			}
		}
	}
	for {
		added := false
		for _, session := range view.Sessions {
			if session.ParentID != "" && local[session.SourceID+"|"+session.Provider+"|"+session.ParentID] && !local[session.Key] {
				local[session.Key] = true
				added = true
			}
		}
		if !added {
			break
		}
	}
	plan := DeletePlan{Keys: []string{}, Targets: []Session{}, Revisions: map[string]string{}}
	for _, session := range view.Sessions {
		if !local[session.Key] {
			continue
		}
		if !validSessionID(session.Provider, session.ID) {
			return plan, errors.New("会话 ID 无效")
		}
		if time.Since(session.UpdatedAt) < time.Minute {
			return plan, fmt.Errorf("“%s”在最近 1 分钟内有写入，请结束会话后稍候重试", session.Title)
		}
		plan.Keys = append(plan.Keys, session.Key)
		plan.Targets = append(plan.Targets, session)
		plan.Revisions[session.Key] = session.Revision
	}
	for id, group := range remote {
		response, err := s.callWorker(ctx, WorkerRequest{Action: "preview", Source: sources[id], Keys: group})
		if err != nil {
			return plan, err
		}
		plan.Keys = append(plan.Keys, response.Plan.Keys...)
		plan.Targets = append(plan.Targets, response.Plan.Targets...)
		for key, revision := range response.Plan.Revisions {
			plan.Revisions[key] = revision
		}
		s.refreshMu.Lock()
		s.updateSessions(nil, response.Plan.Targets)
		s.refreshMu.Unlock()
	}
	slices.Sort(plan.Keys)
	plan.Related = len(plan.Keys) - len(requested)
	return plan, nil
}

func (s *Service) Delete(ctx context.Context, keys []string, revisions map[string]string) MutationResult {
	result := MutationResult{Done: []string{}, Failed: []MutationFailure{}}
	plan, err := s.PrepareDelete(ctx, keys)
	if err != nil {
		result.Failed = append(result.Failed, MutationFailure{Error: err.Error()})
		return result
	}
	expected := slices.Clone(keys)
	slices.Sort(expected)
	if !slices.Equal(expected, plan.Keys) {
		result.Failed = append(result.Failed, MutationFailure{Error: "关联子会话已变化，请重新确认删除"})
		return result
	}
	for _, session := range plan.Targets {
		if revisions[session.Key] != session.Revision {
			result.Failed = append(result.Failed, MutationFailure{Key: session.Key, Error: "会话已变化，请重新确认删除"})
			return result
		}
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	groups := map[string][]Session{}
	sources := map[string]Source{}
	for _, session := range plan.Targets {
		_, source, err := s.Find(session.Key)
		if err != nil {
			result.Failed = append(result.Failed, MutationFailure{Key: session.Key, Error: err.Error()})
			continue
		}
		group := session.SourceID + "|" + session.Provider
		if s.remoteSource(source) {
			group = session.SourceID
		}
		groups[group] = append(groups[group], session)
		sources[group] = source
	}
	for group, sessions := range groups {
		source := sources[group]
		var err error
		if s.remoteSource(source) {
			var selected []string
			for _, session := range sessions {
				selected = append(selected, session.Key)
			}
			response, callErr := s.callWorker(ctx, WorkerRequest{Action: "delete", Source: source, Keys: selected, Revisions: revisions})
			if callErr != nil {
				err = callErr
			} else {
				result.Done = append(result.Done, response.Result.Done...)
				result.Failed = append(result.Failed, response.Result.Failed...)
				continue
			}
		} else {
			err = s.deleteLocal(ctx, source, sessions)
		}
		for _, session := range sessions {
			if err != nil {
				result.Failed = append(result.Failed, MutationFailure{Key: session.Key, Error: err.Error()})
			} else {
				result.Done = append(result.Done, session.Key)
			}
		}
	}
	s.updateSessions(result.Done, nil)
	return result
}

func (s *Service) sessionFiles(source Source, sessions []Session, root string) ([]string, error) {
	var candidates []string
	provider := sessions[0].Provider
	for _, session := range sessions {
		if provider == "claude" || provider == "codex" {
			found := false
			for _, cached := range s.cache {
				if cached.Session.Key == session.Key {
					candidates = append(candidates, cached.Session.Path)
					found = true
				}
			}
			if !found {
				candidates = append(candidates, session.Path)
			}
		}
		if provider == "opencode" && session.Store == "opencode-json" {
			candidates = append(candidates, session.Paths...)
		}
		if provider == "claude" {
			for _, path := range slices.Clone(candidates) {
				if strings.HasSuffix(path, ".jsonl") {
					candidates = append(candidates, strings.TrimSuffix(path, ".jsonl"))
				}
			}
			candidates = append(candidates, filepath.Join(source.filePath(source.ClaudeDir), "file-history", session.ID), filepath.Join(source.filePath(source.ClaudeDir), "session-env", session.ID))
			todos, err := filepath.Glob(filepath.Join(source.filePath(source.ClaudeDir), "todos", session.ID+"*.json"))
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, todos...)
		}
		if provider == "agy" {
			base := source.filePath(source.AgyDir)
			candidates = append(candidates, filepath.Join(base, "brain", session.ID), filepath.Join(base, "annotations", session.ID+".pbtxt"))
			for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
				candidates = append(candidates, filepath.Join(base, "conversations", session.ID+".db"+suffix))
			}
		}
	}
	var out []string
	seen := map[string]bool{}
	logicalRoot := source.filePath(source.dataDir(provider))
	for _, candidate := range candidates {
		relative, err := filepath.Rel(logicalRoot, candidate)
		if err != nil {
			return nil, err
		}
		path, err := ownedPath(root, relative)
		if err != nil {
			return nil, err
		}
		stat, err := os.Lstat(path)
		if missingStore(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !stat.IsDir() && time.Since(stat.ModTime()) < time.Minute {
			return nil, errors.New("会话文件最近有写入，请结束所选 CLI 后稍候重试")
		}
		if stat.IsDir() {
			if err := filepath.WalkDir(path, func(child string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
					return nil
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if time.Since(info.ModTime()) < time.Minute {
					return errors.New("关联会话文件最近有写入，请结束所选 CLI 后稍候重试")
				}
				return nil
			}); err != nil {
				return nil, err
			}
		}
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	slices.SortFunc(out, func(a, b string) int { return len(a) - len(b) })
	var compact []string
	for _, candidate := range out {
		nested := false
		for _, parent := range compact {
			if strings.HasPrefix(candidate, parent+string(filepath.Separator)) {
				nested = true
				break
			}
		}
		if !nested {
			compact = append(compact, candidate)
		}
	}
	return compact, nil
}

func databasePaths(root, provider string) ([]string, error) {
	var out []string
	if provider == "opencode" {
		out = []string{filepath.Join(root, "opencode.db")}
	}
	if provider == "agy" {
		out = []string{filepath.Join(root, "conversation_summaries.db")}
	}
	if provider == "codex" {
		for _, pattern := range []string{"state_*.sqlite", "memories_*.sqlite", "sqlite/codex*.db"} {
			paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
			if err != nil {
				return nil, err
			}
			out = append(out, paths...)
		}
	}
	var present []string
	for _, path := range out {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		if _, err := ownedPath(root, relative); err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); missingStore(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		present = append(present, path)
	}
	slices.Sort(present)
	return present, nil
}

func (s *Service) deleteLocal(ctx context.Context, source Source, targets []Session) error {
	provider := targets[0].Provider
	root, err := storeRoot(source, provider)
	if err != nil {
		return err
	}
	release, err := lockStore(root, provider)
	if err != nil {
		return err
	}
	defer release()
	paths, err := s.sessionFiles(source, targets, root)
	if err != nil {
		return err
	}
	var ids []string
	selected := map[string]bool{}
	for _, session := range targets {
		ids = append(ids, session.ID)
		selected[session.ID] = true
	}
	dbs, err := databasePaths(root, provider)
	if err != nil {
		return err
	}
	var changes []*databaseChange
	defer func() { closeChanges(changes) }()
	for _, path := range dbs {
		change, err := captureDatabase(ctx, path, provider, ids)
		if err != nil {
			return err
		}
		changes = append(changes, change)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	id := hex.EncodeToString(random[:])
	entry := TrashEntry{ID: id, Key: source.ID + "|" + provider + "|" + id, SourceID: source.ID, Provider: provider, Root: root, Title: targets[0].Title, DeletedAt: time.Now(), Sessions: targets, Status: "pending"}
	for index, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry.Files = append(entry.Files, TrashFile{Original: relative, Saved: filepath.Join("files", fmt.Sprintf("%04d", index))})
	}
	for _, change := range changes {
		backup := change.backup
		backup.Path, err = filepath.Rel(root, backup.Path)
		if err != nil {
			return err
		}
		entry.Databases = append(entry.Databases, backup)
	}
	var oldCache, newCache []byte
	cachePath := filepath.Join(root, "jetbox_summaries_proto.pb")
	if provider == "agy" {
		oldCache, err = os.ReadFile(cachePath)
		if err != nil && !missingStore(err) {
			return err
		}
		if err == nil {
			newCache, entry.AgyCache, err = cutAgyCache(oldCache, selected)
			if err != nil {
				return err
			}
		}
	}
	dir := filepath.Join(root, ".session-desk-trash", id)
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		return err
	}
	manifest := filepath.Join(dir, "manifest.json")
	if err := writeJSON(manifest, entry); err != nil {
		return err
	}
	var moved []TrashFile
	cacheWritten := false
	rollback := func(cause error) error {
		var problems []error
		for _, change := range changes {
			if !change.committed {
				change.tx.Rollback()
			} else {
				repair, err := restoreDatabase(context.Background(), change.backup)
				if err != nil {
					problems = append(problems, err)
					continue
				}
				if err := repair.tx.Commit(); err != nil {
					repair.tx.Rollback()
					problems = append(problems, err)
				}
				repair.db.Close()
			}
		}
		if cacheWritten {
			if err := writeBytes(cachePath, oldCache); err != nil {
				problems = append(problems, err)
			}
		}
		for i := len(moved) - 1; i >= 0; i-- {
			file := moved[i]
			destination, _ := ownedPath(root, file.Original)
			if _, err := os.Lstat(destination); !missingStore(err) {
				problems = append(problems, errors.New("原位置出现文件，回收站备份已保留"))
				continue
			}
			if err := os.Rename(filepath.Join(dir, file.Saved), destination); err != nil {
				problems = append(problems, err)
			}
		}
		if len(problems) == 0 {
			if err := os.RemoveAll(dir); err != nil {
				problems = append(problems, err)
			}
		}
		return errors.Join(append([]error{cause}, problems...)...)
	}
	for _, file := range entry.Files {
		original, _ := ownedPath(root, file.Original)
		if err := os.Rename(original, filepath.Join(dir, file.Saved)); err != nil {
			return rollback(err)
		}
		moved = append(moved, file)
	}
	for _, change := range changes {
		if err := change.deleteRows(ctx); err != nil {
			return rollback(err)
		}
	}
	if len(entry.AgyCache) > 0 {
		if err := writeBytes(cachePath, newCache); err != nil {
			return rollback(err)
		}
		cacheWritten = true
	}
	if err := commitChanges(changes); err != nil {
		return rollback(err)
	}
	entry.Status = "trashed"
	if err := writeJSON(manifest, entry); err != nil {
		return rollback(err)
	}
	return nil
}

func writeBytes(path string, data []byte) error {
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

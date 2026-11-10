package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var trashIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *Service) Trash(ctx context.Context) ([]TrashEntry, error) {
	view := s.View()
	var previous []TrashEntry
	cache := filepath.Join(filepath.Dir(s.options.ConfigPath), "trash-cache.json")
	if data, err := os.ReadFile(cache); err == nil {
		if err := json.Unmarshal(data, &previous); err != nil {
			return nil, err
		}
	} else if !missingStore(err) {
		return nil, err
	}
	out := []TrashEntry{}
	for _, source := range view.Sources {
		up, err := s.sourceRunning(ctx, source)
		if err != nil {
			return nil, err
		}
		if !up {
			for _, entry := range previous {
				if entry.SourceID == source.ID {
					entry.Offline = true
					out = append(out, entry)
				}
			}
			continue
		}
		if s.remoteSource(source) {
			response, err := s.callWorker(ctx, WorkerRequest{Action: "trash", Source: source})
			if err != nil {
				return nil, err
			}
			out = append(out, response.Trash...)
			continue
		}
		for _, provider := range Providers {
			if source.dataDir(provider) == "" {
				continue
			}
			root, err := storeRoot(source, provider)
			if missingStore(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			dir := filepath.Join(root, ".session-desk-trash")
			entries, err := os.ReadDir(dir)
			if missingStore(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			for _, folder := range entries {
				if !folder.IsDir() || !trashIDPattern.MatchString(folder.Name()) {
					continue
				}
				path, err := ownedPath(root, filepath.Join(".session-desk-trash", folder.Name(), "manifest.json"))
				if err != nil {
					return nil, err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, err
				}
				var entry TrashEntry
				if err := json.Unmarshal(data, &entry); err != nil {
					return nil, err
				}
				if entry.ID != folder.Name() || entry.SourceID != source.ID || entry.Provider != provider || entry.Key != source.ID+"|"+provider+"|"+entry.ID {
					return nil, errors.New("回收站条目与数据源不一致")
				}
				entry.Root = root
				entry.Files = nil
				entry.Databases = nil
				entry.AgyCache = nil
				for i := range entry.Sessions {
					entry.Sessions[i].Paths = nil
				}
				out = append(out, entry)
			}
		}
	}
	slices.SortFunc(out, func(a, b TrashEntry) int { return b.DeletedAt.Compare(a.DeletedAt) })
	if err := writeJSON(cache, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) findTrash(ctx context.Context, key string) (TrashEntry, Source, error) {
	parts := strings.Split(key, "|")
	if len(parts) != 3 || !slices.Contains(Providers, parts[1]) || !trashIDPattern.MatchString(parts[2]) {
		return TrashEntry{}, Source{}, errors.New("回收站 ID 无效")
	}
	for _, source := range s.View().Sources {
		if source.ID != parts[0] {
			continue
		}
		running, err := s.sourceRunning(ctx, source)
		if err != nil {
			return TrashEntry{}, source, err
		}
		if !running || source.Error != "" {
			return TrashEntry{}, source, errors.New("请先启动此回收站所属的环境并刷新数据源")
		}
		return TrashEntry{ID: parts[2], Key: key, SourceID: source.ID, Provider: parts[1]}, source, nil
	}
	return TrashEntry{}, Source{}, errors.New("未找到回收站数据源，请刷新")
}

func readManifest(source Source, entry TrashEntry) (TrashEntry, string, error) {
	root, err := storeRoot(source, entry.Provider)
	if err != nil {
		return entry, "", err
	}
	if !trashIDPattern.MatchString(entry.ID) {
		return entry, "", errors.New("回收站 ID 无效")
	}
	dir, err := ownedPath(root, filepath.Join(".session-desk-trash", entry.ID))
	if err != nil {
		return entry, "", err
	}
	path, err := ownedPath(root, filepath.Join(".session-desk-trash", entry.ID, "manifest.json"))
	if err != nil {
		return entry, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return entry, "", err
	}
	var manifest TrashEntry
	if err := json.Unmarshal(data, &manifest); err != nil {
		return entry, "", err
	}
	if manifest.ID != entry.ID || manifest.Key != entry.Key || manifest.SourceID != source.ID || manifest.Provider != entry.Provider {
		return entry, "", errors.New("回收站条目无效")
	}
	manifest.Root = root
	for _, session := range manifest.Sessions {
		if session.SourceID != source.ID || session.Provider != manifest.Provider || !validSessionID(session.Provider, session.ID) || session.Key != source.ID+"|"+session.Provider+"|"+session.ID {
			return entry, "", errors.New("回收站会话与数据源不一致")
		}
		for _, path := range append([]string{session.Path}, session.Paths...) {
			relative, err := filepath.Rel(source.filePath(source.dataDir(session.Provider)), path)
			if err != nil {
				return entry, "", err
			}
			if _, err := ownedPath(root, relative); err != nil {
				return entry, "", err
			}
		}
	}
	for _, file := range manifest.Files {
		if !strings.HasPrefix(file.Saved, "files"+string(filepath.Separator)) {
			return entry, "", errors.New("回收站文件路径无效")
		}
		if _, err := ownedPath(root, file.Original); err != nil {
			return entry, "", err
		}
		if _, err := ownedPath(dir, file.Saved); err != nil {
			return entry, "", err
		}
	}
	for _, backup := range manifest.Databases {
		if _, err := ownedPath(root, backup.Path); err != nil {
			return entry, "", err
		}
	}
	return manifest, dir, nil
}

func (s *Service) Restore(ctx context.Context, key string) error {
	entry, source, err := s.findTrash(ctx, key)
	if err != nil {
		return err
	}
	if s.remoteSource(source) {
		response, err := s.callWorker(ctx, WorkerRequest{Action: "restore", Source: source, EntryID: key})
		if err != nil {
			return err
		}
		s.refreshMu.Lock()
		s.updateSessions(nil, response.Snapshot.Sessions)
		s.refreshMu.Unlock()
		return nil
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	manifest, dir, err := readManifest(source, entry)
	if err != nil {
		return err
	}
	release, err := lockStore(manifest.Root, manifest.Provider)
	if err != nil {
		return err
	}
	defer release()
	var files []TrashFile
	for _, file := range manifest.Files {
		original, err := ownedPath(manifest.Root, file.Original)
		if err != nil {
			return err
		}
		saved, err := ownedPath(dir, file.Saved)
		if err != nil {
			return err
		}
		_, originalErr := os.Lstat(original)
		_, savedErr := os.Lstat(saved)
		if savedErr != nil && !missingStore(savedErr) {
			return savedErr
		}
		if originalErr != nil && !missingStore(originalErr) {
			return originalErr
		}
		if originalErr == nil && savedErr == nil {
			return errors.New("原位置已有文件，回收站备份已保留，请先处理冲突")
		}
		if originalErr == nil && missingStore(savedErr) {
			continue
		}
		if missingStore(originalErr) && missingStore(savedErr) {
			return errors.New("回收站备份文件缺失")
		}
		files = append(files, file)
	}
	var changes []*databaseChange
	defer func() { closeChanges(changes) }()
	for _, backup := range manifest.Databases {
		backup.Path, err = ownedPath(manifest.Root, backup.Path)
		if err != nil {
			return err
		}
		change, err := restoreDatabase(ctx, backup)
		if err != nil {
			return err
		}
		changes = append(changes, change)
	}
	var oldCache, newCache []byte
	cachePath := filepath.Join(manifest.Root, "jetbox_summaries_proto.pb")
	if len(manifest.AgyCache) > 0 {
		oldCache, err = os.ReadFile(cachePath)
		if err != nil && !missingStore(err) {
			return err
		}
		newCache, err = restoreAgyCache(oldCache, manifest.AgyCache)
		if err != nil {
			return err
		}
	}
	var moved []TrashFile
	cacheWritten := false
	manifest.Status = "pending"
	if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		return err
	}
	rollback := func(cause error) error {
		var problems []error
		committed := false
		for _, change := range changes {
			if !change.committed {
				change.tx.Rollback()
			} else {
				committed = true
			}
		}
		if committed {
			return errors.Join(cause, errors.New("部分记录已恢复，请在回收站重试恢复"))
		}
		if cacheWritten {
			if err := writeBytes(cachePath, oldCache); err != nil {
				problems = append(problems, err)
			}
		}
		for i := len(moved) - 1; i >= 0; i-- {
			file := moved[i]
			original, _ := ownedPath(manifest.Root, file.Original)
			if err := os.Rename(original, filepath.Join(dir, file.Saved)); err != nil {
				problems = append(problems, err)
			}
		}
		if len(problems) == 0 {
			manifest.Status = "trashed"
			if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
				problems = append(problems, err)
			}
		}
		return errors.Join(append([]error{cause}, problems...)...)
	}
	for _, file := range files {
		original, _ := ownedPath(manifest.Root, file.Original)
		if err := os.MkdirAll(filepath.Dir(original), 0o700); err != nil {
			return rollback(err)
		}
		if err := os.Rename(filepath.Join(dir, file.Saved), original); err != nil {
			return rollback(err)
		}
		moved = append(moved, file)
	}
	if len(manifest.AgyCache) > 0 {
		if err := writeBytes(cachePath, newCache); err != nil {
			return rollback(err)
		}
		cacheWritten = true
	}
	if err := commitChanges(changes); err != nil {
		return rollback(err)
	}
	s.updateSessions(nil, manifest.Sessions)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return nil
}

func (s *Service) Purge(ctx context.Context, key string) error {
	entry, source, err := s.findTrash(ctx, key)
	if err != nil {
		return err
	}
	if s.remoteSource(source) {
		_, err := s.callWorker(ctx, WorkerRequest{Action: "purge", Source: source, EntryID: key})
		return err
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	manifest, dir, err := readManifest(source, entry)
	if err != nil {
		return err
	}
	if manifest.Status != "trashed" {
		return errors.New("请先恢复此条目，再执行彻底删除")
	}
	release, err := lockStore(manifest.Root, manifest.Provider)
	if err != nil {
		return err
	}
	defer release()
	return os.RemoveAll(dir)
}

func (s *Service) RestoreMany(ctx context.Context, keys []string) MutationResult {
	return s.mutateTrash(ctx, keys, "restore")
}

func (s *Service) PurgeMany(ctx context.Context, keys []string) MutationResult {
	return s.mutateTrash(ctx, keys, "purge")
}

func (s *Service) mutateTrash(ctx context.Context, keys []string, action string) MutationResult {
	out := MutationResult{Done: []string{}, Failed: []MutationFailure{}}
	groups := map[string][]string{}
	sources := map[string]Source{}
	for _, source := range s.View().Sources {
		sources[source.ID] = source
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		id, _, _ := strings.Cut(key, "|")
		source, exists := sources[id]
		if !exists {
			out.Failed = append(out.Failed, MutationFailure{Key: key, Error: "未找到回收站数据源，请刷新"})
			continue
		}
		groups[source.ID] = append(groups[source.ID], key)
	}
	for id, group := range groups {
		source := sources[id]
		if s.remoteSource(source) {
			running, err := s.sourceRunning(ctx, source)
			if err == nil && (!running || source.Error != "") {
				err = errors.New("请先启动此回收站所属的环境并刷新数据源")
			}
			var response WorkerResponse
			if err == nil {
				response, err = s.callWorker(ctx, WorkerRequest{Action: action, Source: source, Keys: group})
			}
			if err != nil {
				for _, key := range group {
					out.Failed = append(out.Failed, MutationFailure{Key: key, Error: err.Error()})
				}
				continue
			}
			out.Done = append(out.Done, response.Result.Done...)
			out.Failed = append(out.Failed, response.Result.Failed...)
			if action == "restore" {
				s.refreshMu.Lock()
				s.updateSessions(nil, response.Snapshot.Sessions)
				s.refreshMu.Unlock()
			}
			continue
		}
		for _, key := range group {
			var err error
			if action == "restore" {
				err = s.Restore(ctx, key)
			} else {
				err = s.Purge(ctx, key)
			}
			if err != nil {
				out.Failed = append(out.Failed, MutationFailure{Key: key, Error: err.Error()})
			} else {
				out.Done = append(out.Done, key)
			}
		}
	}
	return out
}

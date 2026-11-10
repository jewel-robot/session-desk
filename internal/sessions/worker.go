package sessions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type WorkerRequest struct {
	Action    string            `json:"action"`
	Source    Source            `json:"source"`
	Key       string            `json:"key,omitempty"`
	Keys      []string          `json:"keys,omitempty"`
	Cursor    int64             `json:"cursor,omitempty"`
	Query     string            `json:"query,omitempty"`
	Provider  string            `json:"provider,omitempty"`
	EntryID   string            `json:"entry_id,omitempty"`
	Revisions map[string]string `json:"revisions,omitempty"`
}

type WorkerResponse struct {
	Snapshot   *Snapshot       `json:"snapshot,omitempty"`
	Transcript *Transcript     `json:"transcript,omitempty"`
	Search     *SearchResult   `json:"search,omitempty"`
	Plan       *DeletePlan     `json:"plan,omitempty"`
	Trash      []TrashEntry    `json:"trash,omitempty"`
	Result     *MutationResult `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}

func HandleWorker(ctx context.Context, request WorkerRequest) (WorkerResponse, error) {
	if !filepath.IsAbs(request.Source.Home) {
		return WorkerResponse{}, errors.New("WSL 用户目录无效")
	}
	config := filepath.Join(request.Source.Home, ".local", "share", "session-desk", "worker", "settings.json")
	service, err := New(Options{ConfigPath: config})
	if err != nil {
		return WorkerResponse{}, err
	}
	service.fixed = []Source{request.Source}
	service.view.Sources = []Source{request.Source}
	var out WorkerResponse
	switch request.Action {
	case "list":
		view, err := service.Refresh(ctx)
		if err != nil {
			return out, err
		}
		out.Snapshot = &view
	case "transcript":
		if _, err := service.Refresh(ctx); err != nil {
			return out, err
		}
		value, err := service.Transcript(ctx, request.Key, request.Cursor)
		if err != nil {
			return out, err
		}
		out.Transcript = &value
	case "search":
		if _, err := service.Refresh(ctx); err != nil {
			return out, err
		}
		value := service.Search(ctx, request.Query, request.Provider, request.Source.ID)
		out.Search = &value
	case "preview":
		value, err := service.PrepareDelete(ctx, request.Keys)
		if err != nil {
			return out, err
		}
		out.Plan = &value
	case "delete":
		value := service.Delete(ctx, request.Keys, request.Revisions)
		out.Result = &value
	case "trash":
		value, err := service.Trash(ctx)
		if err != nil {
			return out, err
		}
		out.Trash = value
	case "restore":
		if len(request.Keys) > 0 {
			value := service.RestoreMany(ctx, request.Keys)
			out.Result = &value
		} else if err := service.Restore(ctx, request.EntryID); err != nil {
			return out, err
		}
		view := service.View()
		out.Snapshot = &view
	case "purge":
		if len(request.Keys) > 0 {
			value := service.PurgeMany(ctx, request.Keys)
			out.Result = &value
		} else if err := service.Purge(ctx, request.EntryID); err != nil {
			return out, err
		}
	default:
		return out, errors.New("WSL 操作无效")
	}
	return out, nil
}

func (s *Service) remoteSource(source Source) bool {
	return runtime.GOOS == "windows" && source.Kind == "wsl" && !s.options.Demo
}

func workerExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(executable), "session-desk-worker")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("请完整解压发行包，缺少 WSL 读取程序：%w", err)
	}
	hash := sha256.Sum256(data)
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(cache, "SessionDesk", "workers", hex.EncodeToString(hash[:8]), "session-desk-worker")
	if stat, err := os.Stat(target); err == nil && stat.Size() == int64(len(data)) {
		return target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		return "", err
	}
	return target, nil
}

func (s *Service) callWorker(ctx context.Context, request WorkerRequest) (WorkerResponse, error) {
	var out WorkerResponse
	s.mu.RLock()
	linuxPath := s.workerPaths[request.Source.Distro]
	executable := s.workerPath
	s.mu.RUnlock()
	if linuxPath == "" {
		var err error
		if executable == "" {
			executable, err = workerExecutable()
			if err != nil {
				return out, err
			}
		}
		path, err := runOutput(ctx, "wsl.exe", "--distribution", request.Source.Distro, "--exec", "wslpath", "-u", executable)
		if err != nil {
			return out, err
		}
		linuxPath = strings.TrimSpace(string(path))
		if !strings.HasPrefix(linuxPath, "/") || strings.ContainsAny(linuxPath, "\r\n\x00") {
			return out, errors.New("WSL 读取程序路径无效")
		}
		s.mu.Lock()
		s.workerPath = executable
		s.workerPaths[request.Source.Distro] = linuxPath
		s.mu.Unlock()
	}
	input, err := json.Marshal(request)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "--distribution", request.Source.Distro, "--exec", linuxPath)
	configureCommand(cmd)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("WSL 操作失败：%w %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(output, &out); err != nil {
		return out, fmt.Errorf("WSL 响应无效：%w", err)
	}
	if out.Error != "" {
		return out, errors.New(out.Error)
	}
	return out, nil
}

func RunWorker(input io.Reader, output io.Writer) error {
	var request WorkerRequest
	decoder := json.NewDecoder(io.LimitReader(input, 4*1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return err
	}
	response, err := HandleWorker(context.Background(), request)
	if err != nil {
		response = WorkerResponse{Error: err.Error()}
	}
	return json.NewEncoder(output).Encode(response)
}

package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Install phases reported to the interface.
const (
	PhaseIdle        = "idle"
	PhaseDownloading = "downloading"
	PhaseLaunching   = "launching"
	PhaseLaunched    = "launched"
	PhaseError       = "error"
)

// maxInstallerSize bounds the download in case the asset metadata is wrong.
const maxInstallerSize = 512 << 20

var (
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	assetPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+-setup\.exe$`)
)

// InstallState is the progress of an update installation.
type InstallState struct {
	Phase    string `json:"phase"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Error    string `json:"error,omitempty"`
}

// Installer downloads the release installer, checks it against the SHA-256 digest GitHub publishes
// for the asset, and starts it silently. Nothing is run unless the digest matches.
type Installer struct {
	Client        *http.Client
	AllowedPrefix string // download URLs must start with this
	Dir           string // download directory
	// Launch starts the verified installer; Done runs after a successful launch (the app quits there).
	Launch func(path string) error
	Done   func()

	mu      sync.Mutex
	state   InstallState
	running bool
}

// NewInstaller returns an installer for release assets of repository ("owner/name").
func NewInstaller(repository string) *Installer {
	return &Installer{
		Client:        &http.Client{Timeout: 15 * time.Minute},
		AllowedPrefix: "https://github.com/" + repository + "/releases/download/",
		Dir:           filepath.Join(os.TempDir(), "SessionDesk-update"),
		Launch:        launch,
	}
}

// State returns the current progress.
func (i *Installer) State() InstallState {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state.Phase == "" {
		return InstallState{Phase: PhaseIdle}
	}
	return i.state
}

// Start validates the release and installs it in the background.
func (i *Installer) Start(result Result) error {
	switch {
	case result.Status != StatusAvailable:
		return errors.New("没有可安装的新版本")
	case !assetPattern.MatchString(result.AssetName):
		return errors.New("发布中没有安装程序")
	case !strings.HasPrefix(result.DownloadURL, i.AllowedPrefix):
		return errors.New("下载地址不在发布仓库中")
	case !digestPattern.MatchString(result.Digest):
		return errors.New("发布缺少安装程序校验值")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.running {
		return errors.New("更新正在进行")
	}
	i.running = true
	i.state = InstallState{Phase: PhaseDownloading, Total: result.Size}
	go i.run(result)
	return nil
}

func (i *Installer) run(result Result) {
	path, err := i.download(result)
	if err == nil {
		i.update(func(s *InstallState) { s.Phase = PhaseLaunching })
		err = i.Launch(path)
	}
	i.mu.Lock()
	i.running = false
	if err != nil {
		i.state.Phase, i.state.Error = PhaseError, err.Error()
	} else {
		i.state.Phase = PhaseLaunched
	}
	i.mu.Unlock()
	if err == nil && i.Done != nil {
		i.Done()
	}
}

func (i *Installer) update(change func(*InstallState)) {
	i.mu.Lock()
	change(&i.state)
	i.mu.Unlock()
}

func (i *Installer) download(result Result) (string, error) {
	if err := os.MkdirAll(i.Dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建下载目录：%w", err)
	}
	target := filepath.Join(i.Dir, result.AssetName)
	partial := target + ".part"
	response, err := i.Client.Get(result.DownloadURL)
	if err != nil {
		return "", errors.New("下载安装程序失败，请检查网络后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载安装程序失败：状态 %d", response.StatusCode)
	}
	if response.ContentLength > 0 {
		i.update(func(s *InstallState) { s.Total = response.ContentLength })
	}
	file, err := os.Create(partial)
	if err != nil {
		return "", fmt.Errorf("无法写入安装程序：%w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash, progress(i)), io.LimitReader(response.Body, maxInstallerSize+1))
	closeErr := file.Close()
	switch {
	case copyErr != nil:
		os.Remove(partial)
		return "", errors.New("下载安装程序中断，请重试")
	case closeErr != nil:
		os.Remove(partial)
		return "", fmt.Errorf("无法写入安装程序：%w", closeErr)
	case written > maxInstallerSize:
		os.Remove(partial)
		return "", errors.New("安装程序大小异常")
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != result.Digest {
		os.Remove(partial)
		return "", errors.New("安装程序校验失败")
	}
	os.Remove(target)
	if err := os.Rename(partial, target); err != nil {
		os.Remove(partial)
		return "", fmt.Errorf("无法保存安装程序：%w", err)
	}
	return target, nil
}

type progressWriter struct{ installer *Installer }

func progress(i *Installer) io.Writer { return progressWriter{i} }

func (p progressWriter) Write(data []byte) (int, error) {
	p.installer.update(func(s *InstallState) { s.Received += int64(len(data)) })
	return len(data), nil
}

// launch starts the installer silently; /RELAUNCH makes it start Session Desk again when it finishes.
func launch(path string) error {
	if runtime.GOOS != "windows" {
		return errors.New("仅 Windows 支持自动安装")
	}
	command := exec.Command(path, "/S", "/RELAUNCH")
	if err := command.Start(); err != nil {
		return fmt.Errorf("无法启动安装程序：%w", err)
	}
	return command.Process.Release()
}

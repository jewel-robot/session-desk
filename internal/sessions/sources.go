package sessions

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

func discoverSources(ctx context.Context, homeOverride string, previous []Source) ([]Source, error) {
	home := homeOverride
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	local := Source{ID: "local", Name: "本机", Kind: "local", Home: home, Running: true}
	if runtime.GOOS == "windows" {
		local.ID, local.Name, local.Kind = "windows:local", "Windows", "windows"
	} else if distro := os.Getenv("WSL_DISTRO_NAME"); distro != "" {
		local.ID, local.Name, local.Kind, local.Distro = "wsl:"+distro, distro, "wsl", distro
	}
	local.ClaudeDir, local.CodexDir = filepath.Join(home, ".claude"), filepath.Join(home, ".codex")
	local.OpenCodeDir = filepath.Join(home, ".local", "share", "opencode")
	local.AgyDir = filepath.Join(home, ".gemini", "antigravity-cli")
	if homeOverride == "" {
		if value := os.Getenv("CLAUDE_CONFIG_DIR"); value != "" {
			local.ClaudeDir = value
		}
		if value := os.Getenv("CODEX_HOME"); value != "" {
			local.CodexDir = value
		}
		if value := os.Getenv("XDG_DATA_HOME"); value != "" {
			local.OpenCodeDir = filepath.Join(value, "opencode")
		}
		if value := os.Getenv("AGY_HOME"); value != "" {
			local.AgyDir = value
		}
	}
	out := []Source{local}
	if runtime.GOOS != "windows" || homeOverride != "" {
		return out, nil
	}
	names, err := runOutput(ctx, "wsl.exe", "--list", "--quiet")
	if err != nil {
		for _, source := range previous {
			if source.Kind == "wsl" {
				source.Running, source.Error = false, "WSL 发现失败："+err.Error()
				out = append(out, source)
			}
		}
		return out, fmt.Errorf("WSL 发现失败：%w", err)
	}
	running, err := runOutput(ctx, "wsl.exe", "--list", "--running", "--quiet")
	if err != nil {
		return out, fmt.Errorf("WSL 运行状态读取失败：%w", err)
	}
	up := map[string]bool{}
	for _, name := range distroNames(running) {
		up[name] = true
	}
	known := map[string]Source{}
	for _, source := range previous {
		known[source.Distro] = source
	}
	for _, name := range distroNames(names) {
		s := known[name]
		s.ID, s.Name, s.Kind, s.Distro, s.Running, s.Error = "wsl:"+name, name, "wsl", name, up[name], ""
		if s.Running {
			data, err := runOutput(ctx, "wsl.exe", "--distribution", name, "--exec", "sh", "-c", `printf '%s' "$HOME"`)
			if err != nil {
				s.Error = "用户目录读取失败：" + err.Error()
			} else {
				home := strings.TrimSpace(string(data))
				if strings.HasPrefix(home, "/") && !strings.ContainsAny(home, "\r\n\x00") {
					s.Home = home
				} else {
					s.Error = "WSL 返回的用户目录无效"
				}
			}
		}
		if s.Home != "" {
			s.ClaudeDir, s.CodexDir = strings.TrimRight(s.Home, "/")+"/.claude", strings.TrimRight(s.Home, "/")+"/.codex"
			s.OpenCodeDir = strings.TrimRight(s.Home, "/") + "/.local/share/opencode"
			s.AgyDir = strings.TrimRight(s.Home, "/") + "/.gemini/antigravity-cli"
		}
		out = append(out, s)
	}
	return out, nil
}

func runOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	configureCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		message := strings.TrimSpace(decodeWSL(output))
		if len(message) > 600 {
			message = message[:600]
		}
		return nil, fmt.Errorf("%w %s", err, message)
	}
	return output, nil
}

func distroNames(data []byte) []string {
	var names []string
	for _, name := range strings.Split(decodeWSL(data), "\n") {
		name = strings.TrimSpace(strings.Trim(name, "\x00\ufeff\r"))
		if name != "" && !strings.HasPrefix(strings.ToLower(name), "docker-desktop") {
			names = append(names, name)
		}
	}
	return names
}

func decodeWSL(data []byte) string {
	if len(data) < 2 || !(data[0] == 0xff && data[1] == 0xfe || bytes.IndexByte(data, 0) >= 0) {
		return string(data)
	}
	if data[0] == 0xff && data[1] == 0xfe {
		data = data[2:]
	}
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		units = append(units, binary.LittleEndian.Uint16(data[i:]))
	}
	return string(utf16.Decode(units))
}

func (s *Service) sourceRunning(ctx context.Context, source Source) (bool, error) {
	if runtime.GOOS != "windows" || source.Kind != "wsl" || s.options.Demo {
		return source.Running, nil
	}
	data, err := runOutput(ctx, "wsl.exe", "--list", "--running", "--quiet")
	if err != nil {
		return false, err
	}
	for _, name := range distroNames(data) {
		if name == source.Distro {
			return true, nil
		}
	}
	return false, nil
}

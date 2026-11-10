package sessions

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf16"
)

var sessionIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var openCodeIDPattern = regexp.MustCompile(`^ses_[a-zA-Z0-9]+$`)

func validSessionID(provider, id string) bool {
	if provider == "opencode" {
		return openCodeIDPattern.MatchString(id)
	}
	return (provider == "claude" || provider == "codex" || provider == "agy") && sessionIDPattern.MatchString(id)
}

type ResumePlan struct {
	Command    string   `json:"command"`
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments"`
}

func resumePlan(session Session, source Source) (ResumePlan, error) {
	if !validSessionID(session.Provider, session.ID) {
		return ResumePlan{}, errors.New("会话 ID 无效，无法生成续接命令")
	}
	if strings.ContainsAny(session.Cwd, "\x00\r\n") {
		return ResumePlan{}, errors.New("会话工作目录无效")
	}
	var run string
	var dataDir, homeVariable string
	var xdgHome string
	switch session.Provider {
	case "claude":
		run = "claude --resume " + session.ID
		dataDir, homeVariable = source.ClaudeDir, "CLAUDE_CONFIG_DIR"
	case "codex":
		run = "codex resume " + session.ID
		dataDir, homeVariable = source.CodexDir, "CODEX_HOME"
	case "opencode":
		run = "opencode --session " + session.ID
		dataDir = filepath.Join(source.OpenCodeDir, "opencode.db")
		xdgHome = filepath.Dir(source.OpenCodeDir)
		if source.Kind == "wsl" {
			dataDir = path.Join(source.OpenCodeDir, "opencode.db")
			xdgHome = path.Dir(source.OpenCodeDir)
		}
		homeVariable = "OPENCODE_DB"
	case "agy":
		run = "agy --conversation " + session.ID
		dataDir, homeVariable = source.AgyDir, "AGY_HOME"
		if session.Cwd == "" {
			session.Cwd = source.Home
		}
	default:
		return ResumePlan{}, errors.New("该工具尚未接入继续会话")
	}
	psQuote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	plan := ResumePlan{Executable: "wt.exe"}
	if source.Kind == "wsl" {
		if source.Distro == "" || strings.ContainsAny(source.Distro, "\x00\r\n") || !strings.HasPrefix(session.Cwd, "/") {
			return ResumePlan{}, errors.New("WSL 发行版或工作目录无效")
		}
		prefix := ""
		if dataDir != "" {
			prefix = "env " + psQuote("SESSION_DESK_DATA_HOME="+dataDir) + " "
			run = homeVariable + "=${SESSION_DESK_DATA_HOME} " + run
		}
		if xdgHome != "" {
			prefix += "env " + psQuote("SESSION_DESK_XDG_HOME="+xdgHome) + " "
			run = "XDG_DATA_HOME=${SESSION_DESK_XDG_HOME} " + run
		}
		shellRun := "exec ${SHELL:-sh} -lic '" + run + "'"
		plan.Command = "wsl.exe --distribution " + psQuote(source.Distro) + " --cd " + psQuote(session.Cwd) + " --exec " + prefix + "sh -lc " + psQuote(shellRun)
		plan.Arguments = terminalArguments(plan.Command)
		return plan, nil
	}
	if source.Kind == "windows" {
		// npm's .ps1 shims are blocked by Restricted execution policies; use the .cmd or native executable.
		name, arguments, _ := strings.Cut(run, " ")
		run = "& (Get-Command " + name + " -CommandType Application -TotalCount 1 -ErrorAction Stop).Source " + arguments
		plan.Command = "Set-Location -LiteralPath " + psQuote(session.Cwd) + " -ErrorAction Stop; " + run
		if dataDir != "" {
			plan.Command = "$env:" + homeVariable + " = " + psQuote(dataDir) + "; " + plan.Command
		}
		if xdgHome != "" {
			plan.Command = "$env:XDG_DATA_HOME = " + psQuote(xdgHome) + "; " + plan.Command
		}
		plan.Arguments = terminalArguments(plan.Command)
		return plan, nil
	}
	plan.Command = "cd '" + strings.ReplaceAll(session.Cwd, "'", "'\"'\"'") + "' && " + run
	return plan, nil
}

// Only the encoded script carries project paths through Windows Terminal's parser.
func terminalArguments(command string) []string {
	shell := "powershell.exe"
	if pwsh, err := exec.LookPath("pwsh.exe"); err == nil {
		shell = pwsh
	}
	encoded := base64.StdEncoding.EncodeToString(utf16LE(command))
	return []string{"-w", "0", "new-tab", "--title", "Session Desk", shell, "-NoLogo", "-NoExit", "-EncodedCommand", encoded}
}

func utf16LE(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[i*2:], unit)
	}
	return data
}

func (s *Service) Plan(key string) (ResumePlan, error) {
	session, source, err := s.Find(key)
	if err != nil {
		return ResumePlan{}, err
	}
	return resumePlan(session, source)
}

func (s *Service) Resume(ctx context.Context, key string) error {
	if runtime.GOOS != "windows" || s.options.Demo {
		return errors.New("请在 Windows 桌面应用中继续会话，当前可以复制续接命令")
	}
	session, source, err := s.Find(key)
	if err != nil {
		return err
	}
	plan, err := resumePlan(session, source)
	if err != nil {
		return err
	}
	if source.Kind != "wsl" {
		stat, err := os.Stat(session.Cwd)
		if err != nil || !stat.IsDir() {
			return errors.New("原项目目录已不存在，请先恢复目录")
		}
	}
	if _, err := exec.LookPath("wt.exe"); err != nil {
		return errors.New("未找到 Windows Terminal，请安装后重试，或复制续接命令")
	}
	_, err = runOutput(ctx, plan.Executable, plan.Arguments...)
	return err
}

func (s *Service) CopyCommand(ctx context.Context, key string) error {
	if runtime.GOOS != "windows" {
		return errors.New("当前环境请使用浏览器复制")
	}
	plan, err := s.Plan(key)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "clip.exe")
	configureCommand(cmd)
	cmd.Stdin = strings.NewReader(string(append([]byte{0xff, 0xfe}, utf16LE(plan.Command)...)))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("复制失败：%w %s", err, strings.TrimSpace(decodeWSL(output)))
	}
	return nil
}

func (s *Service) StartSource(ctx context.Context, id string) error {
	if runtime.GOOS != "windows" || s.options.Demo {
		return errors.New("请在 Windows 应用中启动 WSL")
	}
	for _, source := range s.View().Sources {
		if source.ID == id && source.Kind == "wsl" {
			_, err := runOutput(ctx, "wsl.exe", "--distribution", source.Distro, "--exec", "true")
			return err
		}
	}
	return errors.New("未找到 WSL 数据源")
}

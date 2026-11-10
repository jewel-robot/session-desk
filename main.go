package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"sessiondesk/internal/applog"
	"sessiondesk/internal/buildinfo"
	"sessiondesk/internal/prefs"
	"sessiondesk/internal/sessions"
	"sessiondesk/internal/update"
	"sessiondesk/internal/web"
)

//go:embed all:frontend/dist
var frontend embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	serve := flag.Bool("serve", false, "在本地浏览器中运行")
	listen := flag.String("listen", "127.0.0.1:7435", "浏览器调试地址")
	config := flag.String("config", "", "应用设置文件路径")
	home := flag.String("home", "", "指定本机数据源的用户目录")
	demo := flag.Bool("demo", false, "载入演示会话")
	showVersion := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	info := buildinfo.Current()
	if *showVersion {
		fmt.Printf("Session Desk %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date)
		return
	}
	desktop := !*serve && desktopSupported

	configPath := *config
	if configPath == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			fatal(fmt.Errorf("无法定位用户配置目录：%w", err))
		}
		configPath = filepath.Join(dir, "SessionDesk", "settings.json")
	}
	configDir := filepath.Dir(configPath)

	// The windowed build has no console, so keep a small rotating log file next to the settings.
	logPath := ""
	if desktop {
		if path, closer, err := applog.Open(filepath.Join(configDir, "logs")); err == nil {
			logPath = path
			defer closer.Close()
		}
	}
	log.Printf("Session Desk %s (commit %s) starting", info.Version, info.Commit)

	service, err := sessions.New(sessions.Options{ConfigPath: configPath, Home: *home, Demo: *demo})
	if err != nil {
		fatal(err)
	}
	assets, err := fs.Sub(frontend, "frontend/dist")
	if err != nil {
		fatal(err)
	}
	preferences := prefs.Open(filepath.Join(configDir, "preferences.json"))
	options := web.Options{
		Build:       info,
		Preferences: preferences,
		Desktop:     desktop,
		Demo:        *demo,
		ConfigDir:   configDir,
		LogPath:     logPath,
	}
	if buildinfo.UpdatesEnabled() {
		options.Updates = update.NewChecker(buildinfo.Repository, info.Version)
	}
	if !desktop {
		host, _, err := net.SplitHostPort(*listen)
		if err != nil || host != "127.0.0.1" && host != "localhost" && host != "::1" {
			log.Fatal("浏览器调试地址需要使用本机回环地址")
		}
		server := http.Server{Addr: *listen, Handler: web.NewHandler(service, assets, options), ReadHeaderTimeout: 5 * time.Second}
		fmt.Fprintf(os.Stderr, "Session Desk: http://%s\n", *listen)
		log.Fatal(server.ListenAndServe())
	}
	options.OpenURL = openURL
	if err := runDesktop(web.NewHandler(service, assets, options), preferences, appIcon); err != nil {
		fatal(err)
	}
}

// fatal logs the error and, in the windowed build, shows it in a message box instead of a console.
func fatal(err error) {
	log.Print(err)
	showError("Session Desk 启动失败", err.Error())
	os.Exit(1)
}

package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"sessiondesk/internal/buildinfo"
	"sessiondesk/internal/prefs"
	"sessiondesk/internal/update"
)

// Options carries application-level services shared by the desktop window and the browser preview.
type Options struct {
	Build       buildinfo.Info
	Preferences *prefs.Store
	Updates     *update.Checker
	// Desktop is true inside the native window.
	Desktop bool
	// Demo disables automatic update checks so demo runs and tests stay offline.
	Demo bool
	// ConfigDir and LogPath are shown in the About dialog.
	ConfigDir string
	LogPath   string
	// OpenURL opens a link in the system browser; nil in browser preview mode.
	OpenURL func(string) error
}

type appInfo struct {
	buildinfo.Info
	Repository  string `json:"repository"`
	Desktop     bool   `json:"desktop"`
	UpdateCheck bool   `json:"update_check"`
	ConfigDir   string `json:"config_dir"`
	LogPath     string `json:"log_path"`
	CanOpenURL  bool   `json:"can_open_url"`
}

func registerApp(mux *http.ServeMux, options Options) {
	mux.HandleFunc("GET /api/app", func(w http.ResponseWriter, r *http.Request) {
		info := appInfo{Info: options.Build, Repository: buildinfo.RepositoryURL(), Desktop: options.Desktop, ConfigDir: options.ConfigDir, LogPath: options.LogPath, CanOpenURL: options.OpenURL != nil, UpdateCheck: true}
		if options.Preferences != nil {
			info.UpdateCheck = options.Preferences.Get().AutoUpdateCheck()
		}
		respond(w, info)
	})
	mux.HandleFunc("POST /api/app/preferences", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			UpdateCheck *bool `json:"update_check"`
		}
		if !decode(w, r, &in) {
			return
		}
		if options.Preferences == nil {
			fail(w, fmt.Errorf("当前模式不支持保存偏好设置"))
			return
		}
		if err := options.Preferences.Update(func(p *prefs.Preferences) {
			if in.UpdateCheck != nil {
				value := *in.UpdateCheck
				p.UpdateCheck = &value
			}
		}); err != nil {
			fail(w, err)
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
	// GET /api/update checks automatically (respecting preferences and demo mode); ?force=1 is a manual check.
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		force := r.URL.Query().Get("force") == "1"
		current := options.Build.Version
		if options.Updates == nil || (!force && (options.Demo || (options.Preferences != nil && !options.Preferences.Get().AutoUpdateCheck()))) {
			respond(w, update.Result{Status: update.StatusDisabled, Current: current})
			return
		}
		respond(w, options.Updates.Check(context.WithoutCancel(r.Context()), force))
	})
	mux.HandleFunc("POST /api/open-url", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		if !decode(w, r, &in) {
			return
		}
		parsed, err := url.Parse(in.URL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || strings.ContainsAny(in.URL, "\x00\r\n") {
			fail(w, fmt.Errorf("只能打开 http 或 https 链接"))
			return
		}
		if options.OpenURL == nil {
			fail(w, fmt.Errorf("当前模式请在浏览器中打开链接"))
			return
		}
		if err := options.OpenURL(parsed.String()); err != nil {
			fail(w, fmt.Errorf("打开链接失败：%w", err))
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
}

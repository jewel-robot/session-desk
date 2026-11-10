package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sessiondesk/internal/sessions"
)

// Handler serves the API and assets without application-level services (used by tests).
func Handler(service *sessions.Service, assets fs.FS) http.Handler {
	return NewHandler(service, assets, Options{})
}

// NewHandler serves the session API, application services such as version and update checks, and the assets.
func NewHandler(service *sessions.Service, assets fs.FS, options Options) http.Handler {
	mux := http.NewServeMux()
	registerApp(mux, options)
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		view := service.View()
		if view.UpdatedAt.IsZero() {
			var err error
			view, err = service.Refresh(r.Context())
			if err != nil {
				fail(w, err)
				return
			}
		}
		respond(w, view)
	})
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		view, err := service.Refresh(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, view)
	})
	mux.HandleFunc("GET /api/transcript", func(w http.ResponseWriter, r *http.Request) {
		cursor := int64(0)
		if value := r.URL.Query().Get("cursor"); value != "" {
			var err error
			cursor, err = strconv.ParseInt(value, 10, 64)
			if err != nil || cursor < 0 {
				fail(w, fmt.Errorf("正文分页位置无效"))
				return
			}
		}
		transcript, err := service.Transcript(r.Context(), r.URL.Query().Get("key"), cursor)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, transcript)
	})
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if len([]rune(q.Get("q"))) > 300 {
			fail(w, fmt.Errorf("搜索内容超过 300 个字符"))
			return
		}
		respond(w, service.Search(r.Context(), q.Get("q"), q.Get("provider"), q.Get("source")))
	})
	mux.HandleFunc("GET /api/command", func(w http.ResponseWriter, r *http.Request) {
		plan, err := service.Plan(r.URL.Query().Get("key"))
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, plan)
	})
	for _, action := range []string{"resume", "copy", "sources/start"} {
		mux.HandleFunc("POST /api/"+action, func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Key string `json:"key"`
				ID  string `json:"id"`
			}
			if !decode(w, r, &in) {
				return
			}
			var err error
			switch action {
			case "resume":
				err = service.Resume(r.Context(), in.Key)
			case "copy":
				err = service.CopyCommand(r.Context(), in.Key)
			case "sources/start":
				err = service.StartSource(r.Context(), in.ID)
			}
			if err != nil {
				fail(w, err)
				return
			}
			respond(w, map[string]bool{"ok": true})
		})
	}
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		respond(w, service.Settings())
	})
	mux.HandleFunc("GET /api/trash", func(w http.ResponseWriter, r *http.Request) {
		entries, err := service.Trash(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, map[string]any{"entries": entries})
	})
	mux.HandleFunc("POST /api/delete/preview", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Keys []string `json:"keys"`
		}
		if !decode(w, r, &in) {
			return
		}
		plan, err := service.PrepareDelete(r.Context(), in.Keys)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, plan)
	})
	mux.HandleFunc("POST /api/delete", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Keys      []string          `json:"keys"`
			Revisions map[string]string `json:"revisions"`
		}
		if !decode(w, r, &in) {
			return
		}
		respond(w, service.Delete(r.Context(), in.Keys, in.Revisions))
	})
	mux.HandleFunc("POST /api/trash/restore", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Keys []string `json:"keys"`
		}
		if !decode(w, r, &in) {
			return
		}
		respond(w, service.RestoreMany(r.Context(), in.Keys))
	})
	mux.HandleFunc("POST /api/trash/purge", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Keys []string `json:"keys"`
		}
		if !decode(w, r, &in) {
			return
		}
		respond(w, service.PurgeMany(r.Context(), in.Keys))
	})
	mux.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var settings sessions.Settings
		if !decode(w, r, &settings) {
			return
		}
		if err := service.SaveSettings(settings); err != nil {
			fail(w, err)
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		session, _, err := service.Find(key)
		if err != nil {
			fail(w, err)
			return
		}
		var out strings.Builder
		fmt.Fprintf(&out, "# %s\n\n工具：%s\n\n工作目录：`%s`\n\n", session.Title, session.Provider, session.Cwd)
		if session.Store == "agy-summary" {
			out.WriteString("此记录保留会话摘要。可在 Antigravity CLI 中续接并查看完整对话。\n\n")
		}
		cursor := int64(0)
		for {
			transcript, err := service.Transcript(r.Context(), key, cursor)
			if err != nil {
				fail(w, err)
				return
			}
			for _, part := range transcript.Parts {
				fmt.Fprintf(&out, "## %s", part.Role)
				if part.Kind != "text" {
					fmt.Fprintf(&out, " · %s", part.Kind)
				}
				if part.Name != "" {
					fmt.Fprintf(&out, " · %s", part.Name)
				}
				fmt.Fprintf(&out, "\n\n%s\n\n", part.Text)
			}
			if !transcript.HasMore || transcript.NextCursor == cursor {
				break
			}
			cursor = transcript.NextCursor
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="session-`+time.Now().Format("20060102-150405")+`.md"`)
		fmt.Fprint(w, out.String())
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "未找到接口", http.StatusNotFound)
	})
	mux.Handle("/", http.FileServerFS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	kind, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if kind != "application/json" {
		fail(w, fmt.Errorf("请求需要使用 JSON 格式"))
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		fail(w, err)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func fail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

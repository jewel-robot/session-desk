package main

import (
	"errors"
	"log"
	"log/slog"
	"net/http"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"sessiondesk/internal/prefs"
)

const desktopSupported = true

const (
	defaultWidth  = 1240
	defaultHeight = 840
	minWidth      = 760
	minHeight     = 560
)

func runDesktop(handler http.Handler, preferences *prefs.Store, icon []byte) error {
	var window *application.WebviewWindow
	app := application.New(application.Options{
		Name:        "Session Desk",
		Description: "Windows 与 WSL 会话管理",
		Icon:        icon,
		Logger:      slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: slog.LevelWarn})),
		Assets:      application.AssetOptions{Handler: handler},
		// A second launch brings the existing window forward instead of opening another copy.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.jewel-robot.session-desk",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window == nil {
					return
				}
				if window.IsMinimised() {
					window.UnMinimise()
				}
				window.Show()
				window.Focus()
			},
		},
	})

	saved := preferences.Get().Window
	width, height := defaultWidth, defaultHeight
	if saved.Width >= minWidth && saved.Height >= minHeight && saved.Width <= 10000 && saved.Height <= 10000 {
		width, height = saved.Width, saved.Height
	}
	options := application.WebviewWindowOptions{
		Title: "Session Desk", Frameless: true,
		Width: width, Height: height,
		MinWidth: minWidth, MinHeight: minHeight,
		InitialPosition:  application.WindowCentered,
		URL:              "/",
		BackgroundColour: application.NewRGB(250, 250, 248),
	}
	if saved.Maximised {
		options.StartState = application.WindowStateMaximised
	}
	window = app.Window.NewWithOptions(options)
	rememberWindow(window, preferences, prefs.Window{Width: width, Height: height, Maximised: saved.Maximised})
	return app.Run()
}

// rememberWindow keeps the last normal size and maximised state so the next launch opens the same way.
// Position is intentionally not restored to avoid windows opening on a disconnected monitor.
func rememberWindow(window *application.WebviewWindow, preferences *prefs.Store, state prefs.Window) {
	var mu sync.Mutex
	var timer *time.Timer
	save := func() {
		mu.Lock()
		current := state
		mu.Unlock()
		if err := preferences.Update(func(p *prefs.Preferences) { p.Window = current }); err != nil {
			log.Printf("保存窗口状态失败：%v", err)
		}
	}
	schedule := func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(600*time.Millisecond, save)
	}
	window.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		if window.IsMinimised() {
			return
		}
		maximised := window.IsMaximised()
		width, height := window.Size()
		mu.Lock()
		state.Maximised = maximised
		if !maximised && width >= minWidth && height >= minHeight {
			state.Width, state.Height = width, height
		}
		mu.Unlock()
		schedule()
	})
	window.OnWindowEvent(events.Common.WindowMaximise, func(*application.WindowEvent) {
		mu.Lock()
		state.Maximised = true
		mu.Unlock()
		schedule()
	})
	window.OnWindowEvent(events.Common.WindowUnMaximise, func(*application.WindowEvent) {
		mu.Lock()
		state.Maximised = false
		mu.Unlock()
		schedule()
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		mu.Lock()
		if timer != nil {
			timer.Stop()
		}
		mu.Unlock()
		save()
	})
}

func openURL(link string) error {
	app := application.Get()
	if app == nil {
		return errors.New("应用尚未就绪")
	}
	return app.Browser.OpenURL(link)
}

func quitApp() {
	if app := application.Get(); app != nil {
		app.Quit()
	}
}

var messageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

func showError(title, message string) {
	text, err1 := syscall.UTF16PtrFromString(message)
	caption, err2 := syscall.UTF16PtrFromString(title)
	if err1 != nil || err2 != nil {
		return
	}
	const mbIconError = 0x10
	messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)), mbIconError)
}

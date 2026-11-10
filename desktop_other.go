//go:build !windows

package main

import (
	"errors"
	"net/http"

	"sessiondesk/internal/prefs"
)

const desktopSupported = false

func runDesktop(http.Handler, *prefs.Store, []byte) error {
	return errors.New("当前平台通过 --serve 启动浏览器界面")
}

func openURL(string) error { return errors.New("当前平台请在浏览器中打开链接") }

func showError(string, string) {}

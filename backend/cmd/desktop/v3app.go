package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// This file centralises access to the Wails v3 global application. Wails v3
// exposes runtime capabilities as managers on the Application instead of a
// context-bound runtime package, so DesktopApp resolves the global app where it
// previously carried a context.
func wailsApp() (*application.App, error) {
	app := application.Get()
	if app == nil {
		return nil, errors.New("应用尚未就绪")
	}
	return app, nil
}

func mainWindow() (application.Window, error) {
	app, err := wailsApp()
	if err != nil {
		return nil, err
	}
	window, ok := app.Window.Get(mainWindowName)
	if !ok || window == nil {
		return nil, errors.New("主窗口尚未就绪")
	}
	return window, nil
}

const mainWindowName = "main"

func startupWindowClass() string {
	return fmt.Sprintf("LuckyTVMainWindow-%d", os.Getpid())
}

// desktopAssetRouter keeps the desktop bootstrap/API routes ahead of the
// bundled frontend assets. In Wails v3 the AssetOptions.Handler is the primary
// handler, so we chain our handler with the bundled file server here.
type desktopAssetRouter struct {
	api    http.Handler
	assets http.Handler
}

func (h desktopAssetRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/api" || strings.HasPrefix(path, "/api/") ||
		strings.HasPrefix(path, "/resources/") ||
		path == "/__desktop/runtime-config" {
		h.api.ServeHTTP(w, r)
		return
	}
	h.assets.ServeHTTP(w, r)
}

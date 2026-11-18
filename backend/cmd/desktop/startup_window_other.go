//go:build !windows

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func configureStartupWindow(*application.App) {}

func configureWindowStartup(_ *application.App, window application.Window) {
	window.Show()
}

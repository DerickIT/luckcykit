package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"luckytv/backend/internal/desktopstorage"
	"luckytv/backend/internal/desktopupdate"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	// The updater helper must run before defaultDataDir/runDesktopApp so a
	// replacement never opens the user database or data directory.
	if done, err := desktopupdate.HandleHelperCommand(os.Args); done {
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	if done, err := handleHarnessCommand(os.Args[1:]); done {
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := validateDesktopArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	dataDir, err := defaultDataDir()
	if err != nil {
		log.Fatal(err)
	}
	app := newDesktopApp(dataDir)
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "luckytv:") {
			app.queueSkillLink(arg)
		}
	}
	if err := runDesktopApp(app); err != nil {
		log.Fatal(err)
	}
}

var initialSystemTempDir = os.TempDir()

func systemTempDir() string {
	if strings.TrimSpace(initialSystemTempDir) != "" {
		return initialSystemTempDir
	}
	return os.TempDir()
}

func runDesktopApp(app *DesktopApp) error {
	if err := configureDesktopTemp(app.dataDir); err != nil {
		log.Printf("配置数据目录临时文件夹失败，继续使用系统默认临时目录: %v", err)
	}
	wailsApp := application.New(desktopAppOptions(app))
	// Register the desktop service so its exported methods are callable from the
	// frontend. Asset routing is configured via Assets.Handler below.
	wailsApp.RegisterService(application.NewService(app))
	configureStartupWindow(wailsApp)
	window := wailsApp.Window.NewWithOptions(desktopWindowOptions())
	configureWindowStartup(wailsApp, window)
	wailsApp.OnShutdown(func() { app.shutdown() })
	return wailsApp.Run()
}

func desktopWindowOptions() application.WebviewWindowOptions {
	darkTitle := application.NewRGBPtr(13, 15, 18)
	return application.WebviewWindowOptions{
		Name:             mainWindowName,
		Title:            "LuckyTV",
		Width:            1440,
		Height:           960,
		BackgroundColour: application.NewRGB(13, 15, 18),
		URL:              "/",
		Hidden:           true,
		Windows: application.WindowsWindow{
			Theme: application.Dark,
			CustomTheme: application.ThemeSettings{
				DarkModeActive: &application.WindowTheme{
					TitleBarColour:  darkTitle,
					TitleTextColour: application.NewRGBPtr(244, 244, 245),
					BorderColour:    application.NewRGBPtr(39, 39, 42),
				},
				DarkModeInactive: &application.WindowTheme{
					TitleBarColour:  darkTitle,
					TitleTextColour: application.NewRGBPtr(161, 161, 170),
					BorderColour:    application.NewRGBPtr(39, 39, 42),
				},
			},
		},
	}
}

func desktopAppOptions(app *DesktopApp) application.Options {
	return application.Options{
		Name:        "LuckyTV",
		Description: "LuckyTV 本地工作台",
		Assets: application.AssetOptions{
			Handler: desktopAssetRouter{
				api:    desktopAssetHandler{app: app},
				assets: application.BundledAssetFileServer(assets),
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Windows: application.WindowsOptions{
			WndClass: startupWindowClass(),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: skillInstanceID(app.dataDir),
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				for _, arg := range data.Args {
					if strings.HasPrefix(arg, "luckytv:") {
						app.queueSkillLink(arg)
					}
				}
				app.revealWindow()
			},
		},
	}
}

func prepareDesktopApp(app *DesktopApp) error {
	return app.start(context.Background())
}

func defaultDataDir() (string, error) {
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--data-dir" {
			if i+1 == len(os.Args) {
				return "", fmt.Errorf("--data-dir 需要明确的绝对目录")
			}
			return explicitDesktopDataDir(os.Args[i+1])
		}
		if strings.HasPrefix(arg, "--data-dir=") {
			return explicitDesktopDataDir(strings.TrimPrefix(arg, "--data-dir="))
		}
	}
	if override := strings.TrimSpace(os.Getenv("CANVAS_DESKTOP_DATA_DIR")); override != "" {
		return override, nil
	}
	root, err := desktopstorage.DefaultRoot()
	if err != nil {
		return "", fmt.Errorf("定位用户应用数据目录: %w", err)
	}
	return desktopstorage.Resolve(root)
}

func explicitDesktopDataDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	clean := filepath.Clean(dir)
	if !filepath.IsAbs(dir) || filepath.Dir(clean) == clean {
		return "", fmt.Errorf("--data-dir 需要明确的绝对目录")
	}
	return clean, nil
}

func configureDesktopTemp(dataDir string) error {
	path := filepath.Join(dataDir, "cache", "tmp")
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	for _, name := range []string{"TMPDIR", "TEMP", "TMP"} {
		if err := os.Setenv(name, path); err != nil {
			return err
		}
	}
	return nil
}

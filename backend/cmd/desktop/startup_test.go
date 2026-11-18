package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"luckytv/backend/internal/bootstrap"
	"luckytv/backend/internal/runtimeinfo"
)

func TestSecondLaunchReachesWailsWhilePrimaryOwnsWorkspace(t *testing.T) {
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(key, os.Getenv(key))
	}
	primary := newDesktopApp(t.TempDir())
	if err := primary.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.stop(context.Background()) })
	primaryOptions := desktopAppOptions(primary)
	secondary := newDesktopApp(primary.dataDir)
	link := "luckytv://skills/install?id=official-interview-outline&version=official-interview-outline-1.0.0"
	secondaryOptions := desktopAppOptions(secondary)
	if secondaryOptions.SingleInstance.UniqueID != primaryOptions.SingleInstance.UniqueID {
		t.Fatal("same workspace did not select the primary instance")
	}
	if secondary.runtime() != nil {
		t.Fatal("secondary opened the workspace before single-instance dispatch")
	}
	// Wails forwards these arguments and exits the second process without
	// calling its ServiceStartup. Exercise our actual receiver with the real lock held.
	primaryOptions.SingleInstance.OnSecondInstanceLaunch(application.SecondInstanceData{Args: []string{"--data-dir=" + primary.dataDir, link}})
	if got := primary.ConsumeSkillLink(); got != "/skills?releaseId=official-interview-outline-1.0.0&skillId=official-interview-outline" {
		t.Fatalf("primary did not receive exact-version preview: %q", got)
	}
	if secondary.runtime() != nil || secondary.ConsumeSkillLink() != "" {
		t.Fatal("second launch opened a runtime or consumed the primary's link")
	}
	if primary.runtime() == nil {
		t.Fatal("second launch stopped the primary runtime")
	}
	if _, err := os.Stat(filepath.Join(primary.dataDir, "startup-error.log")); !os.IsNotExist(err) {
		t.Fatalf("forwarding wrote a startup failure: %v", err)
	}
}

func TestSeparateWorkspaceRunsItsOwnStartup(t *testing.T) {
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(key, os.Getenv(key))
	}
	primary := newDesktopApp(t.TempDir())
	if err := primary.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.stop(context.Background()) })
	other := newDesktopApp(t.TempDir())
	t.Cleanup(func() { other.shutdown(context.Background()) })
	otherOptions := desktopAppOptions(other)
	if otherOptions.SingleInstance.UniqueID == desktopAppOptions(primary).SingleInstance.UniqueID {
		t.Fatal("different data directories share the Wails instance lock")
	}
	if other.runtime() != nil {
		t.Fatal("backend opened before the Wails primary-instance decision")
	}
	other.ServiceStartup(context.Background(), application.ServiceOptions{})
	if got := other.StartupState(); got.BaseURL == "" || got.BaseURL == primary.StartupState().BaseURL {
		t.Fatal("isolated startup did not receive its own backend")
	}
}

func TestPrimaryStartupFailureStillRecordsWorkspaceLockError(t *testing.T) {
	ownerDir := t.TempDir()
	app := newDesktopApp(ownerDir)
	app.tempDir = t.TempDir()
	lock, err := runtimeinfo.LockWorkspace(ownerDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	beforeEntries, err := os.ReadDir(ownerDir)
	if err != nil {
		t.Fatal(err)
	}

	startupErr := prepareDesktopApp(app)
	if startupErr == nil || app.runtime() != nil {
		t.Fatal("primary startup bypassed an existing workspace lock")
	}

	afterEntries, err := os.ReadDir(ownerDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEntries) != len(beforeEntries) {
		t.Fatalf("locked workspace mutated owner directory entries: before=%d after=%d", len(beforeEntries), len(afterEntries))
	}
	for i := range beforeEntries {
		if beforeEntries[i].Name() != afterEntries[i].Name() {
			t.Fatalf("locked workspace changed owner directory entry %d: %q -> %q", i, beforeEntries[i].Name(), afterEntries[i].Name())
		}
	}

	data, readErr := os.ReadFile(filepath.Join(app.tempDir, "luckytv-diagnostics", "startup-error.log"))
	if readErr != nil || strings.TrimSpace(string(data)) != startupErr.Error() {
		t.Fatalf("startup failure not recorded in temp diagnostics log: %q, %v", data, readErr)
	}
}

func TestOwnedStartupFailureRecordsErrorLogInDataDir(t *testing.T) {
	app := newDesktopApp(t.TempDir())
	app.tempDir = t.TempDir()
	app.openRuntime = func(_ context.Context, _ string, onPhase func(phase, detail string)) (*bootstrap.Runtime, error) {
		onPhase("migrate", "正在更新本地数据库结构")
		return nil, os.ErrPermission
	}
	err := prepareDesktopApp(app)
	if err == nil {
		t.Fatal("expected prepareDesktopApp failure")
	}
	data, readErr := os.ReadFile(filepath.Join(app.dataDir, "startup-error.log"))
	if readErr != nil || strings.TrimSpace(string(data)) != err.Error() {
		t.Fatalf("owned workspace failure not recorded in dataDir: %q, %v", data, readErr)
	}
}

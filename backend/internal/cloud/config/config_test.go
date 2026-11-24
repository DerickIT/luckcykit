package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromDefaultsToLocalOnlyWithoutSandbox(t *testing.T) {
	cfg, err := LoadFrom(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:8081" || cfg.SandboxEnabled {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestCloudDataDirectoryDoesNotOverlapLocalWorkspace(t *testing.T) {
	for _, test := range []struct {
		name      string
		cloud     string
		workspace string
		wantErr   bool
	}{
		{name: "separate siblings", cloud: "../.local/cloud-service-sandbox", workspace: "../.local/project-workbench-debug"},
		{name: "same directory", cloud: "../.local/project-workbench-debug", workspace: "../.local/project-workbench-debug", wantErr: true},
		{name: "cloud nested in workspace", cloud: "data/cloud", workspace: "data", wantErr: true},
		{name: "workspace nested in cloud", cloud: "data", workspace: "data/workspace", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := LoadFrom(func(key string) string {
				switch key {
				case "LUCKYTV_CLOUD_DATA_DIR":
					return test.cloud
				case "CANVAS_BACKEND_DATA_DIR":
					return test.workspace
				default:
					return ""
				}
			})
			if test.wantErr && err == nil {
				t.Fatal("overlapping cloud and workspace data directories were accepted")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("separate data directories rejected: %v", err)
			}
			_ = cfg
		})
	}
}

func TestCloudDataDirectoryRejectsSymlinkAliases(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "cloud-alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	if !sameOrNestedPath(alias, workspace) {
		t.Fatal("symlink alias to workspace directory was not detected")
	}
	if sameOrNestedPath(filepath.Join(root, "cloud"), workspace) {
		t.Fatal("separate sibling path was treated as overlapping")
	}
}

func TestCanonicalPathFailsClosedForDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "missing-target"), dangling); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := canonicalPath(dangling); err == nil {
		t.Fatal("dangling symlink path unexpectedly canonicalized")
	}
}

func TestSandboxRequiresLoopbackAndNonProduction(t *testing.T) {
	for _, test := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{name: "wildcard listener", vars: map[string]string{"LUCKYTV_CLOUD_SANDBOX": "true", "LUCKYTV_CLOUD_ADDR": ":8081"}, want: "loopback"},
		{name: "hostname listener", vars: map[string]string{"LUCKYTV_CLOUD_SANDBOX": "true", "LUCKYTV_CLOUD_ADDR": "localhost:8081"}, want: "loopback"},
        {name: "production missing database", vars: map[string]string{"LUCKYTV_CLOUD_ENV": "production"}, want: "LUCKYTV_CLOUD_DATABASE_URL is required"},
		{name: "shared workspace data directory", vars: map[string]string{"LUCKYTV_CLOUD_DATA_DIR": "../.local/project-workbench-debug", "CANVAS_BACKEND_DATA_DIR": "../.local/project-workbench-debug"}, want: "must not overlap"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadFrom(func(key string) string { return test.vars[key] })
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

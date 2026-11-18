package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"luckytv/backend/internal/protocol"
)

func TestPackageModuleProviderResolvesInsideInstalledPackage(t *testing.T) {
	rt, err := NewRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(rt, nil)
	svc.SetPackageFetcher(copyPackageFetcher)
	src := t.TempDir()
	pkgJSON := `{"name":"luckytv-plugin-module-test","version":"1.0.0","description":"module provider","luckytv":{"contributes":{"providers":[{"id":"module-test","label":"Module Test","module":"./provider.mjs","capabilities":["image"],"scopes":["canvas"]}]}}}`
	for name, content := range map[string]string{"package.json": pkgJSON, "provider.mjs": "export async function submit() {}\n"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.InstallPackage(src, ""); err != nil {
		t.Fatal(err)
	}
	adapter, ok := rt.Registry().Resolve("module-test")
	if !ok {
		t.Fatal("module provider is not registered")
	}
	module, ok := adapter.(protocol.ModuleProvider)
	if !ok || module.Plugin != "luckytv-plugin-module-test" || !filepath.IsAbs(module.Module) {
		t.Fatalf("adapter = %#v", adapter)
	}
	if _, err := os.Stat(module.Module); err != nil {
		t.Fatalf("module path is not inside the installed package: %v", err)
	}
}

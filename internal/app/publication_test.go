package app_test

import (
	"os"
	"path/filepath"
	"testing"
)

func nativeContent(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	content := t.TempDir()
	for _, dir := range []string{"pipelines", "normalization", "collector/configs", "runtime/sqlmesh", "internal/app/testdata/synthetic"} {
		copyTree(t, filepath.Join(root, dir), filepath.Join(content, dir))
	}
	local := os.Getenv("BATZ_TEST_RUNTIME")
	lockPath := filepath.Join(root, "runtime/dependencies.lock.json")
	if local != "" {
		lockPath = filepath.Join(local, "dependencies.lock.json")
	} else {
		local = os.Getenv("BATZ_TOOLCHAIN_ROOT")
		if local == "" {
			local = filepath.Join(root, "runtime/.local")
		}
	}
	lock, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(content, "runtime/dependencies.lock.json"), lock, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(local, filepath.Join(content, "runtime/.local")); err != nil {
		t.Fatal(err)
	}
	return content
}

package app_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func nativeContent(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	content := t.TempDir()
	for _, dir := range []string{"pipelines", "models", "tests", "config.yaml", "runtime/sqlmesh", "internal/app/testdata/synthetic"} {
		copyTree(t, filepath.Join(root, dir), filepath.Join(content, dir))
	}
	local := os.Getenv("BATZ_TEST_RUNTIME")
	lockPath := filepath.Join(root, "runtime/dependencies.lock.json")
	if local != "" {
		lockPath = filepath.Join(local, "dependencies.lock.json")
	}
	lock, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if local == "" {
		// Fast contracts stop at preflight and need no installed engine. These
		// inert, non-executable files satisfy integrity admission only. Native
		// lanes get their real collector and receipt from ci/test instead.
		local = t.TempDir()
		data := []byte("inert preflight fixture; native execution is forbidden\n")
		sum := sha256.Sum256(data)
		artifacts := make(map[string]any)
		for name, path := range map[string]string{"collector": "batz-collector", "python": "venv/bin/python"} {
			full := filepath.Join(local, path)
			if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, data, 0600); err != nil {
				t.Fatal(err)
			}
			artifacts[name] = map[string]string{"path": "runtime/.local/" + path, "sha256": hex.EncodeToString(sum[:])}
		}
		var receipt map[string]any
		if err := json.Unmarshal(lock, &receipt); err != nil {
			t.Fatal(err)
		}
		receipt["installed_binaries"] = map[string]any{runtime.GOOS + "/" + runtime.GOARCH: artifacts}
		lock, err = json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(content, "runtime/dependencies.lock.json"), lock, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(local, filepath.Join(content, "runtime/.local")); err != nil {
		t.Fatal(err)
	}
	return content
}

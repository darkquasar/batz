package app_test

import (
	"batz/internal/app"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstalledContentRefusedBeforeAcquisition(t *testing.T) {
	for _, changed := range []string{"runtime/.local/batz-collector", "models/probe.sql", "macros/added.sql"} {
		t.Run(changed, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{}
			for _, name := range []string{"bin/batz", "runtime/.local/batz-collector", "runtime/dependencies.lock.json", "config.yaml", "runtime/sqlmesh/runner.py", "runtime/sqlmesh/catalog.py", "runtime/sqlmesh/inspect_preview.py", "models/probe.sql"} {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256([]byte("original"))
				files[name] = hex.EncodeToString(sum[:])
			}
			inventory, err := json.Marshal(map[string]any{"schema": "batz-release-content/v0alpha1", "version": "development", "source_sha": "unknown", "target": runtime.GOOS + "/" + runtime.GOARCH, "files": files})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "release-content.json"), inventory, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, changed)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, changed), []byte("foreign release"), 0600); err != nil {
				t.Fatal(err)
			}
			workspace := filepath.Join(t.TempDir(), "must not exist")
			var out, diagnostics bytes.Buffer
			code := app.Execute([]string{"run", "missing.yaml", "--project", root, "--workspace", workspace}, &out, &diagnostics)
			if code != 4 || !strings.Contains(diagnostics.String(), "release-content") {
				t.Fatalf("content not admitted before acquisition: %d %s", code, &diagnostics)
			}
			if _, err := os.Stat(workspace); !os.IsNotExist(err) {
				t.Fatalf("preflight mutated workspace: %v", err)
			}
		})
	}
}

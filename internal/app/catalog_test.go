package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootCatalogAdmissionIgnoresImplementationPython(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{"config.yaml": "{}", "models/a/events.sql": "SELECT 1", "runtime/sqlmesh/runner.py": "raise RuntimeError('not native content')"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateProjectContent(root); err != nil {
		t.Fatalf("root catalog rejects legitimate implementation: %v", err)
	}
	for _, name := range []string{"config.py", "models/a/evil.py", "macros/evil.py", "linter/evil.py", "external_models/other.yaml", "schema.yaml"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("raise RuntimeError('must not execute')"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := validateProjectContent(root); err == nil {
			t.Fatalf("admitted %s", name)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

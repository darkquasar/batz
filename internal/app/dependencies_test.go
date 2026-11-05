package app_test

import (
	"batz/internal/app"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyTree(t *testing.T, source, dest string) {
	t.Helper()
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(dest, data, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	err = filepath.WalkDir(source, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if e.IsDir() {
			return os.MkdirAll(filepath.Join(dest, rel), 0700)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, rel), b, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTamperedCollectorRefusedBeforeWorkspaceMutation(t *testing.T) {
	root := nativeContent(t)
	content := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	for _, dir := range []string{"internal/app/testdata/synthetic", "models/synthetic-jsonl", "tests/synthetic-jsonl", "config.yaml", "runtime/sqlmesh"} {
		copyTree(t, filepath.Join(root, dir), filepath.Join(content, dir))
	}
	if err := os.MkdirAll(filepath.Join(content, "runtime/.local/venv/bin"), 0700); err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile(filepath.Join(root, "runtime/dependencies.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err = json.Unmarshal(lock, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["installed_binaries"] == nil {
		t.Fatal("test setup requires an installation receipt")
	}
	if err = os.WriteFile(filepath.Join(content, "runtime/dependencies.lock.json"), lock, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(content, "runtime/.local/batz-collector"), []byte("#!/bin/sh\nexit 97\n"), 0700); err != nil {
		t.Fatal(err)
	}
	tools := os.Getenv("BATZ_TOOLCHAIN_ROOT")
	if tools == "" {
		tools = filepath.Join(root, "runtime/.local")
	}
	if err = os.Symlink(filepath.Join(tools, "venv/bin/python"), filepath.Join(content, "runtime/.local/venv/bin/python")); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(content, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", content, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl")}, &out, &diagnostics)
	if code != 4 || !strings.Contains(diagnostics.String(), "collector") {
		t.Fatalf("tampered collector must fail integrity preflight; exit=%d stderr=%s", code, &diagnostics)
	}
	if _, err = os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("integrity failure mutated workspace: %v", err)
	}
}

func copyNativeCatalog(t *testing.T, source, dest string) {
	t.Helper()
	for _, path := range []string{"config.yaml", "models", "tests"} {
		copyTree(t, filepath.Join(source, path), filepath.Join(dest, path))
	}
}

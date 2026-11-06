package app

import (
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

func TestObservedReceiptCannotAuthorizeForeignPython(t *testing.T) {
	root := t.TempDir()
	path := "runtime/.local/venv/bin/python"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("foreign interpreter")
	if err := os.WriteFile(filepath.Join(root, path), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(foreign)
	observed := hex.EncodeToString(sum[:])
	target := runtime.GOOS + "/" + runtime.GOARCH
	lock, err := json.Marshal(map[string]any{"version": "batz-runtime-lock/v0alpha1", "uv": map[string]string{"version": "test-fixture"}, "python": map[string]any{"version": "3.12.12", "binary_sha256": map[string]string{target: strings.Repeat("0", 64)}}, "installed_binaries": map[string]any{target: map[string]any{"python": map[string]string{"path": path, "sha256": observed}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime/dependencies.lock.json"), lock, 0600); err != nil {
		t.Fatal(err)
	}
	sum = sha256.Sum256(lock)
	receipt, err := json.Marshal(map[string]any{"schema": "batz-installation/v0alpha1", "target": target, "runtime_lock_sha256": hex.EncodeToString(sum[:]), "binaries": map[string]any{"python": map[string]string{"path": path, "sha256": observed}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime/.local/installation.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "runtime/.local/python/bin/python3.12")
	if err := os.MkdirAll(filepath.Dir(base), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(root, path)); err != nil {
		t.Fatal(err)
	}
	config := "home = " + filepath.Dir(base) + "\ninclude-system-site-packages = false\nuv = test-fixture\n"
	if err := os.WriteFile(filepath.Join(root, "runtime/.local/venv/pyvenv.cfg"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRuntimeArtifacts(root, "python"); err == nil {
		t.Fatal("forged observation overrode immutable interpreter authority")
	}
	// A setup observation succeeds only after the immutable declaration agrees.
	lock = bytes.ReplaceAll(lock, []byte(strings.Repeat("0", 64)), []byte(observed))
	if err := os.WriteFile(filepath.Join(root, "runtime/dependencies.lock.json"), lock, 0600); err != nil {
		t.Fatal(err)
	}
	sum = sha256.Sum256(lock)
	var installation map[string]any
	if err := json.Unmarshal(receipt, &installation); err != nil {
		t.Fatal(err)
	}
	installation["runtime_lock_sha256"] = hex.EncodeToString(sum[:])
	receipt, err = json.Marshal(installation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime/.local/installation.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRuntimeArtifacts(root, "python"); err != nil {
		t.Fatalf("matching immutable Python refused: %v", err)
	}
	if err := verifyRuntimeArtifacts(root, "collector"); err == nil {
		t.Fatal("missing collector unexpectedly admitted")
	}
	venv := filepath.Join(root, "runtime/.local/venv")
	foreignEnv := filepath.Join(t.TempDir(), "same-byte-foreign-env")
	if err := os.Rename(venv, foreignEnv); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignEnv, venv); err != nil {
		t.Fatal(err)
	}
	if err := verifyRuntimeArtifacts(root, "python"); err == nil {
		t.Fatal("same-byte foreign environment accepted")
	}
	if err := os.Remove(venv); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(foreignEnv, venv); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(venv, "pyvenv.cfg"), []byte(strings.ReplaceAll(config, "= false", "= true")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRuntimeArtifacts(root, "python"); err == nil {
		t.Fatal("environment with system site packages accepted")
	}
}

func TestSourceCollectorReceiptMustMatchCurrentSource(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{".git": "checkout", "go.mod": "module fixture", "go.sum": "sums", "collector/input.go": "package collector", "cmd/batz-collector/main.go": "package main", "internal/buildinfo/buildinfo.go": "package buildinfo", "runtime/.local/batz-collector": "built collector"}
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := sha256.New()
	for _, name := range []string{"cmd/batz-collector/main.go", "collector/input.go", "go.mod", "go.sum", "internal/buildinfo/buildinfo.go"} {
		source.Write([]byte(name + "\x00" + files[name] + "\x00"))
	}
	collector := sha256.Sum256([]byte(files["runtime/.local/batz-collector"]))
	target := runtime.GOOS + "/" + runtime.GOARCH
	lock := []byte(`{"version":"batz-runtime-lock/v0alpha1"}`)
	if err := os.WriteFile(filepath.Join(root, "runtime/dependencies.lock.json"), lock, 0600); err != nil {
		t.Fatal(err)
	}
	lockSum := sha256.Sum256(lock)
	receipt, err := json.Marshal(map[string]any{"schema": "batz-installation/v0alpha1", "target": target, "runtime_lock_sha256": hex.EncodeToString(lockSum[:]), "collector_source_sha256": hex.EncodeToString(source.Sum(nil)), "binaries": map[string]any{"collector": map[string]string{"path": "runtime/.local/batz-collector", "sha256": hex.EncodeToString(collector[:])}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime/.local/installation.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRuntimeArtifacts(root, "collector"); err != nil {
		t.Fatalf("matching source collector refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "collector/input.go"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRuntimeArtifacts(root, "collector"); err == nil {
		t.Fatal("stale source collector accepted")
	}
}

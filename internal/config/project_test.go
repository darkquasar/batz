package config_test

import (
	"batz/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledProjectFollowsExecutableSymlinkAndExplicitPrecedence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installation with spaces")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin/batz")
	if err := os.WriteFile(executable, []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "batz")
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	caller := t.TempDir()
	got, err := config.ProjectRoot("", caller, link)
	if err != nil || got != root {
		t.Fatalf("installed root=%q err=%v, want %s", got, err, root)
	}
	explicit := t.TempDir()
	if got, err := config.ProjectRoot(explicit, caller, link); err != nil || got != explicit {
		t.Fatalf("explicit project lost: %q %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(caller, ".git"), []byte("unrelated checkout"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := config.ProjectRoot("", caller, link); err != nil || got != caller {
		t.Fatalf("unrelated checkout switched to installed content: %q %v", got, err)
	}
}

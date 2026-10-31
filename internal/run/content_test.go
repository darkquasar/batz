package run

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProductionContentExcludesTestOnlyChanges(t *testing.T) {
	project := t.TempDir()
	for path, content := range map[string]string{
		"config.yaml": "{}", "models/events.sql": "SELECT 1",
		"audits/check.sql": "SELECT 0", "macros/shared.sql": "SELECT 2",
		"seeds/labels.csv": "label\none\n", "tests/test_events.yaml": "reviewed: true", "examples.yaml": "example: true",
	} {
		full := filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := copyProject(project, filepath.Join(t.TempDir(), "project"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tests/test_events.yaml"), []byte("reviewed: changed"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := copyProject(project, filepath.Join(t.TempDir(), "project"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("test-only edit changed production content identity")
	}
	for _, path := range []string{"models/events.sql", "audits/check.sql", "macros/shared.sql", "seeds/labels.csv"} {
		if err := os.WriteFile(filepath.Join(project, path), []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
		changed, err := copyProject(project, filepath.Join(t.TempDir(), "project"))
		if err != nil {
			t.Fatal(err)
		}
		if changed[path] == second[path] {
			t.Fatalf("executed content edit lost: %s", path)
		}
	}
}

package app_test

import (
	"batz/internal/app"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitExampleCannotUseIncompatibleRoute(t *testing.T) {
	root, _ := filepath.Abs("../..")
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail/pipeline.yaml"), "--project", root, "--level", "3", "--example", "unregistered.json", "--format", "json"}, &out, &diagnostics)
	if code != 2 {
		t.Fatalf("unassociated file accepted: %d %s", code, &out)
	}
}

func TestPreviewMissingRuntimeNeverDownloadsOrCreatesState(t *testing.T) {
	root := t.TempDir()
	definition := filepath.Join(root, "source.yaml")
	data, err := os.ReadFile("../../pipelines/aws-cloudtrail/pipeline.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(definition, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, strict := range []bool{false, true} {
		args := []string{"explain", definition, "--project", root, "--level", "3", "--format", "json"}
		if strict {
			args = append(args, "--strict")
		}
		var out, diagnostics bytes.Buffer
		code := app.Execute(args, &out, &diagnostics)
		expected := 0
		if strict {
			expected = 3
		}
		if code != expected {
			t.Fatalf("missing runtime: got %d want %d %s %s", code, expected, &out, &diagnostics)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected state or downloads: %v", entries)
	}
}

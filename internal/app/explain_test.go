package app_test

import (
	"batz/internal/app"
	"batz/internal/buildinfo"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBareExplainUsesDeclarationsWithoutWorkspaceOrRuntime(t *testing.T) {
	original := buildinfo.SourceSHA
	buildinfo.SourceSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	t.Cleanup(func() { buildinfo.SourceSHA = original })
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pipelines"), 0700); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../internal/app/testdata/synthetic/pipeline.yaml")
	if err != nil {
		t.Fatal(err)
	}
	definition := filepath.Join(root, "pipelines", "synthetic-jsonl.yaml")
	if err = os.WriteFile(definition, source, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", definition, "--project", root, "--format", "json"}, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("bare explain must show known declarations without installed assets or workspace: exit=%d %s", code, &diagnostics)
	}
	var report contracts.ExplainReport
	if err = json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Level != 1 || len(report.Routes) != 1 || report.Routes[0].Name != "local_raw" || report.Routes[0].Models["findings"] != "probe.findings" {
		t.Fatalf("declarations missing: %+v", report)
	}
	if len(report.Diagnostics) == 0 {
		t.Fatal("missing native assets must be disclosed")
	}
	for _, check := range []struct {
		args []string
		code int
	}{
		{[]string{"--level", "2"}, 0},
		{[]string{"--strict"}, 3},
		{[]string{"--level", "3"}, 4},
	} {
		out.Reset()
		diagnostics.Reset()
		args := append([]string{"explain", definition, "--project", root, "--format", "json"}, check.args...)
		if code := app.Execute(args, &out, &diagnostics); code != check.code {
			t.Fatalf("explain %v: exit=%d want=%d %s", check.args, code, check.code, &diagnostics)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "pipelines" {
		t.Fatalf("explain mutated content root: %v", entries)
	}
}

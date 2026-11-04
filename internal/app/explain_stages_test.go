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

func TestExplainStagesWithoutExecutingRuntime(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail/pipeline.yaml"), "--project", root, "--level", "2", "--format", "json", "--strict"}, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("stage explanation unavailable: exit=%d %s %s", code, &out, &diagnostics)
	}
	var report struct {
		Level  int `json:"level"`
		Stages []struct {
			ID string `json:"id"`
		} `json:"stages"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Level != 2 || len(report.Stages) < 5 {
		t.Fatalf("missing cumulative stages: %s", &out)
	}
}

func assertOfflineSampleBoundaries(t *testing.T, raw []byte) {
	t.Helper()
	var report struct {
		Samples []struct {
			Stage   string           `json:"stage"`
			Basis   string           `json:"basis"`
			Rows    int              `json:"rows"`
			Records []map[string]any `json:"records"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Samples) != 5 {
		t.Fatalf("expected five observed boundaries: %s", raw)
	}
	for _, s := range report.Samples {
		if s.Basis != "sample-executed" || s.Rows != 1 || len(s.Records) != 1 {
			t.Fatalf("invalid sample: %+v", s)
		}
	}
	last := report.Samples[4].Records[0]
	event, ok := last["event"].(map[string]any)
	if !ok || event["action"] != "object-read" {
		t.Fatalf("wrong normalized action: %+v", last)
	}
}

func TestSavedBindingEnrichesExplainWithOrigin(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binding := filepath.Join(t.TempDir(), "binding.yaml")
	content := "apiVersion: batz/v0alpha1\nkind: PipelineBinding\nid: chosen\ndefinition: repo://pipelines/aws-cloudtrail/pipeline.yaml\nversion: 1\nroute: local_raw\ninput:\n  path: repo://pipelines/aws-cloudtrail/fixtures/api-read.json\n"
	if err := os.WriteFile(binding, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail/pipeline.yaml"), "--project", root, "--binding", binding, "--format", "json"}, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("saved context failed: %d %s %s", code, &out, &diagnostics)
	}
	var report struct {
		Resolution map[string]struct {
			Value  any    `json:"value"`
			Source string `json:"source"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	origin, ok := report.Resolution["input.path"]
	if !ok || origin.Value != filepath.Join(root, "pipelines/aws-cloudtrail/fixtures/api-read.json") || origin.Source != binding {
		t.Fatalf("lost value/origin: %s", &out)
	}
}

func TestRunAggregatesMissingWorkspaceAndRequiredInputs(t *testing.T) {
	root, _ := filepath.Abs("../..")
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail/pipeline.yaml"), "--project", root, "--route", "local_raw"}, &out, &diagnostics)
	if code != 2 || !strings.Contains(diagnostics.String(), "missing-workspace") || !strings.Contains(diagnostics.String(), "input.path") {
		t.Fatalf("incomplete static diagnostics: %d %s", code, &diagnostics)
	}
}

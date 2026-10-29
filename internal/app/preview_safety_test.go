package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"batz/internal/app"
	"batz/internal/contracts"
)

func TestRepeatedPreviewHasStableSemanticSections(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var previous map[string]any
	for i := 0; i < 2; i++ {
		var out, diagnostics bytes.Buffer
		code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--example", "api-read", "--format", "json", "--strict"}, &out, &diagnostics)
		if code != 0 {
			t.Fatalf("preview %d: %d %s %s", i, code, &out, &diagnostics)
		}
		assertOfflineSampleBoundaries(t, out.Bytes())
		var report map[string]any
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		delete(report, "runtime")
		if previous != nil && !reflect.DeepEqual(previous, report) {
			t.Fatalf("unstable semantic report:\n%v\n%v", previous, report)
		}
		previous = report
	}
}

func TestExplicitExampleCannotUseIncompatibleRoute(t *testing.T) {
	root, _ := filepath.Abs("../..")
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--example", "unregistered.json", "--format", "json"}, &out, &diagnostics)
	if code != 2 {
		t.Fatalf("unassociated file accepted: %d %s", code, &out)
	}
}

func TestPreviewMissingRuntimeNeverDownloadsOrCreatesState(t *testing.T) {
	root := t.TempDir()
	definition := filepath.Join(root, "source.yaml")
	data, err := os.ReadFile("../../pipelines/aws-cloudtrail.yaml")
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

func TestExplicitLocalExampleWithRouteIsAccepted(t *testing.T) {
	root, _ := filepath.Abs("../..")
	sample := filepath.Join(t.TempDir(), "custom.json")
	source, err := os.ReadFile(filepath.Join(root, "fixtures/cloudtrail/api-read.json"))
	if err != nil {
		t.Fatal(err)
	}
	source = bytes.Replace(source, []byte("2026-10-25T12:00:00Z"), []byte("2026-10-25T12:00:00.000000001Z"), 1)
	if err = os.WriteFile(sample, source, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--example", sample, "--level", "3", "--format", "json", "--strict"}, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("explicit association rejected: %d %s %s", code, &out, &diagnostics)
	}
	var report contracts.ExplainReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	unobserved, ok := report.Runtime["unobserved"].([]any)
	if !ok {
		t.Fatalf("missing example coverage: %s", &out)
	}
	for _, branch := range unobserved {
		if branch == "fractional-time" {
			t.Fatalf("executed fractional-time branch reported unobserved: %s", &out)
		}
	}
	for _, sample := range report.Samples {
		if !strings.Contains(sample.Reason, "Observed preview branches: fractional-time;") {
			t.Fatalf("source-derived coverage missing from stable sample metadata: %+v", sample)
		}
	}
}

func TestWrongReviewedSampleValueFailsIntegrity(t *testing.T) {
	root := nativeContent(t)
	if err := os.MkdirAll(filepath.Join(root, "fixtures/cloudtrail"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"api-read.json", "api-read.expected.json"} {
		data, err := os.ReadFile(filepath.Join("../../fixtures/cloudtrail", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "api-read.expected.json" {
			data = bytes.ReplaceAll(data, []byte("object-read"), []byte("invented-wrong-action"))
		}
		if err = os.WriteFile(filepath.Join(root, "fixtures/cloudtrail", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// nativeContent retains the trusted runner; add the Batz-owned inspection bridge.
	inspector, err := os.ReadFile("../../runtime/sqlmesh/inspect_preview.py")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "runtime/sqlmesh/inspect_preview.py"), inspector, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--format", "json", "--strict"}, &out, &diagnostics)
	if code != 4 {
		t.Fatalf("wrong reviewed action accepted: %d %s %s", code, &out, &diagnostics)
	}
}

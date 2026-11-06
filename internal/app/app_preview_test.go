//go:build batz_app

package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	pipeline "batz/internal/run"
	"batz/internal/scratch"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAppNativePreviewComposesWorkspaceSelectionStructuresMacrosAndRepeatability(t *testing.T) {
	root := nativeContent(t)
	definition := workspaceAssetDefinition(t, root)
	workspace := t.TempDir()
	selected := filepath.Join(workspace, "alternate")
	copyNativeCatalog(t, root, selected)
	// Contributor metadata never participates in the executed preview project.
	if err := os.WriteFile(filepath.Join(selected, "tests/README.md"), []byte("{{ literal contributor notes }}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, "tests/test_unused.yaml"), []byte("invalid test-only YAML: [{{"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(root, "internal/app/testdata/synthetic/collector.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "collector.yaml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(selected, "models/synthetic-jsonl/findings.sql")
	data, err := os.ReadFile(model)
	if err != nil {
		t.Fatal(err)
	}
	native := strings.Replace(string(data), "kind FULL);\n", "kind FULL);\n\n@DEF(mapped_status, 'mapped');\n", 1)
	native = strings.Replace(native, "s.mapping_status='mapped'", "s.mapping_status=@mapped_status", 1)
	native = strings.Replace(native, "SELECT\n", "SELECT\n 9007199254740993::BIGINT AS example_large_integer,\n TIMESTAMP_NS '2026-10-25 12:00:00.000000001' AS example_nanosecond_time,\n", 1)
	if err := os.WriteFile(model, []byte(native), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(selected, "seeds"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, "seeds/labels.csv"), []byte("label\n\"{{ 40 + 2 }}\"\n\"{% raw %}\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	binding := filepath.Join(root, "binding.yaml")
	if err := os.WriteFile(binding, []byte("apiVersion: batz/v0alpha1\nkind: PipelineBinding\nid: demo\ndefinition: repo://internal/app/testdata/synthetic/pipeline.yaml\nversion: 1\nroute: local_raw\nnormalization:\n  project: workspace://alternate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var last contracts.ExplainReport
	var previous map[string]any
	for i := 0; i < 2; i++ {
		var out, diagnostics bytes.Buffer
		code := app.Execute([]string{"explain", definition, "--project", root, "--workspace", workspace, "--binding", binding, "--level", "3", "--example", "events", "--format", "json", "--strict"}, &out, &diagnostics)
		if code != 0 {
			t.Fatalf("native preview %d: %d %s %s", i, code, &out, &diagnostics)
		}
		assertOfflineSampleBoundaries(t, out.Bytes())
		assertLosslessStructures(t, out.Bytes())
		var report contracts.ExplainReport
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.Project != selected || report.Assets[filepath.Join(selected, "config.yaml")] == "" || report.Assets["workspace://collector.yaml"] == "" {
			t.Fatalf("workspace/context selection ignored: %s", &out)
		}
		last = report
		var stable map[string]any
		if err := json.Unmarshal(out.Bytes(), &stable); err != nil {
			t.Fatal(err)
		}
		delete(stable, "runtime")
		if previous != nil && !reflect.DeepEqual(previous, stable) {
			t.Fatalf("unstable semantic preview: %v %v", previous, stable)
		}
		previous = stable
	}
	assertInspectionFailures(t, root, last)
	if _, err := os.Stat(filepath.Join(workspace, ".batz")); !os.IsNotExist(err) {
		t.Fatalf("preview mutated production workspace: %v", err)
	}
}

// Reuse real preview outputs without another collector or SQLMesh Context.

func assertInspectionFailures(t *testing.T, root string, report contracts.ExplainReport) {
	t.Helper()
	dir, ok := report.Runtime["scratch"].(string)
	if !ok {
		t.Fatal("missing native preview scratch")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "inspect.json"))
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	expected, ok := request["expectations"].(string)
	if !ok {
		t.Fatal("missing reviewed expectation path")
	}
	contents, err := os.ReadFile(expected)
	if err != nil {
		t.Fatal(err)
	}
	wrong := filepath.Join(dir, "wrong-expected.json")
	if err := os.WriteFile(wrong, bytes.ReplaceAll(contents, []byte("observed"), []byte("wrong-action")), 0600); err != nil {
		t.Fatal(err)
	}
	request["expectations"] = wrong
	request["result"] = filepath.Join(dir, "negative-result.json")
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "negative-inspect.json")
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := scratch.Profile(dir, []string{filepath.Join(root, "runtime/.local/python"), filepath.Join(root, "runtime/.local/venv"), filepath.Join(root, "runtime/sqlmesh")})
	if err != nil {
		t.Fatal(err)
	}
	operational := filepath.Join(dir, "operational.py")
	if err := os.WriteFile(operational, []byte("raise OSError('invented unavailable output')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, script string
		exit         int
	}{
		{name: "wrong-reviewed-value", script: filepath.Join(root, "runtime/sqlmesh/inspect_preview.py"), exit: 4},
		{name: "operational", script: operational, exit: 1},
	} {
		for _, name := range []string{"inspection.lease", "inspection.log"} {
			if err := os.Rename(filepath.Join(dir, name), filepath.Join(dir, tc.name+"-previous-"+name)); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := pipeline.InspectPrivate(ctx, dir, dir, profile, filepath.Join(root, "runtime/.local/venv/bin/python"), tc.script, path)
		cancel()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != tc.exit {
			t.Fatalf("inspection %s classification: %v", tc.name, err)
		}
	}
}

func TestAppExplicitLocalExampleWithRouteIsAccepted(t *testing.T) {
	root := nativeContent(t)
	sample := filepath.Join(t.TempDir(), "custom.json")
	source, err := os.ReadFile(filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	source = bytes.Replace(source, []byte("2026-10-25T12:00:00.000000001"), []byte("2026-10-25T12:00:00.000000002"), 1)
	if err = os.WriteFile(sample, source, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--example", sample, "--level", "3", "--format", "json", "--strict"}, &out, &diagnostics)
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
	if len(report.Samples) == 0 {
		t.Fatal("explicit example produced no observed samples")
	}
	for _, sample := range report.Samples {
		if !strings.Contains(sample.Reason, "Observed preview branches: fractional-time;") {
			t.Fatalf("source-derived coverage missing from stable sample metadata: %+v", sample)
		}
	}
}

func TestAppTinyMixedNativeWiringRetainsStrictPartial(t *testing.T) {
	root := nativeContent(t)
	var err error
	input := filepath.Join(t.TempDir(), "mixed.json")
	valid, bad, unsupported := map[string]any{"id": "valid"}, map[string]any{"id": "bad"}, map[string]any{"id": "unsupported"}
	bad["disposition"] = "rejected"
	unsupported["disposition"] = "unsupported"
	writeEvents(t, input, []map[string]any{valid, bad, unsupported, nil})
	nativePath := filepath.Join(root, "internal/app/testdata/synthetic/collector.yaml")
	native, err := os.ReadFile(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	condition := `"${BATZ_STAGE_DIR:}" == "" || "${BATZ_SOURCE_PATH:}" == "" || "${BATZ_RUN_ID:}" == "" || "${BATZ_BINDING_ID:}" == "" || "${BATZ_BATCH_ID:}" == "" || "${BATZ_ATTEMPT_ID:}" == ""`
	changed := strings.Replace(string(native), `root.payload = content().string()`, `root.payload = if `+condition+` { throw("missing-handoff") } else { content().string() }`, 1)
	if changed == string(native) {
		t.Fatal("handoff assertion was not installed")
	}
	if err := os.WriteFile(nativePath, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	var out, stderr bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input, "--strict"}, &out, &stderr)
	var report contracts.RunReport
	if err = json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 6 || report.Outcome != "published" || report.Quality == nil || *report.Quality != "partial" || report.NormalizedRevision == nil || report.Coverage.Mapped != 1 || report.Coverage.Unsupported != 1 || report.Counts[0].Accepted != 3 || report.Counts[0].Rejected != 1 || report.Counts[1].Rejected != 1 {
		t.Fatalf("strict partial not retained: %d %s %s", code, &out, &stderr)
	}
	if got := string(query(t, root, *report.Catalog, "SELECT count(*) FROM demo.findings")); got != "[[1]]" {
		t.Fatal(got)
	}
	saved, err := os.ReadFile(filepath.Join(workspace, ".batz/reports", string(report.Identity.Run)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted contracts.RunReport
	if err = json.Unmarshal(saved, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Exit != 6 || persisted.NormalizedRevision == nil || *persisted.NormalizedRevision != *report.NormalizedRevision {
		t.Fatal("durable run lost strict partial revision")
	}
	contextPath := filepath.Join(workspace, ".batz/parsed", string(*report.ParsedRevision), "context.json")
	info, err := os.Stat(contextPath)
	if err != nil || info.Mode().Perm()&0222 != 0 {
		t.Fatalf("context was not read-only: %v %v", info, err)
	}
	var defaultOut, defaultErrors bytes.Buffer
	code = app.Execute([]string{"retry", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &defaultOut, &defaultErrors)
	var relaxed contracts.RunReport
	if err := json.Unmarshal(defaultOut.Bytes(), &relaxed); err != nil {
		t.Fatal(err)
	}
	if code != 0 || relaxed.Quality == nil || *relaxed.Quality != "partial" || relaxed.NormalizedRevision == nil || *relaxed.NormalizedRevision != *report.NormalizedRevision || relaxed.Coverage != report.Coverage {
		t.Fatalf("default strictness changed publication facts: %d %s %s", code, &defaultOut, &defaultErrors)
	}
	if _, modeled := relaxed.Resources["model"]; modeled {
		t.Fatal("strictness-only retry reran models")
	}

}

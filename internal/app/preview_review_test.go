package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"batz/internal/app"
	"batz/internal/contracts"
)

func previewContent(t *testing.T) string {
	t.Helper()
	root := nativeContent(t)
	if err := os.MkdirAll(filepath.Join(root, "fixtures/cloudtrail"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"api-read.json", "api-read.expected.json"} {
		data, err := os.ReadFile(filepath.Join("../../fixtures/cloudtrail", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "fixtures/cloudtrail", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestPreviewRejectsJinjaTemplatingBeforeLaunch(t *testing.T) {
	const model = "MODEL (name cloudtrail.api_activity, kind FULL);\n"
	tests := []struct {
		name    string
		file    string
		content string
		strict  bool
	}{
		{"query block", "models/api_activity.sql", model + `JINJA_QUERY_BEGIN;
SELECT 40 + 2 AS invented_probe;
JINJA_END;
`, true},
		{"lowercase statement block", "models/api_activity.sql", model + `jinja_statement_begin;
SELECT 40 + 2 AS invented_probe;
Jinja_End;
`, false},
		{"expression delimiter", "models/api_activity.sql", model + `SELECT {{ 40 + 2 }} AS invented_probe;
`, true},
		{"statement delimiter", "models/api_activity.sql", model + `{% set invented_probe = 42 %}
SELECT 42 AS answer;
`, false},
		{"comment delimiter", "models/api_activity.sql", model + `{# invented_probe #}
SELECT 42 AS answer;
`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := previewContent(t)
			if err := os.WriteFile(filepath.Join(root, "normalization", tt.file), []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}
			assertTemplatingRejected(t, root, tt.file, tt.strict)
		})
	}
	t.Run("yaml config delimiter", func(t *testing.T) {
		root := previewContent(t)
		config := filepath.Join(root, "normalization/config.yaml")
		data, err := os.ReadFile(config)
		if err != nil {
			t.Fatal(err)
		}
		templated := bytes.Replace(data, []byte(`"disable_anonymized_analytics": true`), []byte(`"disable_anonymized_analytics": {{ 'invented_probe' != '' }}`), 1)
		if bytes.Equal(templated, data) {
			t.Fatal("test did not template the project config")
		}
		if err = os.WriteFile(config, templated, 0600); err != nil {
			t.Fatal(err)
		}
		assertTemplatingRejected(t, root, "config.yaml", false)
	})
}

// assertTemplatingRejected requires the templating diagnostic itself, so an
// unavailable native runtime (also exit 3 without a run) cannot pass.
func assertTemplatingRejected(t *testing.T, root, file string, strict bool) {
	t.Helper()
	args := []string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--format", "json"}
	want := 0
	if strict {
		args = append(args, "--strict")
		want = 3
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute(args, &out, &diagnostics)
	var report contracts.ExplainReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v %s %s", err, &out, &diagnostics)
	}
	if code != want || report.Runtime["run"] != nil || report.Runtime["scratch"] != nil {
		t.Fatalf("templated content reached scratch or native runtime: got %d want %d %s %s", code, want, &out, &diagnostics)
	}
	if len(report.Diagnostics) != 1 {
		t.Fatalf("want one templating diagnostic: %s", &out)
	}
	d := report.Diagnostics[0]
	if d.Category != "incomplete-capability" || d.Code != "preview-failed" || !strings.Contains(d.Message, "unsupported Jinja templating") || !strings.HasSuffix(d.Message, file) {
		t.Fatalf("templating not identified: %+v", d)
	}
	if strings.Contains(out.String(), "invented_probe") {
		t.Fatalf("diagnostic echoed project content: %s", &out)
	}
}

func TestPreviewAdmitsNativeMacrosAndTemplateLikeSeedData(t *testing.T) {
	root := previewContent(t)
	path := filepath.Join(root, "normalization/models/api_activity.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	native := strings.Replace(string(data), "kind FULL);\n", "kind FULL);\n\n@DEF(mapped_status, 'mapped');\n", 1)
	native = strings.Replace(native, "s.mapping_status='mapped'", "s.mapping_status=@mapped_status", 1)
	if !strings.Contains(native, "@DEF(") || !strings.Contains(native, "=@mapped_status") {
		t.Fatal("test did not add a native SQLMesh macro")
	}
	if err = os.WriteFile(path, []byte(native), 0600); err != nil {
		t.Fatal(err)
	}
	// CSV is data rather than SQL code; template-like text there stays admissible.
	if err = os.MkdirAll(filepath.Join(root, "normalization/seeds"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "normalization/seeds/labels.csv"), []byte("label\n\"{{ 40 + 2 }}\"\n\"{% raw %}\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--format", "json", "--strict"}, &out, &diagnostics)
	var report contracts.ExplainReport
	if err = json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v %s %s", err, &out, &diagnostics)
	}
	if code != 0 || report.Runtime["run"] == nil || len(report.Samples) == 0 {
		t.Fatalf("native SQLMesh content not previewed: %d %s %s", code, &out, &diagnostics)
	}
}

func TestInspectionOperationalFailureIsNotIntegrityFailure(t *testing.T) {
	root := previewContent(t)
	if err := os.WriteFile(filepath.Join(root, "runtime/sqlmesh/inspect_preview.py"), []byte("raise OSError('invented unavailable output')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--format", "json", "--strict"}, &out, &diagnostics)
	if code != 1 {
		t.Fatalf("operational failure misclassified: %d %s %s", code, &out, &diagnostics)
	}
}

func TestExplainUsesContextSelectedNormalizationProject(t *testing.T) {
	root := previewContent(t)
	selected := filepath.Join(root, "other-models")
	copyTree(t, filepath.Join(root, "normalization"), selected)
	model := filepath.Join(selected, "models/api_activity.sql")
	data, err := os.ReadFile(model)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(model, bytes.ReplaceAll(data, []byte("'object-read'"), []byte("'wrong-context-action'")), 0600); err != nil {
		t.Fatal(err)
	}
	binding := filepath.Join(root, "binding.yaml")
	text := "apiVersion: batz/v0alpha1\nkind: PipelineBinding\nid: demo\ndefinition: repo://pipelines/aws-cloudtrail.yaml\nversion: 1\nroute: local_raw\nnormalization:\n  project: ./other-models\n"
	if err = os.WriteFile(binding, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--binding", binding, "--level", "3", "--format", "json", "--strict"}, &out, &diagnostics)
	if code != 4 || !strings.Contains(out.String(), "required") {
		t.Fatalf("context project ignored: %d %s %s", code, &out, &diagnostics)
	}
}

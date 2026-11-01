package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func previewContent(t *testing.T) string {
	t.Helper()
	root := nativeContent(t)
	if err := os.MkdirAll(filepath.Join(root, "pipelines/aws-cloudtrail/fixtures"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"api-read.json", "api-read.expected.json"} {
		data, err := os.ReadFile(filepath.Join("../../pipelines/aws-cloudtrail/fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "pipelines/aws-cloudtrail/fixtures", name), data, 0600); err != nil {
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
		{"query block", "models/aws-cloudtrail/api_activity.sql", model + `JINJA_QUERY_BEGIN;
SELECT 40 + 2 AS invented_probe;
JINJA_END;
`, true},
		{"lowercase statement block", "models/aws-cloudtrail/api_activity.sql", model + `jinja_statement_begin;
SELECT 40 + 2 AS invented_probe;
Jinja_End;
`, false},
		{"expression delimiter", "models/aws-cloudtrail/api_activity.sql", model + `SELECT {{ 40 + 2 }} AS invented_probe;
`, true},
		{"statement delimiter", "models/aws-cloudtrail/api_activity.sql", model + `{% set invented_probe = 42 %}
SELECT 42 AS answer;
`, false},
		{"comment delimiter", "models/aws-cloudtrail/api_activity.sql", model + `{# invented_probe #}
SELECT 42 AS answer;
`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := previewContent(t)
			if err := os.WriteFile(filepath.Join(root, ".", tt.file), []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}
			assertTemplatingRejected(t, root, tt.file, tt.strict)
		})
	}
	t.Run("yaml config delimiter", func(t *testing.T) {
		root := previewContent(t)
		config := filepath.Join(root, "config.yaml")
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
	args := []string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail/pipeline.yaml"), "--project", root, "--level", "3", "--format", "json"}
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

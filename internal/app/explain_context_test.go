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

func TestExplainPreservesTimestampDefaultText(t *testing.T) {
	const timestamp = "2026-10-25T12:00:00.120000000+10:00"
	root := t.TempDir()
	data, err := os.ReadFile("../../pipelines/aws-cloudtrail.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("      params:\n"), []byte("      params:\n        since: {type: timestamp, required: false, env: BATZ_SINCE, default: "+timestamp+"}\n"), 1)
	definition := filepath.Join(root, "source.yaml")
	if err := os.WriteFile(definition, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", definition, "--project", root, "--format", "json"}, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("static explain failed: %d %s %s", code, &out, &diagnostics)
	}
	var report contracts.ExplainReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := json.Unmarshal(report.Resolution["input.since"].Value, &got); err != nil {
		t.Fatal(err)
	}
	if got != timestamp {
		t.Fatalf("timestamp default changed: got %q want %q", got, timestamp)
	}
}

func TestRunAggregatesInvalidInputWithMissingValuesAndAssets(t *testing.T) {
	root := t.TempDir()
	data, err := os.ReadFile("../../pipelines/aws-cloudtrail.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("      params:\n"), []byte("      params:\n        since: {type: timestamp, required: false, env: BATZ_SINCE}\n        prefix: {type: string, required: true, env: BATZ_PREFIX}\n"), 1)
	definition := filepath.Join(root, "source.yaml")
	if err := os.WriteFile(definition, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", definition, "--project", root, "--route", "local_raw", "--input", "since=invalid-time"}, &out, &diagnostics)
	if code != 2 {
		t.Fatalf("invalid static selection: got %d %s %s", code, &out, &diagnostics)
	}
	for _, problem := range []string{"missing-workspace", "input.path", "input.prefix", "invalid timestamp since", "missing-asset"} {
		if !strings.Contains(diagnostics.String(), problem) {
			t.Fatalf("missing %q in aggregated diagnostics: %s", problem, &diagnostics)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("invalid preflight created state: %v", entries)
	}
}

func writeContextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExplainHistoricalPrecedenceTracesAndTypedCLIValues(t *testing.T) {
	project := t.TempDir()
	contextDir := t.TempDir()
	workspace := filepath.Join(contextDir, "laptop")
	definition := filepath.Join(project, "source.yaml")
	// These are alpha-contract adaptations of the historical design examples.
	// The remote route is declared for static explanation, never executed.
	writeContextFile(t, definition, `apiVersion: batz/v0alpha2
kind: SourcePipeline
name: aws-cloudtrail
version: 1
normalization: {project: repo://normalization/cloudtrail, staging: cloudtrail.source_staging, evidence: cloudtrail.build_evidence, checks: cloudtrail.required_checks}
routes:
  s3_raw:
    input:
      engine: bento
      framing: records-json
      config: repo://collector/s3.yaml
      params:
        bucket: {type: string, required: true, env: BATZ_BUCKET, default: route-bucket}
        prefix: {type: string, required: true, env: BATZ_PREFIX}
        prefixes: {type: string-list, required: false, env: BATZ_PREFIXES}
        since: {type: timestamp, required: false, env: BATZ_SINCE}
    output:
      parsed: {format: parquet}
      normalized: {models: {api_activity: cloudtrail.api_activity}}
`)
	writeContextFile(t, filepath.Join(project, "workspace.yaml"), `apiVersion: batz/v0alpha1
kind: Workspace
storage: {parsed: workspace://parsed, normalized: workspace://normalized}
credentials:
  aws: {provider: aws_profile, profile: responder-readonly}
  okta: {provider: 1password, reference: "op://DFIR/Okta/api-token"}
`)
	writeContextFile(t, filepath.Join(workspace, "workspace.yaml"), `apiVersion: batz/v0alpha1
kind: Workspace
credentials:
  aws: {provider: aws_profile, profile: laptop-readonly}
pipeline_defaults:
  aws-cloudtrail:
    s3_raw:
      input: {bucket: example-evidence}
`)
	binding := filepath.Join(contextDir, "binding.yaml")
	writeContextFile(t, binding, `apiVersion: batz/v0alpha1
kind: PipelineBinding
id: standalone
workspace: ./laptop
definition: repo://source.yaml
version: 1
route: s3_raw
input: {prefix: AWSLogs/example/}
`)
	caseFile := filepath.Join(contextDir, "case.yaml")
	writeContextFile(t, caseFile, `apiVersion: batz/v0alpha1
kind: Case
id: investigation
workspace: ./laptop
credentials:
  aws: {provider: aws_profile, profile: case-readonly}
pipelines:
  - id: aws-account-a
    definition: repo://source.yaml
    version: 1
    route: s3_raw
    input: {bucket: case-evidence, prefix: AWSLogs/example/}
`)
	tests := []struct {
		name         string
		args         []string
		bucket       string
		profile      string
		bucketSource string
	}{
		{name: "standalone", args: []string{"--binding", binding}, bucket: "example-evidence", profile: "laptop-readonly", bucketSource: filepath.Join(workspace, "workspace.yaml")},
		{name: "case", args: []string{"--case", caseFile, "--binding-id", "aws-account-a"}, bucket: "case-evidence", profile: "case-readonly", bucketSource: caseFile},
		{name: "standalone CLI override", args: []string{"--binding", binding, "--input", "bucket=cli-evidence"}, bucket: "cli-evidence", profile: "laptop-readonly", bucketSource: "cli"},
		{name: "case CLI override", args: []string{"--case", caseFile, "--binding-id", "aws-account-a", "--input", "bucket=cli-evidence"}, bucket: "cli-evidence", profile: "case-readonly", bucketSource: "cli"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"explain", definition, "--project", project, "--format", "json", "--input", `prefixes=["a/","b/"]`, "--input", "since=2026-10-25T12:00:00.120000000+10:00"}
			args = append(args, tt.args...)
			var out, diagnostics bytes.Buffer
			code := app.Execute(args, &out, &diagnostics)
			if code != 0 {
				t.Fatalf("static context failed: %d %s %s", code, &out, &diagnostics)
			}
			var report contracts.ExplainReport
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			var bucket, parsed string
			var credential map[string]string
			for key, dst := range map[string]any{"input.bucket": &bucket, "storage.parsed": &parsed, "credentials.aws": &credential} {
				if err := json.Unmarshal(report.Resolution[key].Value, dst); err != nil {
					t.Fatalf("decode %s: %v", key, err)
				}
			}
			if bucket != tt.bucket || credential["profile"] != tt.profile || report.Resolution["input.bucket"].Source != tt.bucketSource {
				t.Fatalf("wrong precedence: %s", &out)
			}
			if parsed != filepath.Join(workspace, "parsed") || !bytes.Contains(report.Resolution["credentials.okta"].Value, []byte("op://DFIR/Okta/api-token")) {
				t.Fatalf("unrelated inherited defaults lost: %s", &out)
			}
			if string(report.Resolution["input.prefixes"].Value) != `["a/","b/"]` || string(report.Resolution["input.since"].Value) != `"2026-10-25T12:00:00.120000000+10:00"` {
				t.Fatalf("typed CLI values changed: %s", &out)
			}
			if len(report.Runtime) != 0 || len(report.Diagnostics) == 0 {
				t.Fatalf("static explain should disclose missing assets without execution: %s", &out)
			}
		})
	}
}

func assertLosslessStructures(t *testing.T, raw []byte) {
	t.Helper()
	var report contracts.ExplainReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	structures := make(map[string]map[string]contracts.ExplainField)
	for _, structure := range report.Structures {
		fields := make(map[string]contracts.ExplainField)
		for _, field := range structure.Fields {
			fields[field.Name] = field
		}
		structures[structure.Stage] = fields
	}
	if len(structures) != 5 || structures["parsed"]["payload"].NativeType != "VARCHAR" || structures["parsed"]["payload"].Encoding != "json-text" {
		t.Fatalf("source/intermediate structure or JSON text encoding lost: %s", string(raw))
	}
	event := structures["api_activity"]["event"]
	if !strings.HasPrefix(event.NativeType, "STRUCT(") || len(event.Fields) == 0 {
		t.Fatalf("nested event columns flattened: %+v", event)
	}
	integer := structures["api_activity"]["example_large_integer"]
	instant := structures["api_activity"]["example_nanosecond_time"]
	if integer.NativeType != "BIGINT" || integer.Encoding != "decimal-string-if-unsafe" || instant.NativeType != "TIMESTAMP_NS" || instant.Encoding != "iso-string" {
		t.Fatalf("native type/encoding lost: integer=%+v instant=%+v", integer, instant)
	}
	for _, sample := range report.Samples {
		if sample.Stage != "api_activity" {
			continue
		}
		if len(sample.Records) != 1 {
			t.Fatalf("unexpected sample records: %+v", sample)
		}
		record, ok := sample.Records[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected record type: %T", sample.Records[0])
		}
		if record["example_large_integer"] != "9007199254740993" || record["example_nanosecond_time"] != "2026-10-25T12:00:00.000000001" {
			t.Fatalf("native values lost precision: %+v", record)
		}
		return
	}
	t.Fatal("normalized sample unavailable")
}

func TestPreviewRejectsAutomaticExtensionInstallationBeforeLaunch(t *testing.T) {
	tests := []struct {
		name       string
		connection string
		setting    string
		args       []string
		want       int
	}{
		{name: "data install", connection: "connection", setting: "autoinstall_known_extensions", args: []string{"--strict"}, want: 3},
		{name: "state install", connection: "state_connection", setting: "autoinstall_known_extensions", args: []string{"--strict"}, want: 3},
		{name: "data load default explain", connection: "connection", setting: "autoload_known_extensions", want: 0},
		{name: "state load", connection: "state_connection", setting: "autoload_known_extensions", args: []string{"--strict"}, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := previewContent(t)
			path := filepath.Join(root, "normalization/cloudtrail/config.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			settings := config
			for _, key := range []string{"gateways", "local", tt.connection, "connector_config"} {
				nested, ok := settings[key].(map[string]any)
				if !ok {
					t.Fatalf("invalid fixture configuration at %s", key)
				}
				settings = nested
			}
			settings[tt.setting] = true
			changed, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, changed, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--format", "json"}
			args = append(args, tt.args...)
			var out, diagnostics bytes.Buffer
			code := app.Execute(args, &out, &diagnostics)
			var report contracts.ExplainReport
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatalf("decode preview: %v %s %s", err, &out, &diagnostics)
			}
			if code != tt.want || report.Runtime["run"] != nil || report.Runtime["scratch"] != nil || !strings.Contains(out.String(), "extension installation") {
				t.Fatalf("automatic installation was not refused before launch: %d %s %s", code, &out, &diagnostics)
			}
		})
	}
}

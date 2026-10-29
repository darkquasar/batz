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

func TestPreflightEmitsStructuredDiagnostic(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root}, &out, &stderr)
	var diagnostic contracts.Diagnostic
	if err = json.Unmarshal(stderr.Bytes(), &diagnostic); err != nil {
		t.Fatalf("unstructured diagnostic: %s", &stderr)
	}
	if code != 2 || diagnostic.Category != "invalid-input" || diagnostic.Code != "missing-workspace" || diagnostic.Field != "workspace" || diagnostic.Source != "cli" {
		t.Fatalf("wrong diagnostic: %d %+v", code, diagnostic)
	}
}
func TestReceiptFailurePreservesIntegrityClassification(t *testing.T) {
	root := nativeContent(t)
	path := filepath.Join(root, "collector/configs/cloudtrail/local-records.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(b), `count("records") - 1`, `0`, 1)
	if changed == string(b) {
		t.Fatal("mutation missing")
	}
	if err = os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "two.json")
	writeRecords(t, input, []map[string]any{sourceRecords(t), sourceRecords(t)})
	var out, stderr bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", t.TempDir(), "--input", "path=" + input}, &out, &stderr)
	var report contracts.RunReport
	if err = json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 4 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Category != "integrity" || report.Diagnostics[0].Code != "receipt-integrity" {
		t.Fatalf("wrong integrity outcome: %d %s %s", code, &out, &stderr)
	}
}

package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"path/filepath"
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

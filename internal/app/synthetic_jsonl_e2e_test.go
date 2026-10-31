//go:build batz_synthetic_jsonl

package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestE2ESyntheticJSONLPipelineUsesItsOwnProjectAndOutput(t *testing.T) {
	root := nativeContent(t)
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", t.TempDir(), "--binding-id", "probe", "--input", "path=" + filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl")}, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("JSONL pipeline failed: %d %s %s", code, &out, &diagnostics)
	}
	var report contracts.RunReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if got := string(query(t, root, *report.Catalog, `SELECT event.action,user.id,typeof("@timestamp"),epoch_ns("@timestamp") FROM probe.findings ORDER BY event.id`)); got != `[["observed","invented-user","TIMESTAMP_NS",1792929600000000001],["observed","other-user","TIMESTAMP_NS",1792929600123000000]]` {
		t.Fatal(got)
	}
}

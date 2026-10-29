package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalFixturePublishesQueryableRevision(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + filepath.Join(root, "fixtures/cloudtrail/api-read.json")}, &out, &diagnostics)
	if code != 0 {
		_ = filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && filepath.Ext(path) == ".log" {
				content, _ := os.ReadFile(path)
				t.Logf("native log %s:\n%s", path, content)
			}
			return nil
		})
		t.Fatalf("local run must publish a queryable revision; exit=%d stderr=%s stdout=%s", code, &diagnostics, &out)
	}
	var report contracts.RunReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("run report: %v: %s", err, &out)
	}
	if report.NormalizedRevision == nil || report.Catalog == nil || report.Quality == nil || *report.Quality != "complete" {
		t.Fatalf("incomplete publication: %+v", report)
	}
	python := filepath.Join(root, "runtime/.local/venv/bin/python")
	script := "import duckdb,json,sys; c=duckdb.connect(sys.argv[1],read_only=True); print(json.dumps(c.execute(\"SELECT event.action,user.id,epoch_ns(\\\"@timestamp\\\"),json_extract_string(batz.source_payload,'$.unmapped_example'),source.ip FROM demo.api_activity\").fetchall()))"
	cmd := exec.Command(python, "-c", script, *report.Catalog)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	actual, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ordinary reader: %v: %s", err, actual)
	}
	expected := []any{[]any{"object-read", "arn:aws:iam::111122223333:user/example-responder", time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC).UnixNano(), "kept", "192.0.2.10"}}
	want, _ := json.Marshal(expected)
	if !bytes.Equal(bytes.TrimSpace(actual), want) {
		var decoded any
		if err := json.Unmarshal(actual, &decoded); err != nil {
			t.Fatal(err)
		}
		// Compare decoded JSON through its canonical encoding; Python spacing is immaterial.
		canonical, _ := json.Marshal(decoded)
		if !bytes.Equal(canonical, want) {
			t.Fatalf("published semantics: got %s want %s", canonical, want)
		}
	}
}

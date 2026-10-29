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

func TestUnsupportedSourceShapeCannotCommitParsedInput(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"eventTime,eventName\n2026-01-01T00:00:00Z,GetObject\n", `[{"eventName":"GetObject"}]`} {
		input := filepath.Join(t.TempDir(), "unsupported.txt")
		if err = os.WriteFile(input, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		workspace := t.TempDir()
		var out, diagnostics bytes.Buffer
		code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--input", "path=" + input}, &out, &diagnostics)
		var report contracts.RunReport
		if err = json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if code == 0 || report.AcquisitionComplete || report.ParsedRevision != nil || report.NormalizedRevision != nil || !strings.Contains(diagnostics.String(), "unsupported-shape") {
			t.Fatalf("unsupported input outcome: %d %s %s", code, &out, &diagnostics)
		}
	}
}

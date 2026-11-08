package app_test

import (
	"batz/internal/app"
	"bytes"
	"encoding/json"
	"testing"
)

func TestVersionNeedsNoContentOrRuntime(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, diagnostics bytes.Buffer
	if code := app.Execute([]string{"version", "--format", "json"}, &out, &diagnostics); code != 0 {
		t.Fatalf("version exit=%d: %s", code, &diagnostics)
	}
	var identity struct {
		Version     string `json:"version"`
		SourceSHA   string `json:"source_sha"`
		Target      string `json:"target"`
		Development bool   `json:"development"`
		Dirty       bool   `json:"dirty"`
	}
	if err := json.Unmarshal(out.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Version != "development" || identity.SourceSHA != "unknown" || identity.Target == "" || !identity.Development {
		t.Fatalf("unstamped binary claimed release identity: %s", &out)
	}
}

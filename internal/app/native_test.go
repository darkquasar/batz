package app_test

import (
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeEvents(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	var out bytes.Buffer
	for _, record := range records {
		if err := json.NewEncoder(&out).Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func query(t *testing.T, root, catalog, sql string) []byte {
	t.Helper()
	script := "import duckdb,json,sys; c=duckdb.connect(sys.argv[1],read_only=True); print(json.dumps(c.execute(sys.argv[2]).fetchall(),separators=(',',':')))"
	cmd := exec.Command(filepath.Join(root, "runtime/.local/venv/bin/python"), "-I", "-c", script, catalog, sql)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ordinary SQL query: %v %s", err, b)
	}
	return bytes.TrimSpace(b)
}

func assertNativeResources(t *testing.T, report contracts.RunReport) {
	t.Helper()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	limits := value["limits"].(map[string]any)
	if limits["collector_memory_bytes"] != nil || limits["model_memory_bytes"] != nil {
		t.Fatal("owner removed RAM cutoffs, but report still declares them")
	}
	resources, ok := value["resources"].(map[string]any)
	if !ok {
		t.Fatal("resource measurements missing")
	}
	for _, stage := range []string{"collection", "model"} {
		if _, ok := resources[stage].(map[string]any); !ok {
			t.Fatalf("missing stage measurements: %s", stage)
		}
	}
	model := resources["model"].(map[string]any)
	if model["samples"].(float64) < 2 || model["average_rss_bytes"].(float64) <= 0 || model["peak_rss_bytes"].(float64) < model["average_rss_bytes"].(float64) || model["sample_interval_ms"].(float64) != 100 {
		t.Fatalf("invalid native measurements: %v", model)
	}
	t.Logf("native model RAM measurements: %v", model)
}

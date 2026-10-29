package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func runFixture(t *testing.T, root, workspace, input string, extra ...string) contracts.RunReport {
	t.Helper()
	args := []string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}
	args = append(args, extra...)
	var out, diagnostics bytes.Buffer
	code := app.Execute(args, &out, &diagnostics)
	if code != 0 {
		_ = filepath.WalkDir(workspace, func(path string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && (filepath.Base(path) == "model.log" || filepath.Base(path) == "collector.log") {
				b, _ := os.ReadFile(path)
				t.Logf("%s\n%s", path, b)
			}
			return nil
		})
		t.Fatalf("native run exit=%d %s %s", code, &diagnostics, &out)
	}
	var report contracts.RunReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	return report
}
func sourceRecords(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile("../../fixtures/cloudtrail/api-read.json")
	if err != nil {
		t.Fatal(err)
	}
	var v map[string][]map[string]any
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v["Records"][0]
}
func writeRecords(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"Records": records})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
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
func TestSyntheticNanosecondProbeSurvivesNativePublication(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "synthetic-stack-precision.json")
	records := []map[string]any{}
	for i, instant := range []string{"2026-10-25T12:00:00.000000001Z", "2026-10-25T12:00:00.000000002Z", "2026-10-25T12:00:00.123Z"} {
		r := sourceRecords(t)
		r["eventTime"] = instant
		r["eventID"] = []string{"nano-1", "nano-2", "coarse"}[i]
		r["synthetic_stack_probe"] = true
		records = append(records, r)
	}
	writeRecords(t, input, records)
	report := runFixture(t, root, t.TempDir(), input)
	got := query(t, root, *report.Catalog, "SELECT event.id,epoch_ns(\"@timestamp\"),batz.time.fraction_digits,typeof(\"@timestamp\") FROM demo.api_activity ORDER BY event.id")
	epoch := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC).UnixNano()
	want, _ := json.Marshal([]any{[]any{"coarse", epoch + 123000000, 3, "TIMESTAMP_NS"}, []any{"nano-1", epoch + 1, 9, "TIMESTAMP_NS"}, []any{"nano-2", epoch + 2, 9, "TIMESTAMP_NS"}})
	if !bytes.Equal(got, want) {
		t.Fatalf("precision loss: got %s want %s", got, want)
	}
}
func TestSameIntervalNewHistoricalMembershipIsEvaluated(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "selected.json")
	workspace := t.TempDir()
	writeRecords(t, input, []map[string]any{sourceRecords(t)})
	a := runFixture(t, root, workspace, input)
	historical := sourceRecords(t)
	historical["eventID"] = "historical"
	historical["eventTime"] = "2020-01-01T00:00:00Z"
	writeRecords(t, input, []map[string]any{historical})
	b := runFixture(t, root, workspace, input)
	if len(b.Inputs) != 2 || len(b.Counts) != 2 || b.Counts[0].Accepted != 2 {
		t.Fatalf("report does not describe committed membership: inputs=%d counts=%v", len(b.Inputs), b.Counts)
	}
	for _, key := range []string{"pipeline", "collector-config", "runtime-lock", "model-runner", "coordinator"} {
		if len(b.Content[key]) != 64 {
			t.Fatalf("missing executed content identity: %s", key)
		}
	}
	if *a.NormalizedRevision == *b.NormalizedRevision {
		t.Fatal("changed membership reused normalized revision")
	}
	if got := string(query(t, root, *a.Catalog, "SELECT count(*) FROM demo.api_activity")); got != "[[1]]" {
		t.Fatalf("prior reader changed: %s", got)
	}
	if got := string(query(t, root, *b.Catalog, "SELECT count(*),count(DISTINCT batz.input_digest),count(DISTINCT batz.evaluation_nonce) FROM demo.api_activity")); got != "[[2,1,1]]" {
		t.Fatalf("full membership not freshly evaluated: %s", got)
	}
	if got := string(query(t, root, *b.Catalog, "SELECT year(\"@timestamp\") FROM demo.api_activity WHERE event.id='historical'")); got != "[[2020]]" {
		t.Fatalf("historical input omitted: %s", got)
	}
}
func TestExplicitEmptyNativeInputPublishesCheckedEmptyRevision(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "empty.json")
	writeRecords(t, input, []map[string]any{})
	report := runFixture(t, root, t.TempDir(), input)
	if report.Quality == nil || *report.Quality != "complete" {
		t.Fatal("empty completion not explicit")
	}
	if got := string(query(t, root, *report.Catalog, "SELECT count(*) FROM demo.api_activity")); got != "[[0]]" {
		t.Fatal(got)
	}
}

func TestOffsetMissingTimeAndDistinctOrigins(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "times.json")
	records := []map[string]any{}
	for i, instant := range []string{"2026-10-25T22:00:00.000000001+10:00", "2026-10-25T12:00:00.000000001Z", ""} {
		r := sourceRecords(t)
		r["eventID"] = []string{"offset", "utc", "undated"}[i]
		if instant == "" {
			delete(r, "eventTime")
		} else {
			r["eventTime"] = instant
		}
		records = append(records, r)
	}
	writeRecords(t, input, records)
	report := runFixture(t, root, t.TempDir(), input)
	got := string(query(t, root, *report.Catalog, `SELECT count(*),count(DISTINCT batz.origins[1].observation_id),count(DISTINCT epoch_ns("@timestamp")),count("@timestamp") FROM demo.api_activity`))
	if got != "[[3,3,1,2]]" {
		t.Fatalf("offset/undated/origin mismatch: %s", got)
	}
}

func TestMalformedMappedFieldTypesBecomeDurableRejects(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "types.json")
	records := []map[string]any{sourceRecords(t)}
	for _, field := range []string{"eventID", "awsRegion", "accountId", "sourceIPAddress", "errorCode"} {
		r := sourceRecords(t)
		if field == "accountId" {
			r["userIdentity"].(map[string]any)[field] = map[string]any{"bad": true}
		} else {
			r[field] = 42
		}
		records = append(records, r)
	}
	writeRecords(t, input, records)
	report := runFixture(t, root, t.TempDir(), input)
	if report.Quality == nil || *report.Quality != "partial" || len(report.Counts) != 2 || report.Counts[1].Rejected != 5 || report.Coverage.Mapped != 1 {
		t.Fatalf("invalid types silently mapped: %+v", report)
	}
	if got := string(query(t, root, *report.Catalog, "SELECT count(*) FROM demo.source_staging WHERE mapping_status='rejected' AND mapping_reject_reason IS NOT NULL")); got != "[[5]]" {
		t.Fatal(got)
	}
}

func TestRunReportsRAMMeasurementsWithoutMemoryCutoff(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	report := runFixture(t, root, t.TempDir(), filepath.Join(root, "fixtures/cloudtrail/api-read.json"))
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

func TestNativeBindingsRemainIndependentAcrossABA(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sourceDir := t.TempDir()
	aInput, bInput := filepath.Join(sourceDir, "a.json"), filepath.Join(sourceDir, "b.json")
	aRecord, bRecord := sourceRecords(t), sourceRecords(t)
	aRecord["eventID"] = "a-first"
	bRecord["eventID"] = "b-only"
	writeRecords(t, aInput, []map[string]any{aRecord})
	writeRecords(t, bInput, []map[string]any{bRecord})
	a := runFixture(t, root, workspace, aInput)
	b := runFixture(t, root, workspace, bInput, "--binding-id", "other")
	aRecord["eventID"] = "a-second"
	writeRecords(t, aInput, []map[string]any{aRecord})
	again := runFixture(t, root, workspace, aInput)
	if got := string(query(t, root, *a.Catalog, "SELECT event.id FROM demo.api_activity")); got != `[["a-first"]]` {
		t.Fatal(got)
	}
	if got := string(query(t, root, *b.Catalog, "SELECT event.id FROM other.api_activity")); got != `[["b-only"]]` {
		t.Fatal(got)
	}
	if got := string(query(t, root, *again.Catalog, "SELECT event.id FROM demo.api_activity ORDER BY event.id")); got != `[["a-first"],["a-second"]]` {
		t.Fatal(got)
	}
}

func TestExistingReaderSelectsNewRevisionAndIgnoresUncommittedFiles(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	input := filepath.Join(root, "fixtures/cloudtrail/api-read.json")
	a := runFixture(t, root, workspace, input)
	orphan := filepath.Join(workspace, ".batz/parsed/uncommitted")
	if err = os.MkdirAll(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	parsed, err := os.ReadFile(filepath.Join(workspace, ".batz/parsed", string(*a.ParsedRevision), "parsed.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(orphan, "parsed.parquet"), parsed, 0600); err != nil {
		t.Fatal(err)
	}
	script := `import duckdb,json,sys
c=duckdb.connect(sys.argv[1],read_only=True)
assert c.execute("SELECT count(*) FROM demo.api_activity").fetchone()[0]==1
print("READY",flush=True)
path=sys.stdin.readline().strip()
c.execute("ATTACH '"+path.replace("'","''")+"' AS revision_b (READ_ONLY)")
print(json.dumps(c.execute("SELECT (SELECT count(*) FROM demo.api_activity),(SELECT count(*) FROM revision_b.demo.api_activity)").fetchall(),separators=(',',':')),flush=True)
c.close()`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime/.local/venv/bin/python"), "-I", "-c", script, *a.Catalog)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		stdin.Close()
		if !waited {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	ready, err := reader.ReadString('\n')
	if err != nil || ready != "READY\n" {
		t.Fatalf("reader setup: %v %s %s", err, ready, &stderr)
	}
	b := runFixture(t, root, workspace, input)
	if _, err = io.WriteString(stdin, *b.Catalog+"\n"); err != nil {
		t.Fatal(err)
	}
	observed, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("existing reader: %v %s", err, &stderr)
	}
	if observed != "[[1,2]]\n" {
		t.Fatalf("mixed/stray membership: %s", observed)
	}
	err = cmd.Wait()
	waited = true
	if err != nil {
		t.Fatalf("reader: %v %s", err, &stderr)
	}
}

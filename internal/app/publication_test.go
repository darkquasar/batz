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

func nativeContent(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	content := t.TempDir()
	for _, dir := range []string{"pipelines", "normalization", "collector/configs", "runtime/sqlmesh"} {
		copyTree(t, filepath.Join(root, dir), filepath.Join(content, dir))
	}
	lock, err := os.ReadFile(filepath.Join(root, "runtime/dependencies.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(content, "runtime/dependencies.lock.json"), lock, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(root, "runtime/.local"), filepath.Join(content, "runtime/.local")); err != nil {
		t.Fatal(err)
	}
	return content
}
func TestWrongTimestampCannotReplacePublishedRevision(t *testing.T) {
	root := nativeContent(t)
	input, err := filepath.Abs("../../fixtures/cloudtrail/api-read.json")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	prior := runFixture(t, root, workspace, input)
	path := filepath.Join(root, "normalization/models/api_activity.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := strings.Replace(string(b), "s.occurrence_time AS", "make_timestamp_ns(epoch_ns(s.occurrence_time)+1) AS", 1)
	if wrong == string(b) {
		t.Fatal("test did not mutate native SQL")
	}
	if err = os.WriteFile(path, []byte(wrong), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &out, &diagnostics)
	var failed contracts.RunReport
	if err = json.Unmarshal(out.Bytes(), &failed); err != nil {
		t.Fatal(err)
	}
	if code == 0 || failed.Outcome != "normalization-failed" || !failed.AcquisitionComplete || failed.ParsedRevision == nil || failed.NormalizedRevision == nil || *failed.NormalizedRevision != *prior.NormalizedRevision {
		t.Fatalf("wrong instant published or evidence lost: exit=%d report=%s diagnostics=%s", code, &out, &diagnostics)
	}
	if code != 4 || len(failed.Diagnostics) != 1 || failed.Diagnostics[0].Category != "assertion" || failed.Diagnostics[0].Code != "required-check" {
		t.Fatalf("wrong assertion classification: %d %s", code, &out)
	}
	if got := string(query(t, root, *prior.Catalog, "SELECT count(*) FROM demo.api_activity")); got != "[[1]]" {
		t.Fatal(got)
	}
}

func TestNativeBuildEvidenceCannotBeStaleEvenForEmptyInput(t *testing.T) {
	for _, field := range []string{"input_digest", "attempt_id", "evaluation_nonce"} {
		t.Run(field, func(t *testing.T) {
			root := nativeContent(t)
			input := filepath.Join(t.TempDir(), "empty.json")
			writeRecords(t, input, []map[string]any{})
			workspace := t.TempDir()
			prior := runFixture(t, root, workspace, input)
			path := filepath.Join(root, "normalization/models/build_evidence.sql")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			changed := strings.Replace(string(b), "c."+field, "'stale' AS "+field, 1)
			if err = os.WriteFile(path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diagnostics bytes.Buffer
			code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &out, &diagnostics)
			var failed contracts.RunReport
			if err = json.Unmarshal(out.Bytes(), &failed); err != nil {
				t.Fatal(err)
			}
			if code == 0 || !failed.AcquisitionComplete || failed.NormalizedRevision == nil || *failed.NormalizedRevision != *prior.NormalizedRevision {
				t.Fatalf("stale empty build accepted: %d %s %s", code, &out, &diagnostics)
			}
			logs, err := filepath.Glob(filepath.Join(workspace, ".batz/builds", string(failed.Identity.Attempt), "model.log"))
			if err != nil || len(logs) != 1 {
				t.Fatal("missing model failure evidence")
			}
			log, err := os.ReadFile(logs[0])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(log, []byte("stale or mismatched native evaluation evidence")) {
				t.Fatalf("wrong negative failure: %s", log)
			}
		})
	}
}
func TestWrongSameTypeActionAndActorFailNativeChecks(t *testing.T) {
	for _, field := range []string{"action", "actor"} {
		t.Run(field, func(t *testing.T) {
			root := nativeContent(t)
			path := filepath.Join(root, "normalization/models/api_activity.sql")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			wrong := strings.Replace(string(b), "'object-read'", "'object-write'", 1)
			if field == "actor" {
				wrong = strings.Replace(string(b), "json_extract_string(s.payload,'$.userIdentity.arn')", "'arn:aws:iam::111111111111:user/wrong'", 1)
			}
			if wrong == string(b) {
				t.Fatal("mutation missing")
			}
			if err = os.WriteFile(path, []byte(wrong), 0600); err != nil {
				t.Fatal(err)
			}
			input, err := filepath.Abs("../../fixtures/cloudtrail/api-read.json")
			if err != nil {
				t.Fatal(err)
			}
			workspace := t.TempDir()
			var out, diagnostics bytes.Buffer
			code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &out, &diagnostics)
			var failed contracts.RunReport
			if err = json.Unmarshal(out.Bytes(), &failed); err != nil {
				t.Fatal(err)
			}
			if code == 0 || failed.NormalizedRevision != nil || !failed.AcquisitionComplete {
				t.Fatalf("wrong %s published: %d %s %s", field, code, &out, &diagnostics)
			}
			log, err := os.ReadFile(filepath.Join(workspace, ".batz/builds", string(failed.Identity.Attempt), "model.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(log, []byte("required published value/identity assertion failed")) {
				t.Fatalf("wrong negative failure: %s", log)
			}
		})
	}
}

func TestExactObservationMembershipAndCompleteOrigins(t *testing.T) {
	for _, mutation := range []string{"duplicate-and-omit", "file", "revision", "row"} {
		t.Run(mutation, func(t *testing.T) {
			root := nativeContent(t)
			input := filepath.Join(t.TempDir(), "two.json")
			first, second := sourceRecords(t), sourceRecords(t)
			first["eventID"] = "first"
			second["eventID"] = "second"
			writeRecords(t, input, []map[string]any{first, second})
			workspace := t.TempDir()
			prior := runFixture(t, root, workspace, input)
			path := filepath.Join(root, "normalization/models/api_activity.sql")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			from, to := "", ""
			switch mutation {
			case "duplicate-and-omit":
				from = "FROM cloudtrail.source_staging s"
				to = "FROM (SELECT * FROM cloudtrail.source_staging WHERE json_extract_string(payload,'$.eventID')='first') s CROSS JOIN range(2) duplicates"
			case "file":
				from = "'file':s.parsed_file"
				to = "'file':'parsed/missing.parquet'"
			case "revision":
				from = "'parsed_revision':s.parsed_revision"
				to = "'parsed_revision':'wrong-revision'"
			case "row":
				from = "'row':s.parsed_row"
				to = "'row':999999::BIGINT"
			}
			changed := strings.Replace(string(b), from, to, 1)
			if changed == string(b) {
				t.Fatal("mutation missing")
			}
			if err = os.WriteFile(path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diagnostics bytes.Buffer
			code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &out, &diagnostics)
			var failed contracts.RunReport
			if err = json.Unmarshal(out.Bytes(), &failed); err != nil {
				t.Fatal(err)
			}
			if code == 0 || !failed.AcquisitionComplete || failed.NormalizedRevision == nil || *failed.NormalizedRevision != *prior.NormalizedRevision {
				t.Fatalf("substituted observation/origin published: %d %s %s", code, &out, &diagnostics)
			}
			if got := string(query(t, root, *prior.Catalog, "SELECT count(*) FROM demo.api_activity")); got != "[[2]]" {
				t.Fatal(got)
			}
		})
	}
}

func TestValidUnrepresentableTimestampStopsBeforeNormalizedPublication(t *testing.T) {
	for _, instant := range []string{"2026-10-25T12:00:00.0000000001Z", "1500-01-01T00:00:00Z"} {
		t.Run(instant, func(t *testing.T) {
			root := nativeContent(t)
			input := filepath.Join(t.TempDir(), "source.json")
			writeRecords(t, input, []map[string]any{sourceRecords(t)})
			workspace := t.TempDir()
			prior := runFixture(t, root, workspace, input)
			unsupported := sourceRecords(t)
			unsupported["eventTime"] = instant
			writeRecords(t, input, []map[string]any{unsupported})
			var out, stderr bytes.Buffer
			code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &out, &stderr)
			var report contracts.RunReport
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if code != 3 || !report.AcquisitionComplete || report.NormalizedRevision == nil || *report.NormalizedRevision != *prior.NormalizedRevision || report.Quality != nil || len(report.Diagnostics) != 1 || report.Diagnostics[0].Category != "incomplete-capability" || report.Diagnostics[0].Code != "timestamp-representation-unavailable" {
				t.Fatalf("timestamp limitation silently accepted: %d %s %s", code, &out, &stderr)
			}
			log, err := os.ReadFile(filepath.Join(workspace, ".batz/builds", string(report.Identity.Attempt), "model.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(log, []byte("timestamp-representation-unavailable")) {
				t.Fatalf("missing explicit representation blocker: %s", log)
			}
		})
	}
}

func TestStrictPartialRemainsPublished(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "mixed.json")
	valid, rejected, unsupported := sourceRecords(t), sourceRecords(t), sourceRecords(t)
	rejected["eventTime"] = "invalid-time"
	unsupported["eventName"] = "PutObject"
	writeRecords(t, input, []map[string]any{valid, rejected, unsupported})
	workspace := t.TempDir()
	var out, stderr bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input, "--strict"}, &out, &stderr)
	var report contracts.RunReport
	if err = json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 6 || report.Outcome != "published" || report.Quality == nil || *report.Quality != "partial" || report.NormalizedRevision == nil || report.Coverage.Mapped != 1 || report.Coverage.Unsupported != 1 {
		t.Fatalf("strict partial not retained: %d %s %s", code, &out, &stderr)
	}
	if got := string(query(t, root, *report.Catalog, "SELECT count(*) FROM demo.api_activity")); got != "[[1]]" {
		t.Fatal(got)
	}
	saved, err := os.ReadFile(filepath.Join(workspace, ".batz/reports", string(report.Identity.Run)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted contracts.RunReport
	if err = json.Unmarshal(saved, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Exit != 6 || persisted.NormalizedRevision == nil || *persisted.NormalizedRevision != *report.NormalizedRevision {
		t.Fatal("durable run lost strict partial revision")
	}
}

package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"batz/collector/plugins"
	"batz/internal/contracts"
	"github.com/parquet-go/parquet-go"
)

// Commit tests need real closed files, not a native collector/model launch.
func parsedReceipt(t *testing.T, root, batch string) verifiedReceipt {
	t.Helper()
	stage := filepath.Join(root, "staging", batch)
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	source := []byte(`{"Records":[{"eventID":"one"},null]}`)
	request := plugins.Request{
		Framing:  "records-json",
		Identity: contracts.Identity{Binding: "demo", Run: contracts.RunID("run-" + batch), Batch: contracts.BatchID(batch), Attempt: contracts.AttemptID(batch)},
		Input:    contracts.InputRef{ID: hash([]byte(batch)), Locator: "synthetic://state", SHA256: hash(source), Bytes: int64(len(source))},
		Stage:    stage, Source: filepath.Join(stage, "input.json"), Limits: Limits(), AcquiredAt: "2026-10-29T00:00:00Z",
	}
	if err := durableWrite(request.Source, source); err != nil {
		t.Fatal(err)
	}
	row := plugins.Record{BindingID: "demo", RunID: string(request.Identity.Run), BatchID: batch, AttemptID: batch, InputID: request.Input.ID, InputLocator: request.Input.Locator, InputSHA256: request.Input.SHA256, AcquiredAt: request.AcquiredAt, Representation: "reconstructed-json", Payload: `{"eventID":"one"}`, ParseStatus: "accepted"}
	row.ObservationID = hash(encode([]any{request.Identity.Binding, request.Identity.Batch, request.Input.ID, row.Ordinal}))
	if err := parquet.WriteFile(filepath.Join(stage, "parsed.parquet"), []plugins.Record{row}); err != nil {
		t.Fatal(err)
	}
	row.Ordinal, row.Payload, row.ParseStatus, row.RejectReason = 1, "null", "rejected", "expected-object"
	row.ObservationID = hash(encode([]any{request.Identity.Binding, request.Identity.Batch, request.Input.ID, row.Ordinal}))
	if err := parquet.WriteFile(filepath.Join(stage, "rejects.parquet"), []plugins.Record{row}); err != nil {
		t.Fatal(err)
	}
	files := make([]contracts.FileRef, 0, 3)
	for _, item := range []struct {
		name string
		rows int64
	}{{name: "parsed.parquet", rows: 1}, {name: "rejects.parquet", rows: 1}, {name: "input.json"}} {
		ref, err := fileRef(stage, filepath.Join(stage, item.name), item.rows)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, ref)
	}
	receipt := contracts.CollectionReceipt{APIVersion: contracts.APIVersion, Kind: "CollectionReceipt", Identity: request.Identity, Inputs: []contracts.InputRef{request.Input}, ParsedFiles: files[:1], RejectFiles: files[1:2], SourceFiles: files[2:], Counts: contracts.Counts{Boundary: "parse", Accepted: 1, Rejected: 1}, Complete: true}
	if err := writeJSON(filepath.Join(stage, "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(stage, "context.json"), request); err != nil {
		t.Fatal(err)
	}
	verified, err := verifyReceipt(stage, request)
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

func stateFixture(t *testing.T) (string, *sql.DB) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(root, ".batz")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := openState(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("INSERT INTO bindings(id,selection) VALUES ('demo','selection')"); err != nil {
		t.Fatal(err)
	}
	return root, db
}

func TestDuplicateParsedDeliveryReturnsOneCommittedRevision(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	first, err := commitParsed(db, root, verified, sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := commitParsed(db, root, verified, sql.NullString{})
	if err != nil {
		t.Fatalf("duplicate receipt should resolve its committed revision: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM parsed").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || first.revision != again.revision || first.digest != again.digest {
		t.Fatalf("duplicate publication: count=%d first=%+v again=%+v", count, first, again)
	}
}

func TestParsedCommitRejectsMissingEvidenceAndStaleProgress(t *testing.T) {
	root, db := stateFixture(t)
	first, err := commitParsed(db, root, parsedReceipt(t, root, "one"), sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commitParsed(db, root, parsedReceipt(t, root, "stale"), sql.NullString{}); err == nil {
		t.Fatal("stale predecessor advanced progress")
	}
	missing := parsedReceipt(t, root, "missing")
	if err := os.Remove(filepath.Join(missing.stage, "rejects.parquet")); err != nil {
		t.Fatal(err)
	}
	if _, err := commitParsed(db, root, missing, sql.NullString{String: string(first.revision), Valid: true}); err == nil {
		t.Fatal("missing reject evidence advanced progress")
	}
	var revision string
	var count int
	if err := db.QueryRow("SELECT parsed FROM bindings WHERE id='demo'").Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM parsed").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if revision != string(first.revision) || count != 1 {
		t.Fatalf("failed commit changed state: %s count=%d", revision, count)
	}
	if err := verifyInventory(root, first); err != nil {
		t.Fatalf("referenced files lost: %v", err)
	}
}

func TestInventoryReloadPinsSelectedRevision(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	first, err := commitParsed(db, root, verified, sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := commitParsed(db, root, parsedReceipt(t, root, "two"), sql.NullString{String: string(first.revision), Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadInventory(db, "demo", first.revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.members) != 2 || len(reloaded.members) != 1 || reloaded.digest != first.digest {
		t.Fatalf("inventory acquired future membership: %+v", reloaded)
	}
	verified.receipt.Counts.Accepted++
	if _, err := commitParsed(db, root, verified, sql.NullString{}); err == nil {
		t.Fatal("conflicting evidence reused a committed batch")
	}
}

func TestUnknownStateVersionIsRefusedWithoutMutation(t *testing.T) {
	root, db := stateFixture(t)
	if _, err := db.Exec("PRAGMA user_version=999; PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "state.sqlite")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := openState(root); err == nil {
		state.Close()
		t.Fatal("unknown state version accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("future database changed during refusal")
	}
}

func TestParsedCommitIncludesRunStatus(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	report := contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}
	if err := saveRun(db, root, Request{Binding: "demo"}, report); err != nil {
		t.Fatal(err)
	}
	inventory, err := commitParsed(db, root, verified, sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := db.QueryRow("SELECT report FROM runs WHERE id=?", report.Identity.Run).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Outcome != "parsed-committed" || !report.AcquisitionComplete || report.ParsedRevision == nil || *report.ParsedRevision != inventory.revision || len(report.Counts) != 1 || report.Counts[0].Rejected != 1 {
		t.Fatalf("parsed inventory committed without corresponding run status: %s", raw)
	}
}

func TestCompletedBatchRecoversWithoutSourceAccess(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	contextBytes, err := os.ReadFile(filepath.Join(verified.stage, "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	receiptBytes, err := os.ReadFile(filepath.Join(verified.stage, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(verified.stage, "completed.json"), map[string]string{"context_sha256": hash(contextBytes), "receipt_sha256": hash(receiptBytes)}); err != nil {
		t.Fatal(err)
	}
	req := Request{Workspace: filepath.Dir(root), Binding: "demo", Definition: "definition", Route: "local", Source: "synthetic://state", Project: filepath.Join(root, "missing-model-project")}
	if _, err := db.Exec("UPDATE bindings SET selection=? WHERE id='demo'", string(encode([]string{req.Definition, req.Route, req.Source}))); err != nil {
		t.Fatal(err)
	}
	if err := saveRun(db, root, req, contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}); err != nil {
		t.Fatal(err)
	}
	req.Retry = true
	report, _ := Execute(context.Background(), req)
	if report.ParsedRevision == nil || !report.AcquisitionComplete {
		t.Fatalf("completed evidence was not recovered: %+v", report)
	}
	if _, collected := report.Resources["collection"]; collected {
		t.Fatal("recovery reacquired the source")
	}
}

func TestRecoveryCannotSubstituteAnInputOutsideTheFrozenSelection(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	if err := markCollectionComplete(verified.stage); err != nil {
		t.Fatal(err)
	}
	input := verified.receipt.Inputs[0]
	input.SHA256 = hash([]byte("different frozen evidence"))
	req := Request{Workspace: filepath.Dir(root), Binding: "demo", Definition: "definition", Route: "local", Source: input.Locator, Selected: []contracts.InputRef{input}, Project: filepath.Join(root, "missing-model-project")}
	if _, err := db.Exec("UPDATE bindings SET selection=? WHERE id='demo'", string(encode([]string{req.Definition, req.Route, req.Source}))); err != nil {
		t.Fatal(err)
	}
	if err := saveRun(db, root, req, contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}); err != nil {
		t.Fatal(err)
	}
	req.Retry = true
	report, err := Execute(context.Background(), req)
	if err == nil || report.AcquisitionComplete || report.ParsedRevision != nil || report.Exit != 4 {
		t.Fatalf("recovery substituted unselected content: %+v %v", report, err)
	}
}

func TestNormalizedCommitIncludesRunStatus(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	report := contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}
	if err := saveRun(db, root, Request{Binding: "demo"}, report); err != nil {
		t.Fatal(err)
	}
	input, err := commitParsed(db, root, verified, sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	applyInventory(&report, input)
	report.Outcome = "published"
	published := contracts.PublishedInventory{Revision: "normalized", Identity: report.Identity, InputRevision: input.revision}
	report.NormalizedRevision = &published.Revision
	if err := commitNormalized(db, published, report, sql.NullString{}); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := db.QueryRow("SELECT report FROM runs WHERE id=?", report.Identity.Run).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Outcome != "published" || report.NormalizedRevision == nil || *report.NormalizedRevision != published.Revision {
		t.Fatalf("normalized pointer committed without corresponding run status: %s", raw)
	}
}

func TestRetryCannotSubstituteNewerParsedMembership(t *testing.T) {
	root, db := stateFixture(t)
	req := Request{Workspace: filepath.Dir(root), Binding: "demo", Definition: "definition", Route: "local", Source: "synthetic://state", Project: filepath.Join(root, "unused-models")}
	if _, err := db.Exec("UPDATE bindings SET selection=? WHERE id='demo'", string(encode([]string{req.Definition, req.Route, req.Source}))); err != nil {
		t.Fatal(err)
	}
	first := parsedReceipt(t, root, "one")
	if err := saveRun(db, root, req, contracts.RunReport{Identity: first.receipt.Identity, Outcome: "running"}); err != nil {
		t.Fatal(err)
	}
	input, err := commitParsed(db, root, first, sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commitParsed(db, root, parsedReceipt(t, root, "two"), sql.NullString{String: string(input.revision), Valid: true}); err != nil {
		t.Fatal(err)
	}
	req.Retry, req.RetryRun = true, first.receipt.Identity.Run
	report, err := Execute(context.Background(), req)
	if err == nil || report.ParsedRevision != nil || report.Exit != 2 {
		t.Fatalf("retry silently selected newer membership: %+v %v", report, err)
	}
}

func TestStatusReadsCommittedFailureWhileOwnerIsBusy(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	report := contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}
	if err := saveRun(db, root, Request{Binding: "demo"}, report); err != nil {
		t.Fatal(err)
	}
	input, err := commitParsed(db, root, verified, sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	applyInventory(&report, input)
	report.Outcome, report.Exit = "normalization-failed", 4
	if err := saveRun(db, root, Request{Binding: "demo"}, report); err != nil {
		t.Fatal(err)
	}
	lease, err := os.OpenFile(filepath.Join(root, "owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStatus(filepath.Dir(root), string(report.Identity.Run))
	if err != nil {
		t.Fatal(err)
	}
	if status.Run.Exit != 4 || status.Run.Outcome != report.Outcome || !status.RetryEligible || len(status.ParsedFiles) != 1 {
		t.Fatalf("read-only status lost committed failure: %+v", status)
	}
	missing := filepath.Join(t.TempDir(), "missing-workspace")
	if _, err := ReadStatus(missing, ""); err == nil {
		t.Fatal("missing workspace reported success")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("status created a workspace")
	}
}

func TestSliceTwoStateUpgradesWithoutLosingEvidence(t *testing.T) {
	root, db := stateFixture(t)
	input, err := commitParsed(db, root, parsedReceipt(t, root, "one"), sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE runs; DROP INDEX parsed_batch; PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := openState(root)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version int
	if err := upgraded.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadInventory(upgraded, "demo", input.revision)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 || reloaded.digest != input.digest {
		t.Fatal("coordinator migration changed committed evidence")
	}
	if err := verifyInventory(root, reloaded); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRecordsInvalidCompletedEvidenceAsFailure(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	req := Request{Workspace: filepath.Dir(root), Binding: "demo", Definition: "definition", Route: "local", Source: "synthetic://state"}
	if _, err := db.Exec("UPDATE bindings SET selection=? WHERE id='demo'", string(encode([]string{req.Definition, req.Route, req.Source}))); err != nil {
		t.Fatal(err)
	}
	if err := saveRun(db, root, req, contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := markCollectionComplete(verified.stage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(verified.stage, "receipt.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	req.Retry = true
	if _, err := Execute(context.Background(), req); err == nil {
		t.Fatal("changed completed evidence was accepted")
	}
	status, err := ReadStatus(req.Workspace, string(verified.receipt.Identity.Run))
	if err != nil {
		t.Fatal(err)
	}
	if status.Run.Outcome != "acquisition-failed" || status.Run.AcquisitionComplete || len(status.Run.Diagnostics) != 1 || status.Run.Diagnostics[0].Category != "integrity" {
		t.Fatalf("recovery failure was left running: %+v", status)
	}
}

func TestPinnedRetryCannotCommitAnotherPendingBatch(t *testing.T) {
	for _, pin := range []contracts.RunID{"missing", "run-one"} {
		t.Run(string(pin), func(t *testing.T) {
			root, db := stateFixture(t)
			req := Request{Workspace: filepath.Dir(root), Binding: "demo", Definition: "definition", Route: "local", Source: "synthetic://state", Project: filepath.Join(root, "unused-models")}
			if _, err := db.Exec("UPDATE bindings SET selection=? WHERE id='demo'", string(encode([]string{req.Definition, req.Route, req.Source}))); err != nil {
				t.Fatal(err)
			}
			first := parsedReceipt(t, root, "one")
			if err := saveRun(db, root, req, contracts.RunReport{Identity: first.receipt.Identity, Outcome: "running"}); err != nil {
				t.Fatal(err)
			}
			input, err := commitParsed(db, root, first, sql.NullString{})
			if err != nil {
				t.Fatal(err)
			}
			pending := parsedReceipt(t, root, "two")
			if err := markCollectionComplete(pending.stage); err != nil {
				t.Fatal(err)
			}
			if err := saveRun(db, root, req, contracts.RunReport{Identity: pending.receipt.Identity, Outcome: "running"}); err != nil {
				t.Fatal(err)
			}
			req.Retry, req.RetryRun = true, pin
			_, _ = Execute(context.Background(), req)
			var revision string
			var count int
			if err := db.QueryRow("SELECT parsed FROM bindings WHERE id='demo'").Scan(&revision); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT count(*) FROM parsed").Scan(&count); err != nil {
				t.Fatal(err)
			}
			status, err := ReadStatus(req.Workspace, string(pending.receipt.Identity.Run))
			if err != nil {
				t.Fatal(err)
			}
			if revision != string(input.revision) || count != 1 || status.Run.Outcome != "running" || status.Run.AcquisitionComplete {
				t.Fatalf("pinned retry changed pending B: revision=%s count=%d status=%+v", revision, count, status)
			}
		})
	}
}

func TestReportExportFailureKeepsCommandAndStatusConsistent(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		cause         error
	}{
		{name: "published", outcome: "published"},
		{name: "model-failed", outcome: "normalization-failed", cause: failure(4, "assertion", errors.New("required check"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, db := stateFixture(t)
			verified := parsedReceipt(t, root, "one")
			req := Request{Binding: "demo", Workspace: filepath.Dir(root)}
			report := contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}
			if err := saveRun(db, root, req, report); err != nil {
				t.Fatal(err)
			}
			input, err := commitParsed(db, root, verified, sql.NullString{})
			if err != nil {
				t.Fatal(err)
			}
			applyInventory(&report, input)
			published := contracts.PublishedInventory{Revision: "published", Identity: report.Identity, InputRevision: input.revision, Quality: "complete", Counts: report.Counts}
			report = applyPublication(root, published, report)
			if err := commitNormalized(db, published, report, sql.NullString{}); err != nil {
				t.Fatal(err)
			}
			report.Outcome = tc.outcome
			cause := finishFailure(context.Background(), &report, tc.cause)
			if err := os.Rename(filepath.Join(root, "reports"), filepath.Join(root, "saved-reports")); err != nil {
				t.Fatal(err)
			}
			if err := durableWrite(filepath.Join(root, "reports"), []byte("blocked export directory")); err != nil {
				t.Fatal(err)
			}
			if err := persistOutcome(db, root, req, &report, cause); err == nil {
				t.Fatal("report export unexpectedly succeeded")
			}
			status, err := ReadStatus(req.Workspace, string(report.Identity.Run))
			if err != nil {
				t.Fatal(err)
			}
			if report.Exit != 1 || status.Run.Exit != report.Exit || len(status.Run.Diagnostics) == 0 || status.Run.Diagnostics[len(status.Run.Diagnostics)-1].Code != "report-export" || status.CurrentNormalized == nil || *status.CurrentNormalized != published.Revision {
				t.Fatalf("export failure disagrees with durable status or lost publication: report=%+v status=%+v", report, status)
			}
		})
	}
}

func TestRecoveryRecordsMismatchedCompletedIdentityAsFailure(t *testing.T) {
	root, db := stateFixture(t)
	verified := parsedReceipt(t, root, "one")
	req := Request{Workspace: filepath.Dir(root), Binding: "demo", Definition: "definition", Route: "local", Source: "synthetic://state"}
	if _, err := db.Exec("UPDATE bindings SET selection=? WHERE id='demo'", string(encode([]string{req.Definition, req.Route, req.Source}))); err != nil {
		t.Fatal(err)
	}
	if err := saveRun(db, root, req, contracts.RunReport{Identity: verified.receipt.Identity, Outcome: "running"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(verified.stage, "context.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var contextRequest plugins.Request
	if err := json.Unmarshal(data, &contextRequest); err != nil {
		t.Fatal(err)
	}
	contextRequest.Identity.Attempt = "another-attempt"
	if err := os.WriteFile(path, encode(contextRequest), 0600); err != nil {
		t.Fatal(err)
	}
	if err := markCollectionComplete(verified.stage); err != nil {
		t.Fatal(err)
	}
	req.Retry = true
	if _, err := Execute(context.Background(), req); err == nil {
		t.Fatal("mismatched completed identity was accepted")
	}
	status, err := ReadStatus(req.Workspace, string(verified.receipt.Identity.Run))
	if err != nil {
		t.Fatal(err)
	}
	if status.Run.Outcome != "acquisition-failed" || status.Run.AcquisitionComplete || len(status.Run.Diagnostics) != 1 || status.Run.Diagnostics[0].Code != "receipt-integrity" {
		t.Fatalf("original attempt remained running after identity rejection: %+v", status)
	}
}

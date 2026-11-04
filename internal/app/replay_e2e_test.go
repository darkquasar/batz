//go:build batz_aws_cloudtrail

package app_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestE2EGrowingDirectorySkipsChangesAndReplaysWithoutLosingHistory(t *testing.T) {
	root, workspace, directory := previewContent(t), t.TempDir(), t.TempDir()
	write := func(name, eventID, instant, marker string) {
		t.Helper()
		r := sourceRecords(t)
		r["recipientAccountId"], r["eventID"], r["eventTime"], r["unmapped_example"] = "111122223333", eventID, instant, marker
		records := []map[string]any{r}
		if name == "A" {
			anonymous, malformed, unscoped := sourceRecords(t), sourceRecords(t), sourceRecords(t)
			delete(anonymous, "eventID")
			delete(anonymous, "eventTime")
			malformed["eventID"] = 42
			unscoped["recipientAccountId"] = map[string]any{"unreliable": true}
			records = append(records, anonymous, anonymous, malformed, unscoped)
		}
		writeRecords(t, filepath.Join(directory, name+".json"), records)
	}
	write("A", "event-a", "2026-10-25T12:00:00.000000001Z", "A")
	write("B", "event-b", "2026-10-25T12:00:00.000000002Z", "B")
	first := runFixture(t, root, workspace, directory)
	assertAcquisition(t, first, 2, 0, 0, 0)
	if first.Quality == nil || *first.Quality != "complete" {
		t.Fatal("identity must not reject otherwise supported records")
	}
	for _, table := range []string{"source_staging", "api_activity"} {
		got := string(query(t, root, *first.Catalog, `SELECT count(*),count(DISTINCT batz.source_event_id),count(batz.logical_event_id),count(batz.norm_event_variant_id) FROM demo.`+table))
		if got != "[[6,6,2,6]]" {
			t.Fatalf("%s lost optional identity records: %s", table, got)
		}
	}
	write("C", "event-c", "2020-01-01T00:00:00.123+02:00", "C")
	second := runFixture(t, root, workspace, directory)
	assertAcquisition(t, second, 1, 0, 2, 0)
	if got := string(query(t, root, *second.Catalog, `SELECT count(*),min(year("@timestamp")) FROM demo.api_activity`)); got != "[[7,2019]]" {
		t.Fatal(got)
	}
	write("B", "event-b", "2026-10-25T12:00:00.000000002Z", "changed-B")
	third := runFixture(t, root, workspace, directory)
	assertAcquisition(t, third, 0, 1, 2, 0)
	replay := runFixture(t, root, workspace, directory, "--replay")
	assertAcquisition(t, replay, 0, 0, 0, 3)
	if replay.Reconciliation == nil || replay.Reconciliation.Observations != 15 || replay.Reconciliation.Variants != 12 || replay.Reconciliation.LogicalEvents != 3 || replay.Reconciliation.ConflictingEvents != 1 || replay.Reconciliation.UnidentifiedObservations != 8 {
		t.Fatalf("missing reconciliation accounting: %+v", replay.Reconciliation)
	}
	if got := string(query(t, root, *replay.Catalog, `SELECT count(*),count(DISTINCT batz.logical_event_id),sum(len(batz.origins)),count(*) FILTER(WHERE batz.conflict) FROM demo.api_activity`)); got != "[[12,3,15,2]]" {
		t.Fatal(got)
	}
	if got := string(query(t, root, *replay.Catalog, `SELECT event.id,epoch_ns("@timestamp"),batz.time.original,batz.time.fraction_digits,batz.time.offset FROM demo.api_activity WHERE event.id='event-c'`)); got != `[["event-c",1577829600123000000,"2020-01-01T00:00:00.123+02:00",3,"+02:00"]]` {
		t.Fatal("replay lost historical precision/offset: " + got)
	}
	if got := string(query(t, root, *replay.Catalog, `SELECT count(DISTINCT epoch_ns("@timestamp")) FILTER(WHERE event.id IN ('event-a','event-b')),min(epoch_ns("@timestamp")) FILTER(WHERE event.id IN ('event-a','event-b')),max(epoch_ns("@timestamp")) FILTER(WHERE event.id IN ('event-a','event-b')),count(*) FILTER(WHERE event.id IS NULL AND "@timestamp" IS NULL) FROM demo.api_activity`)); got != "[[2,1792929600000000001,1792929600000000002,4]]" {
		t.Fatal("replay lost nano-distinct or missing occurrence time: " + got)
	}
	if got := string(query(t, root, *replay.Catalog, `SELECT count(DISTINCT origin.source_event_id) FROM demo.api_activity,unnest(batz.origins) u(origin)`)); got != "[[15]]" {
		t.Fatal("source origins lost: " + got)
	}
	if got := string(query(t, root, *first.Catalog, `SELECT count(*),min(epoch_ns("@timestamp")),max(epoch_ns("@timestamp")) FROM demo.api_activity WHERE event.id IN ('event-a','event-b')`)); got != "[[2,1792929600000000001,1792929600000000002]]" {
		t.Fatal("old revision changed: " + got)
	}
	unchanged := runFixture(t, root, workspace, directory)
	assertAcquisition(t, unchanged, 0, 0, 3, 0)
	if unchanged.NormalizedRevision == nil || *unchanged.NormalizedRevision != *replay.NormalizedRevision {
		t.Fatal("unchanged membership should reuse publication")
	}
	if _, ran := unchanged.Resources["collection"]; ran {
		t.Fatal("unchanged files started collection")
	}
	if _, ran := unchanged.Resources["model"]; ran {
		t.Fatal("unchanged membership started models")
	}
}

func assertAcquisition(t *testing.T, report any, added, changed, skipped, replayed int) {
	t.Helper()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Acquisition map[string][]any `json:"acquisition"`
	}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int{"new": added, "changed": changed, "skipped": skipped, "replayed": replayed} {
		if decoded.Acquisition == nil || len(decoded.Acquisition[key]) != want {
			t.Fatalf("%s count want %d: %s", key, want, raw)
		}
	}
}

//go:build batz_app

package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppGrowingDirectorySkipsChangesAndReplaysWithoutLosingHistory(t *testing.T) {
	root, workspace, directory := nativeContent(t), t.TempDir(), t.TempDir()
	// A real empty JSONL file still yields typed empty Parquet and publication.
	emptyPath := filepath.Join(directory, "empty.jsonl")
	writeEvents(t, emptyPath, nil)
	empty := runSharedFixture(t, root, workspace, directory)
	assertAcquisition(t, empty, 1, 0, 0, 0)
	if empty.Outcome != "published" || empty.Catalog == nil || empty.ParsedRevision == nil || empty.NormalizedRevision == nil {
		t.Fatalf("incomplete empty publication: %+v", empty)
	}
	if got := string(query(t, root, *empty.Catalog, `SELECT count(*) FROM demo.findings`)); got != "[[0]]" {
		t.Fatal(got)
	}
	if err := os.Remove(emptyPath); err != nil {
		t.Fatal(err)
	}
	a := []map[string]any{
		{"id": "nano-1", "observed_at": "2026-10-25T12:00:00.000000001"},
		{"id": "coarse", "observed_at": "2026-10-25T12:00:00.123"},
		// Normalized time is supplied by this invented source; no vendor parser.
		{"id": "offset", "observed_at": "2026-10-25T12:00:00.000000001", "original_time": "2026-10-25T22:00:00.000000001+10:00"},
		{"id": "undated"},
	}
	writeEvents(t, filepath.Join(directory, "A.jsonl"), a)
	writeB := func(marker string) {
		writeEvents(t, filepath.Join(directory, "B.jsonl"), []map[string]any{{"id": "nano-2", "observed_at": "2026-10-25T12:00:00.000000002", "marker": marker}})
	}
	writeB("B")
	first := runSharedFixture(t, root, workspace, directory)
	assertAcquisition(t, first, 2, 0, 0, 0)
	if first.Quality == nil || *first.Quality != "complete" {
		t.Fatal("optional identity must not block a valid pipeline")
	}
	writeEvents(t, filepath.Join(directory, "C.jsonl"), []map[string]any{{"id": "historical", "observed_at": "2019-12-31T22:00:00.123"}})
	second := runSharedFixture(t, root, workspace, directory)
	assertAcquisition(t, second, 1, 0, 2, 0)
	if got := string(query(t, root, *second.Catalog, `SELECT count(*),min(year("@timestamp")) FROM demo.findings`)); got != "[[6,2019]]" {
		t.Fatal(got)
	}
	writeB("changed-B")
	third := runSharedFixture(t, root, workspace, directory)
	assertAcquisition(t, third, 0, 1, 2, 0)
	replay := runSharedFixture(t, root, workspace, directory, "--replay")
	assertAcquisition(t, replay, 0, 0, 0, 3)
	if replay.Reconciliation == nil || replay.Reconciliation.Observations != 13 || replay.Reconciliation.Variants != 13 || replay.Reconciliation.LogicalEvents != 0 || replay.Reconciliation.ConflictingEvents != 0 || replay.Reconciliation.UnidentifiedObservations != 13 {
		t.Fatalf("generic reconciliation: %+v", replay.Reconciliation)
	}
	sql := `SELECT count(*),count(DISTINCT origin.source_event_id),count(batz.logical_event_id),
        count(DISTINCT epoch_ns("@timestamp")) FILTER(WHERE event.id IN ('nano-1','nano-2')),
        min(epoch_ns("@timestamp")) FILTER(WHERE event.id IN ('nano-1','nano-2')),
        max(epoch_ns("@timestamp")) FILTER(WHERE event.id IN ('nano-1','nano-2')),
        list(DISTINCT typeof("@timestamp")),
        list(DISTINCT epoch_ns("@timestamp")) FILTER(WHERE event.id='coarse'),
        list(DISTINCT epoch_ns("@timestamp")) FILTER(WHERE event.id='historical'),
        count(*) FILTER(WHERE event.id='undated' AND "@timestamp" IS NULL),
        list(DISTINCT json_extract_string(batz.source_payload,'$.original_time')) FILTER(WHERE event.id='offset'),
        count(*) FILTER(WHERE json_extract_string(batz.source_payload,'$.marker')='changed-B')
        FROM demo.findings,unnest(batz.origins) u(origin)`
	if got := string(query(t, root, *replay.Catalog, sql)); got != `[[13,13,0,2,1792929600000000001,1792929600000000002,["TIMESTAMP_NS"],[1792929600123000000],[1577829600123000000],2,["2026-10-25T22:00:00.000000001+10:00"],2]]` {
		t.Fatal("replay lost native precision, payload or origins: " + got)
	}
	if got := string(query(t, root, *first.Catalog, `SELECT event.id,epoch_ns("@timestamp") FROM demo.findings ORDER BY event.id`)); got != `[["coarse",1792929600123000000],["nano-1",1792929600000000001],["nano-2",1792929600000000002],["offset",1792929600000000001],["undated",null]]` {
		t.Fatal("old revision changed: " + got)
	}
	unchanged := runSharedFixture(t, root, workspace, directory)
	assertAcquisition(t, unchanged, 0, 0, 3, 0)
	if unchanged.NormalizedRevision == nil || *unchanged.NormalizedRevision != *replay.NormalizedRevision {
		t.Fatal("unchanged membership should reuse publication")
	}
	for _, stage := range []string{"collection", "model"} {
		if _, ran := unchanged.Resources[stage]; ran {
			t.Fatalf("unchanged membership started %s", stage)
		}
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

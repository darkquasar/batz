package run

import (
	"context"
	"errors"
	"testing"

	"batz/internal/contracts"
)

func TestPublicationQualityAndStrictExit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		counts  []contracts.Counts
		strict  bool
		quality string
		exit    int
	}{
		{name: "empty", quality: "complete"},
		{name: "complete", counts: []contracts.Counts{{Accepted: 1}}, strict: true, quality: "complete"},
		{name: "unsupported-default", counts: []contracts.Counts{{Accepted: 80, Unsupported: 20}}, quality: "complete"},
		{name: "unsupported-strict", counts: []contracts.Counts{{Accepted: 80, Unsupported: 20}}, strict: true, quality: "complete"},
		{name: "parse-partial", counts: []contracts.Counts{{Boundary: "parse", Accepted: 1, Rejected: 1}, {Boundary: "normalize", Accepted: 1}}, quality: "partial"},
		{name: "mapping-partial", counts: []contracts.Counts{{Boundary: "normalize", Accepted: 995, Rejected: 5}}, quality: "partial"},
		{name: "strict-partial", counts: []contracts.Counts{{Rejected: 1, Unsupported: 1}}, strict: true, quality: "partial", exit: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quality, exit := publicationPolicy(tc.counts, tc.strict)
			if quality != tc.quality || exit != tc.exit {
				t.Fatalf("got %s/%d, want %s/%d", quality, exit, tc.quality, tc.exit)
			}
		})
	}
}

func TestFailedPreparationDoesNotRemainRunning(t *testing.T) {
	report := contracts.RunReport{Outcome: "running"}
	err := finishFailure(context.Background(), &report, failure(2, "invalid-input", errors.New("no committed inventory")))
	if err == nil || report.Outcome != "blocked" || report.Exit != 2 {
		t.Fatalf("failed preparation remained running: %+v %v", report, err)
	}
}

func TestFailedModelsPreserveParsedAndPriorPublishedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, category string
		exit           int
	}{
		{name: "capability", category: "incomplete-capability", exit: 3},
		{name: "required-check", category: "assertion", exit: 4},
		{name: "operational", category: "execution", exit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, normalized := contracts.RevisionID("B"), contracts.RevisionID("A")
			report := contracts.RunReport{Outcome: "normalization-failed", AcquisitionComplete: true, ParsedRevision: &parsed, NormalizedRevision: &normalized}
			err := finishFailure(context.Background(), &report, &Failure{Code: tc.exit, Category: tc.category, Message: "invented model failure"})
			if err == nil || report.Exit != tc.exit || report.Quality != nil || report.Outcome != "normalization-failed" || *report.ParsedRevision != "B" || *report.NormalizedRevision != "A" || !report.AcquisitionComplete || report.Diagnostics[0].Category != tc.category {
				t.Fatalf("failure changed evidence or classification: %+v %v", report, err)
			}
		})
	}
}

func TestMissingSourceFailsBeforeAnyNativeChild(t *testing.T) {
	report, err := Execute(context.Background(), Request{Workspace: t.TempDir(), Binding: "demo", Source: "/nonexistent-batz-source"})
	if err == nil || report.Outcome != "acquisition-failed" || report.AcquisitionComplete || report.ParsedRevision != nil || report.NormalizedRevision != nil || len(report.Resources) != 0 {
		t.Fatalf("missing source started native work or committed evidence: %+v %v", report, err)
	}
}

func TestReceiptIntegrityFailureIsDistinctFromModelFailure(t *testing.T) {
	report := contracts.RunReport{Outcome: "acquisition-failed"}
	err := finishFailure(context.Background(), &report, failure(4, "integrity", errors.New("invented invalid receipt")))
	if err == nil || report.Exit != 4 || report.AcquisitionComplete || report.ParsedRevision != nil || report.Diagnostics[0].Category != "integrity" || report.Diagnostics[0].Code != "receipt-integrity" {
		t.Fatalf("receipt failure lost classification: %+v %v", report, err)
	}
}

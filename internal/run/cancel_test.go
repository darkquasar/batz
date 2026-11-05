package run

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCancelledRunReportsCancellation(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := Execute(ctx, Request{Contract: fixtureContract(t, root), Root: root, Workspace: t.TempDir(), Definition: filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), Route: "local_raw", Source: filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl"), Binding: "demo", NativeConfig: filepath.Join(root, "internal/app/testdata/synthetic/collector.yaml"), Project: filepath.Join(root, ".")})
	if err == nil || report.Exit != 130 || report.Outcome != "cancelled" || len(report.Diagnostics) != 1 || report.Diagnostics[0].Category != "cancelled" || report.Diagnostics[0].Code != "cancelled" {
		t.Fatalf("cancellation misclassified: %+v %v", report, err)
	}
}

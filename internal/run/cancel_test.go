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
	report, err := Execute(ctx, Request{Contract: fixtureContract(t, root), Root: root, Workspace: t.TempDir(), Definition: filepath.Join(root, "pipelines/aws-cloudtrail/pipeline.yaml"), Route: "local_raw", Source: filepath.Join(root, "pipelines/aws-cloudtrail/fixtures/api-read.json"), Binding: "demo", NativeConfig: filepath.Join(root, "pipelines/aws-cloudtrail/input/local-records.yaml"), Project: filepath.Join(root, ".")})
	if err == nil || report.Exit != 130 || report.Outcome != "cancelled" || len(report.Diagnostics) != 1 || report.Diagnostics[0].Category != "cancelled" || report.Diagnostics[0].Code != "cancelled" {
		t.Fatalf("cancellation misclassified: %+v %v", report, err)
	}
}

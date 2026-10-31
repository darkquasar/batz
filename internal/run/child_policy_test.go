package run

import (
	"batz/internal/contracts"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// Real exit statuses exercise the production error boundary without pretending
// to produce collector receipts or model evidence.
func TestNativeExitClassification(t *testing.T) {
	for _, tt := range []struct {
		name, exit, category, diagnostic string
		code                             int
		classify                         func(error) error
	}{
		{"framing", "2", "execution", "unsupported-shape", 1, collectionFailure},
		{"capability", "3", "incomplete-capability", "timestamp-representation-unavailable", 3, modelFailure},
		{"assertion", "4", "assertion", "", 4, modelFailure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := exec.Command("/bin/sh", "-c", "exit "+tt.exit).Run()
			err = tt.classify(fmt.Errorf("supervised child: %w", err))
			var f *Failure
			if !errors.As(err, &f) || f.Code != tt.code || f.Category != tt.category || !strings.Contains(err.Error(), tt.diagnostic) {
				t.Fatalf("wrong classification: %v", err)
			}
			report := contracts.RunReport{Outcome: "acquisition-failed"}
			if tt.name != "framing" {
				report.Outcome = "normalization-failed"
				report.AcquisitionComplete = true
			}
			_ = finishFailure(context.Background(), &report, err)
			if report.Exit != tt.code || report.Diagnostics[0].Category != tt.category || report.ParsedRevision != nil || report.NormalizedRevision != nil {
				t.Fatalf("wrong public failure: %+v", report)
			}
			if tt.name == "framing" && (report.Diagnostics[0].Code != "unsupported-shape" || report.AcquisitionComplete) {
				t.Fatalf("shape failure acquired input: %+v", report)
			}

		})
	}
	err := exec.Command("/bin/sh", "-c", "exit 1").Run()
	if collectionFailure(err) != err || modelFailure(err) != err {
		t.Fatal("ordinary process failure was reclassified")
	}
}

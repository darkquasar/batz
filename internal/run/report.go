package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"batz/internal/contracts"
)

func finishFailure(ctx context.Context, report *contracts.RunReport, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		err = failure(130, "cancelled", ctx.Err())
		report.Outcome = "cancelled"
	}
	if report.Outcome == "running" {
		report.Outcome = "blocked"
	}
	report.Exit = 1
	category, code := "execution", report.Outcome
	var f *Failure
	if errors.As(err, &f) {
		report.Exit = f.Code
		category = f.Category
		switch category {
		case "busy":
			code = "workspace-busy"
			report.Outcome = "blocked"
		case "cancelled":
			code = "cancelled"
		case "assertion":
			code = "required-check"
		case "invalid-input":
			code = "invalid-field"
		case "incomplete-capability":
			code = "missing-dependency"
		case "integrity":
			code = "receipt-integrity"
			if report.AcquisitionComplete {
				code = "build-integrity"
			}
		}
		if f.DiagnosticCode != "" {
			code = f.DiagnosticCode
		}
	}
	if strings.HasPrefix(err.Error(), "execution: unsupported-shape:") {
		code = "unsupported-shape"
	}
	report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: category, Code: code, Message: err.Error()})
	return err
}

func applyInventory(report *contracts.RunReport, inventory committedInventory) {
	report.ParsedRevision = &inventory.revision
	report.AcquisitionComplete = true
	report.Outcome = "parsed-committed"
	counts := contracts.Counts{Boundary: "parse"}
	report.Inputs = []contracts.InputRef{}
	for _, receipt := range inventory.receipts {
		counts.Accepted += receipt.Counts.Accepted
		counts.Rejected += receipt.Counts.Rejected
		report.Inputs = append(report.Inputs, receipt.Inputs...)
	}
	report.Counts = []contracts.Counts{counts}
}

func publicationPolicy(counts []contracts.Counts, strict bool) (string, int) {
	for _, count := range counts {
		if count.Rejected > 0 {
			if strict {
				return "partial", 6
			}
			return "partial", 0
		}
	}
	return "complete", 0
}

func persistOutcome(db *sql.DB, root string, req Request, report *contracts.RunReport, cause error) error {
	if err := recordRun(db, req, *report); err != nil {
		report.Exit = 1
		return errors.Join(cause, fmt.Errorf("persist run outcome: %w", err))
	}
	if err := exportReport(root, *report); err != nil {
		exportErr := finishFailure(context.Background(), report, &Failure{Code: 1, Category: "execution", DiagnosticCode: "report-export", Message: err.Error()})
		// SQLite remains canonical when its derived JSON export cannot be written.
		// Do not retry the failed filesystem operation while recording its outcome.
		return errors.Join(cause, exportErr, recordRun(db, req, *report))
	}
	return cause
}

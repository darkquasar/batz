package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"batz/collector/plugins"
	"batz/internal/contracts"
)

func collect(ctx context.Context, req Request, limits contracts.Limits, artifact string, lease *os.File, report *contracts.RunReport, checkpoint func(string)) (verifiedReceipt, error) {
	report.Outcome = "acquisition-failed"
	stage := filepath.Join(artifact, "staging", string(report.Identity.Attempt))
	if e := os.MkdirAll(stage, 0700); e != nil {
		return verifiedReceipt{}, e
	}
	input, data, e := snapshotSource(req.Source, limits.InputBytes)
	if e != nil {
		return verifiedReceipt{}, failure(1, "execution", e)
	}
	if req.Expected != nil && input != *req.Expected {
		return verifiedReceipt{}, failure(1, "execution", errors.New("source changed between selection and opening"))
	}

	if e = durableWrite(filepath.Join(stage, "input.json"), data); e != nil {
		return verifiedReceipt{}, e
	}
	collectorRequest := plugins.Request{Framing: req.Contract.Framing, Identity: report.Identity, Input: input, Stage: stage, Source: filepath.Join(stage, "input.json"), AcquiredAt: time.Now().UTC().Format(time.RFC3339Nano), Limits: limits}
	if e = writeJSON(filepath.Join(stage, "context.json"), collectorRequest); e != nil {
		return verifiedReceipt{}, e
	}
	if e = os.Chmod(filepath.Join(stage, "context.json"), 0400); e != nil {
		return verifiedReceipt{}, e
	}
	environment := []string{
		"BATZ_STAGE_DIR=" + stage, "BATZ_SOURCE_PATH=" + collectorRequest.Source,
		"BATZ_RUN_ID=" + string(report.Identity.Run), "BATZ_BINDING_ID=" + string(report.Identity.Binding),
		"BATZ_BATCH_ID=" + string(report.Identity.Batch), "BATZ_ATTEMPT_ID=" + string(report.Identity.Attempt),
	}
	if report.Identity.Case != nil {
		environment = append(environment, "BATZ_CASE_ID="+*report.Identity.Case)
	}
	checkpointAt(checkpoint, "before-receipt")
	collectCtx, cancel := context.WithTimeout(ctx, time.Duration(limits.CollectionSeconds)*time.Second)
	collectionUsage, e := sandboxedChild(req.PrivateScratch, req.SandboxProfile, collectCtx, lease, stage, filepath.Join(stage, "collector.log"), childBudget{limits.ScratchBytes, []string{stage}}, environment, filepath.Join(req.Root, "runtime/.local/batz-collector"), req.NativeConfig, filepath.Join(stage, "context.json"))
	report.Resources["collection"] = combineUsage(report.Resources["collection"], collectionUsage)
	cancel()
	if e != nil {
		return verifiedReceipt{}, collectionFailure(e)
	}
	if e = markCollectionComplete(stage); e != nil {
		return verifiedReceipt{}, e
	}
	checkpointAt(checkpoint, "after-receipt")
	verified, e := verifyReceipt(stage, collectorRequest)
	if e != nil {
		return verifiedReceipt{}, failure(4, "integrity", e)
	}
	return verified, nil
}

func collectionFailure(err error) error {
	var childExit *exec.ExitError
	if errors.As(err, &childExit) && childExit.ExitCode() == 2 {
		return failure(1, "execution", fmt.Errorf("unsupported-shape: declared source framing failed (%w)", err))
	}
	return err
}

// Each file commits through the existing durable receipt boundary. A later
// acquisition failure leaves those observations retryable and publishes no model.
func acquireSelection(ctx context.Context, req *Request, db *sql.DB, artifact string, lease *os.File, report *contracts.RunReport, prior sql.NullString, checkpoint func(string)) (committedInventory, error) {
	report.Outcome = "acquisition-failed"
	selected := req.Selected
	var err error
	if selected == nil {
		selected, err = freezeSelection(req.Source, req.Contract.Framing, report.Limits)
		if err != nil {
			return committedInventory{}, err
		}
	}
	req.Selected = selected
	accounting, err := classifySelection(db, report.Identity.Binding, selected, req.Replay)
	if err != nil {
		return committedInventory{}, err
	}
	report.Acquisition = &accounting
	if err := saveRun(db, artifact, *req, *report); err != nil {
		return committedInventory{}, err
	}
	checkpointAt(checkpoint, "after-selection")
	skipped := map[string]bool{}
	for _, input := range accounting.Skipped {
		skipped[input.ID] = true
	}
	var inventory committedInventory
	if prior.Valid {
		inventory, err = loadInventory(db, report.Identity.Binding, contracts.RevisionID(prior.String))
		if err != nil {
			return inventory, err
		}
		if err := verifyInventory(artifact, inventory); err != nil {
			return inventory, err
		}
	}
	remainingRecords := report.Limits.Records
	for _, input := range selected {
		if skipped[input.ID] {
			continue
		}
		if len(accounting.Acquired) > 0 {
			report.Identity.Batch = contracts.BatchID(id())
			report.Identity.Attempt = contracts.AttemptID(id())
		}
		report.Outcome = "running"
		report.AcquisitionComplete = false
		if err := saveRun(db, artifact, *req, *report); err != nil {
			return inventory, err
		}
		single := *req
		single.Source = input.Locator
		single.Expected = &input
		collectionLimits := report.Limits
		collectionLimits.Records = remainingRecords
		verified, err := collect(ctx, single, collectionLimits, artifact, lease, report, checkpoint)
		if err != nil {
			return inventory, err
		}
		remainingRecords -= verified.receipt.Counts.Accepted + verified.receipt.Counts.Rejected
		if remainingRecords < 0 {
			return inventory, errors.New("selected record limit exceeded")
		}
		inventory, err = publishParsed(db, artifact, verified, prior, checkpoint)
		if err != nil {
			return inventory, err
		}
		accounting.Acquired = append(accounting.Acquired, input)
		report.Acquisition = &accounting
		prior = sql.NullString{String: string(inventory.revision), Valid: true}
		applyInventory(report, inventory)
		if err := saveRun(db, artifact, *req, *report); err != nil {
			return inventory, err
		}
	}
	if inventory.revision == "" {
		return inventory, errors.New("no selected JSON files or committed inventory")
	}
	return inventory, nil
}

func combineUsage(previous, next contracts.ResourceUsage) contracts.ResourceUsage {
	total := previous.Samples + next.Samples
	if total > 0 {
		var sum int64
		if previous.AverageRSSBytes != nil {
			sum += *previous.AverageRSSBytes * previous.Samples
		}
		if next.AverageRSSBytes != nil {
			sum += *next.AverageRSSBytes * next.Samples
		}
		average := sum / total
		next.AverageRSSBytes = &average
	}
	if previous.PeakRSSBytes != nil && (next.PeakRSSBytes == nil || *previous.PeakRSSBytes > *next.PeakRSSBytes) {
		next.PeakRSSBytes = previous.PeakRSSBytes
	}
	if previous.ScratchPeakBytes > next.ScratchPeakBytes {
		next.ScratchPeakBytes = previous.ScratchPeakBytes
	}
	if next.MeasurementError == nil {
		next.MeasurementError = previous.MeasurementError
	}
	next.Samples = total
	next.UnavailableSamples += previous.UnavailableSamples
	next.WallMS += previous.WallMS
	return next
}

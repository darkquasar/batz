package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"batz/collector/plugins"
	"batz/internal/contracts"
)

func collect(ctx context.Context, req Request, artifact string, lease *os.File, report *contracts.RunReport, checkpoint func(string)) (verifiedReceipt, error) {
	limits := report.Limits
	report.Outcome = "acquisition-failed"
	stage := filepath.Join(artifact, "staging", string(report.Identity.Attempt))
	if e := os.MkdirAll(stage, 0700); e != nil {
		return verifiedReceipt{}, e
	}
	before, e := os.Stat(req.Source)
	if e != nil {
		return verifiedReceipt{}, e
	}
	if !before.Mode().IsRegular() || before.Size() > limits.InputBytes {
		return verifiedReceipt{}, failure(2, "invalid-input", fmt.Errorf("source must be one regular file within byte budget"))
	}
	f, e := os.Open(req.Source)
	if e != nil {
		return verifiedReceipt{}, e
	}
	data, e := io.ReadAll(io.LimitReader(f, limits.InputBytes+1))
	f.Close()
	if e != nil {
		return verifiedReceipt{}, e
	}
	after, e := os.Stat(req.Source)
	if e != nil {
		return verifiedReceipt{}, e
	}
	if int64(len(data)) > limits.InputBytes || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return verifiedReceipt{}, failure(1, "execution", fmt.Errorf("source changed during snapshot"))
	}
	input := contracts.InputRef{Locator: req.Source, SHA256: hash(data), Bytes: int64(len(data))}
	input.ID = hash(encode([]string{input.Locator, input.SHA256}))
	report.Inputs = []contracts.InputRef{input}
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
	report.Resources["collection"] = collectionUsage
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

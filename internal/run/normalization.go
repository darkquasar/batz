package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"time"

	"batz/internal/contracts"
)

func normalize(ctx context.Context, req Request, db *sql.DB, artifact string, lease *os.File, inventory committedInventory, priorNormalized sql.NullString, report contracts.RunReport, checkpoint func(string)) (contracts.RunReport, error) {
	report.Outcome = "normalization-failed"
	limits := report.Limits
	attempt := filepath.Join(artifact, "builds", string(report.Identity.Attempt))
	if e := os.MkdirAll(attempt, 0700); e != nil {
		return report, e
	}
	content, e := copyProject(req.Project, filepath.Join(attempt, "project"))
	if e != nil {
		return report, failure(2, "invalid-input", e)
	}
	executable, e := os.Executable()
	if e != nil {
		return report, e
	}
	for name, path := range map[string]string{"collector-config": req.NativeConfig, "runtime-lock": filepath.Join(req.Root, "runtime/dependencies.lock.json"), "model-runner": filepath.Join(req.Root, "runtime/sqlmesh/runner.py"), "model-catalog": filepath.Join(req.Root, "runtime/sqlmesh/catalog.py"), "coordinator": executable} {
		ref, e := fileRef(req.Root, path, 0)
		if e != nil {
			return report, e
		}
		content[name] = ref.SHA256
	}
	for name, path := range req.Scope.Sources {
		digest, ok := content[path]
		if !ok {
			return report, failure(4, "integrity", errors.New("discovered source missing from execution snapshot"))
		}
		content["model:"+name] = digest
	}
	content["execution-scope"] = hash(encode(req.Scope))
	content["pipeline"] = hash(encode(req.Contract))
	report.Content = content
	if priorNormalized.Valid {
		var raw []byte
		if e := db.QueryRow("SELECT inventory FROM normalized WHERE revision=? AND binding=?", priorNormalized.String, req.Binding).Scan(&raw); e != nil {
			return report, e
		}
		var current contracts.PublishedInventory
		if e := json.Unmarshal(raw, &current); e != nil {
			return report, e
		}
		if current.InputRevision == inventory.revision && string(current.InputDigest) == inventory.digest && reflect.DeepEqual(current.Content, content) {
			files := append([]contracts.FileRef{current.Catalog}, current.RejectFiles...)
			for _, dataset := range current.Datasets {
				files = append(files, dataset.Files...)
			}
			for _, file := range files {
				if e := verifyFile(artifact, file); e != nil {
					return report, failure(4, "integrity", e)
				}
			}
			report = applyPublication(artifact, current, report)
			if e := saveRun(db, artifact, req, report); e != nil {
				return report, e
			}
			checkpointAt(checkpoint, "after-normalized-commit")
			return report, nil
		}
	}
	revision := contracts.RevisionID(id())
	nonce := id()
	modelState := filepath.Join(attempt, "model-state")
	output := filepath.Join(artifact, "normalized", string(revision))
	if e = os.MkdirAll(filepath.Dir(output), 0700); e != nil {
		return report, e
	}
	buildRequest := map[string]any{"scope": req.Scope, "contract": req.Contract, "artifact_root": artifact, "project": filepath.Join(attempt, "project"), "model_state": modelState, "output": output, "members": inventory.members, "input_revision": inventory.revision, "input_digest": inventory.digest, "attempt_id": report.Identity.Attempt, "evaluation_nonce": nonce, "binding_id": req.Binding, "revision": revision}
	requestPath := filepath.Join(attempt, "request.json")
	if e = writeJSON(requestPath, buildRequest); e != nil {
		return report, e
	}
	report.Outcome = "running"
	if e := saveRun(db, artifact, req, report); e != nil {
		return report, e
	}
	report.Outcome = "normalization-failed"
	modelCtx, modelCancel := context.WithTimeout(ctx, time.Duration(limits.ModelSeconds)*time.Second)
	modelUsage, e := sandboxedChild(req.PrivateScratch, req.SandboxProfile, modelCtx, lease, attempt, filepath.Join(attempt, "model.log"), childBudget{limits.ScratchBytes, []string{attempt, output, filepath.Join(artifact, "parsed", string(inventory.revision))}}, nil, filepath.Join(req.Root, "runtime/.local/venv/bin/python"), "-I", "-B", filepath.Join(req.Root, "runtime/sqlmesh/runner.py"), requestPath)
	report.Resources["model"] = modelUsage
	modelCancel()
	// SQLMesh state is disposable once its process group has stopped. Preserve
	// it if settlement failed; logs and requests remain available in all cases.
	var unsettled unsettledProcessError
	if !errors.As(e, &unsettled) {
		e = errors.Join(e, os.RemoveAll(modelState))
	}
	if e != nil {
		return report, modelFailure(e)
	}
	var built builtRevision
	b, e := os.ReadFile(filepath.Join(output, "built.json"))
	if e != nil {
		return report, e
	}
	if e = json.Unmarshal(b, &built); e != nil {
		return report, e
	}
	if e = verifyBuilt(artifact, built, inventory, report.Identity.Attempt, nonce); e != nil {
		return report, e
	}
	report.Counts = append(report.Counts, built.Counts)
	quality, _ := publicationPolicy(report.Counts, req.Strict)
	allInputs := []contracts.InputRef{}
	sources := []contracts.FileRef{}
	rejects := append([]contracts.FileRef(nil), built.Rejects...)
	for _, r := range inventory.receipts {
		allInputs = append(allInputs, r.Inputs...)
		sources = append(sources, r.SourceFiles...)
		rejects = append(rejects, r.RejectFiles...)
	}
	published := contracts.PublishedInventory{Reconciliation: built.Reconciliation, APIVersion: contracts.APIVersion, Kind: "PublishedInventory", Revision: revision, Identity: report.Identity, InputRevision: inventory.revision, InputDigest: contracts.MembershipDigest(inventory.digest), Inputs: allInputs, Profile: contracts.Profile, Content: content, Datasets: built.Datasets, SourceFiles: sources, RejectFiles: rejects, Counts: report.Counts, Quality: quality, Catalog: built.Catalog}
	if e = writeJSON(filepath.Join(output, "inventory.json"), published); e != nil {
		return report, e
	}
	if e = syncDir(output); e != nil {
		return report, e
	}
	if e = syncDir(filepath.Dir(output)); e != nil {
		return report, e
	}
	report.Outcome = "publication-failed"
	publishedReport := applyPublication(artifact, published, report)
	checkpointAt(checkpoint, "before-normalized-commit")
	if e = commitNormalized(db, published, publishedReport, priorNormalized); e != nil {
		return report, e
	}
	checkpointAt(checkpoint, "after-normalized-commit")
	return publishedReport, nil
}

func applyPublication(root string, published contracts.PublishedInventory, report contracts.RunReport) contracts.RunReport {
	report.Outcome = "published"
	report.Reconciliation = published.Reconciliation
	report.Quality = &published.Quality
	report.NormalizedRevision = &published.Revision
	report.Counts = published.Counts
	catalog := filepath.Join(root, published.Catalog.Path)
	report.Catalog = &catalog
	for _, count := range report.Counts {
		if count.Boundary == "normalize" {
			report.Coverage = contracts.Coverage{Mapped: count.Accepted, Unsupported: count.Unsupported}
		}
	}
	_, report.Exit = publicationPolicy(report.Counts, report.Strict)
	return report
}

func verifyBuilt(artifact string, built builtRevision, inventory committedInventory, attempt contracts.AttemptID, nonce string) error {
	var e error
	if !built.Checks || built.InputRevision != inventory.revision || built.InputDigest != inventory.digest || built.AttemptID != attempt || built.Nonce != nonce {
		return failure(4, "integrity", errors.New("stale evaluated build"))
	}
	var remaining int64
	for _, receipt := range inventory.receipts {
		remaining += receipt.Counts.Accepted
	}
	if built.Counts.Boundary != "normalize" {
		return failure(4, "integrity", errors.New("invalid normalization boundary"))
	}
	for _, count := range []int64{built.Counts.Accepted, built.Counts.Rejected, built.Counts.Unsupported, built.Counts.Excluded} {
		if count < 0 || count > remaining {
			return failure(4, "integrity", errors.New("normalization accounting mismatch"))
		}
		remaining -= count
	}
	if remaining != 0 {
		return failure(4, "integrity", errors.New("unaccounted committed observations"))
	}
	if r := built.Reconciliation; r != nil {
		if r.Observations != built.Counts.Accepted || r.Variants < 0 || r.Variants > r.Observations || r.LogicalEvents < 0 || r.LogicalEvents > r.Variants || r.ConflictingEvents < 0 || r.ConflictingEvents > r.LogicalEvents || r.UnidentifiedObservations < 0 || r.UnidentifiedObservations > r.Observations {
			return failure(4, "integrity", errors.New("invalid reconciliation accounting"))
		}
	}
	rejectRows := built.Counts.Rejected
	for _, file := range built.Rejects {
		if file.Rows < 0 || file.Rows > rejectRows {
			return failure(4, "integrity", errors.New("reject evidence count mismatch"))
		}
		rejectRows -= file.Rows
	}
	if rejectRows != 0 {
		return failure(4, "integrity", errors.New("missing reject evidence"))
	}
	for _, dataset := range built.Datasets {
		var rows int64
		for _, file := range dataset.Files {
			if e = verifyFile(artifact, file); e != nil {
				return failure(4, "integrity", e)
			}
			rows += file.Rows
		}
		if rows != dataset.Rows {
			return failure(4, "integrity", errors.New("dataset counts"))
		}
	}
	if e = verifyFile(artifact, built.Catalog); e != nil {
		return failure(4, "integrity", e)
	}
	for _, file := range built.Rejects {
		if e = verifyFile(artifact, file); e != nil {
			return failure(4, "integrity", e)
		}
	}
	return nil
}

func modelFailure(e error) error {
	var childExit *exec.ExitError
	if errors.As(e, &childExit) && childExit.ExitCode() == 4 {
		return failure(4, "assertion", e)
	}
	if errors.As(e, &childExit) && childExit.ExitCode() == 3 {
		return &Failure{Code: 3, Category: "incomplete-capability", DiagnosticCode: "timestamp-representation-unavailable", Message: "timestamp-representation-unavailable: supplied timestamp needs an owner-approved representation; " + e.Error()}
	}
	return e
}

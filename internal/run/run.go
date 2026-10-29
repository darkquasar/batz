// Package run owns the local acquisition/publication lifecycle behind one command.
// Verified receipts and committed memberships are private values of this package.
package run

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"

	"batz/collector/plugins"
	"batz/internal/contracts"
	"github.com/parquet-go/parquet-go"
	_ "modernc.org/sqlite"
)

type Request struct {
	PrivateScratch                                                             string
	CaseID                                                                     string
	Root, Workspace, Definition, Route, Source, Binding, NativeConfig, Project string
	SandboxProfile                                                             string
	Strict                                                                     bool
}
type Failure struct {
	Code                              int
	Category, Message, DiagnosticCode string
}

func (e *Failure) Error() string { return e.Category + ": " + e.Message }
func failure(code int, category string, err error) error {
	return &Failure{Code: code, Category: category, Message: err.Error()}
}
func Limits() contracts.Limits {
	return contracts.Limits{InputFiles: 1, InputBytes: 16 << 20, Records: 10000, CollectionSeconds: 120, ModelSeconds: 300, DuckDBThreads: 1, ScratchBytes: 1 << 30, CollectionSlots: 1, ModelSlots: 1}
}
func id() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func writeJSON(path string, v any) error { return durableWrite(path, encode(v)) }
func durableWrite(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func fileRef(root, path string, rows int64) (contracts.FileRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return contracts.FileRef{}, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return contracts.FileRef{}, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return contracts.FileRef{}, err
	}
	return contracts.FileRef{Path: filepath.ToSlash(rel), SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: size, Rows: rows}, nil
}
func confined(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") || strings.Contains(relative, ":") {
		return "", fmt.Errorf("invalid relative path")
	}
	for _, part := range strings.Split(relative, "/") {
		if part == ".." || part == "" || part == "." {
			return "", fmt.Errorf("invalid path component")
		}
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if resolved != path {
		return "", fmt.Errorf("symlinks are not admitted")
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes artifact root")
	}
	stat, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !stat.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file")
	}
	return path, nil
}
func verifyFile(root string, f contracts.FileRef) error {
	if f.Bytes < 0 || f.Rows < 0 {
		return fmt.Errorf("negative counts")
	}
	p, err := confined(root, f.Path)
	if err != nil {
		return err
	}
	actual, err := fileRef(root, p, f.Rows)
	if err != nil {
		return err
	}
	if actual.SHA256 != f.SHA256 || actual.Bytes != f.Bytes {
		return fmt.Errorf("file identity mismatch: %s", f.Path)
	}
	return nil
}

type member struct {
	File     contracts.FileRef    `json:"file"`
	Revision contracts.RevisionID `json:"revision"`
}
type committedInventory struct {
	revision contracts.RevisionID
	digest   string
	members  []member
	receipts []contracts.CollectionReceipt
}
type verifiedReceipt struct {
	receipt contracts.CollectionReceipt
	stage   string
}
type builtRevision struct {
	InputRevision contracts.RevisionID `json:"input_revision"`
	InputDigest   string               `json:"input_digest"`
	AttemptID     contracts.AttemptID  `json:"attempt_id"`
	Nonce         string               `json:"evaluation_nonce"`
	Datasets      []contracts.Dataset  `json:"datasets"`
	Catalog       contracts.FileRef    `json:"catalog"`
	Rejects       []contracts.FileRef  `json:"rejects"`
	Counts        contracts.Counts     `json:"counts"`
	Checks        bool                 `json:"checks_passed"`
}

func verifyReceipt(stage string, expected plugins.Request) (verifiedReceipt, error) {
	b, err := os.ReadFile(filepath.Join(stage, "receipt.json"))
	if err != nil {
		return verifiedReceipt{}, err
	}
	var receipt contracts.CollectionReceipt
	if err = decodeWire(b, &receipt); err != nil {
		return verifiedReceipt{}, err
	}
	if receipt.APIVersion != contracts.APIVersion || receipt.Kind != "CollectionReceipt" || !receipt.Complete || !reflect.DeepEqual(receipt.Identity, expected.Identity) || !reflect.DeepEqual(receipt.Inputs, []contracts.InputRef{expected.Input}) {
		return verifiedReceipt{}, fmt.Errorf("completion identity mismatch")
	}
	if receipt.Continuation != nil || receipt.RejectFiles == nil || receipt.Counts.Boundary != "parse" || receipt.Counts.Unsupported != 0 || receipt.Counts.Excluded != 0 || receipt.Counts.Accepted < 0 || receipt.Counts.Rejected < 0 || receipt.Counts.Accepted+receipt.Counts.Rejected > expected.Limits.Records {
		return verifiedReceipt{}, fmt.Errorf("invalid receipt counts")
	}
	if len(receipt.SourceFiles) != 1 || len(receipt.ParsedFiles) != 1 || len(receipt.RejectFiles) > 1 {
		return verifiedReceipt{}, fmt.Errorf("invalid closed inventory")
	}
	sourceBytes, err := os.ReadFile(expected.Source)
	if err != nil {
		return verifiedReceipt{}, err
	}
	if hash(sourceBytes) != expected.Input.SHA256 {
		return verifiedReceipt{}, fmt.Errorf("source snapshot changed")
	}
	var source struct {
		Records []json.RawMessage `json:"Records"`
	}
	if err = json.Unmarshal(sourceBytes, &source); err != nil || source.Records == nil {
		return verifiedReceipt{}, fmt.Errorf("invalid source shape")
	}
	if int64(len(source.Records)) != receipt.Counts.Accepted+receipt.Counts.Rejected {
		return verifiedReceipt{}, fmt.Errorf("source accounting mismatch")
	}
	ordinals := map[int64]bool{}
	var total [2]int64
	seen := map[string]bool{}
	for kind, files := range [][]contracts.FileRef{receipt.ParsedFiles, receipt.RejectFiles} {
		for _, f := range files {
			if seen[f.Path] {
				return verifiedReceipt{}, fmt.Errorf("duplicate file")
			}
			seen[f.Path] = true
			if err = verifyFile(stage, f); err != nil {
				return verifiedReceipt{}, err
			}
			pf, e := os.Open(filepath.Join(stage, f.Path))
			if e != nil {
				return verifiedReceipt{}, e
			}
			stat, e := pf.Stat()
			if e != nil {
				pf.Close()
				return verifiedReceipt{}, e
			}
			footer, e := parquet.OpenFile(pf, stat.Size())
			pf.Close()
			if e != nil {
				return verifiedReceipt{}, e
			}
			if footer.Schema().String() != parquet.SchemaOf(new(plugins.Record)).String() {
				return verifiedReceipt{}, fmt.Errorf("parsed schema mismatch")
			}
			rows, e := parquet.ReadFile[plugins.Record](filepath.Join(stage, f.Path))
			if e != nil {
				return verifiedReceipt{}, e
			}
			if int64(len(rows)) != f.Rows {
				return verifiedReceipt{}, errors.New("parquet row count mismatch")
			}
			for _, row := range rows {
				if row.BindingID != string(expected.Identity.Binding) || row.RunID != string(expected.Identity.Run) || row.BatchID != string(expected.Identity.Batch) || row.AttemptID != string(expected.Identity.Attempt) || row.InputID != expected.Input.ID || row.InputSHA256 != expected.Input.SHA256 || row.Ordinal < 0 || row.Ordinal >= int64(len(source.Records)) || ordinals[row.Ordinal] {
					return verifiedReceipt{}, fmt.Errorf("per-record provenance mismatch")
				}
				ordinals[row.Ordinal] = true
				expectedID := hash(encode([]any{expected.Identity.Binding, expected.Identity.Batch, expected.Input.ID, row.Ordinal}))
				status := "accepted"
				if kind == 1 {
					status = "rejected"
				}
				if row.ObservationID != expectedID || row.InputLocator != expected.Input.Locator || row.AcquiredAt != expected.AcquiredAt || row.Representation != "reconstructed-json" || row.ParseStatus != status || (kind == 0 && row.RejectReason != "") || (kind == 1 && row.RejectReason == "") {
					return verifiedReceipt{}, fmt.Errorf("record identity/status mismatch")
				}
				src := json.NewDecoder(bytes.NewReader(source.Records[row.Ordinal]))
				src.UseNumber()
				retained := json.NewDecoder(strings.NewReader(row.Payload))
				retained.UseNumber()
				var a, b any
				if src.Decode(&a) != nil || retained.Decode(&b) != nil || !reflect.DeepEqual(a, b) {
					return verifiedReceipt{}, fmt.Errorf("record payload differs from source")
				}
			}
			total[kind] += f.Rows
		}
	}
	if total[0] != receipt.Counts.Accepted || total[1] != receipt.Counts.Rejected {
		return verifiedReceipt{}, fmt.Errorf("receipt accounting mismatch")
	}
	for _, f := range receipt.SourceFiles {
		if seen[f.Path] {
			return verifiedReceipt{}, fmt.Errorf("duplicate source")
		}
		if err = verifyFile(stage, f); err != nil {
			return verifiedReceipt{}, err
		}
		if f.SHA256 != expected.Input.SHA256 || f.Bytes != expected.Input.Bytes || f.Rows != 0 {
			return verifiedReceipt{}, fmt.Errorf("source identity mismatch")
		}
	}
	return verifiedReceipt{receipt, stage}, nil
}
func openState(root string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", filepath.Join(root, "state.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version != 0 && version != 1 {
		db.Close()
		return nil, fmt.Errorf("unsupported metadata version %d", version)
	}
	_, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=1000; CREATE TABLE IF NOT EXISTS bindings(id TEXT PRIMARY KEY,selection TEXT NOT NULL,parsed TEXT,normalized TEXT); CREATE TABLE IF NOT EXISTS parsed(seq INTEGER PRIMARY KEY AUTOINCREMENT,revision TEXT UNIQUE NOT NULL,binding TEXT NOT NULL,receipt BLOB NOT NULL); CREATE TABLE IF NOT EXISTS normalized(revision TEXT PRIMARY KEY,binding TEXT NOT NULL,inventory BLOB NOT NULL); PRAGMA user_version=1")
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func commitParsed(db *sql.DB, root string, verified verifiedReceipt, prior sql.NullString) (committedInventory, error) {
	if verified.stage == "" {
		return committedInventory{}, fmt.Errorf("unverified receipt")
	}
	revision := contracts.RevisionID(id())
	dest := filepath.Join(root, "parsed", string(revision))
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return committedInventory{}, err
	}
	if err := os.Rename(verified.stage, dest); err != nil {
		return committedInventory{}, err
	}
	if err := syncDir(filepath.Dir(dest)); err != nil {
		return committedInventory{}, err
	}
	receipt := verified.receipt
	for _, files := range [][]contracts.FileRef{receipt.ParsedFiles, receipt.RejectFiles, receipt.SourceFiles} {
		for i := range files {
			files[i].Path = filepath.ToSlash(filepath.Join("parsed", string(revision), files[i].Path))
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return committedInventory{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO parsed(revision,binding,receipt) VALUES (?,?,?)", revision, receipt.Identity.Binding, encode(receipt)); err != nil {
		return committedInventory{}, err
	}
	result, err := tx.Exec("UPDATE bindings SET parsed=? WHERE id=? AND parsed IS ?", revision, receipt.Identity.Binding, prior)
	if err != nil {
		return committedInventory{}, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return committedInventory{}, fmt.Errorf("parsed revision compare-and-swap failed")
	}
	if err = tx.Commit(); err != nil {
		return committedInventory{}, err
	}
	rows, err := db.Query("SELECT revision,receipt FROM parsed WHERE binding=? ORDER BY seq", receipt.Identity.Binding)
	if err != nil {
		return committedInventory{}, err
	}
	defer rows.Close()
	inventory := committedInventory{revision: revision}
	for rows.Next() {
		var rev contracts.RevisionID
		var raw []byte
		if err = rows.Scan(&rev, &raw); err != nil {
			return committedInventory{}, err
		}
		var r contracts.CollectionReceipt
		if err = json.Unmarshal(raw, &r); err != nil {
			return committedInventory{}, err
		}
		inventory.receipts = append(inventory.receipts, r)
		for _, f := range r.ParsedFiles {
			inventory.members = append(inventory.members, member{f, rev})
		}
	}
	if err = rows.Err(); err != nil {
		return committedInventory{}, err
	}
	sorted := append([]member(nil), inventory.members...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].File.Path < sorted[j].File.Path })
	// encoding/json sorts map keys; integer and ASCII path values are canonical here.
	files := []map[string]any{}
	for _, m := range sorted {
		files = append(files, map[string]any{"path": m.File.Path, "sha256": m.File.SHA256, "bytes": m.File.Bytes, "rows": m.File.Rows})
	}
	inventory.digest = hash(encode(map[string]any{"binding_id": receipt.Identity.Binding, "input_revision": revision, "files": files}))
	return inventory, nil
}
func copyProject(source, destination string) (map[string]string, error) {
	hashes := map[string]string{}
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in native project")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(destination, rel), 0700)
		}
		ext := filepath.Ext(path)
		if ext != ".sql" && ext != ".yaml" && ext != ".yml" && ext != ".csv" {
			return fmt.Errorf("unsupported project content: %s", rel)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = hash(b)
		return durableWrite(filepath.Join(destination, rel), b)
	})
	return hashes, err
}
func Execute(ctx context.Context, req Request) (report contracts.RunReport, err error) {
	limits := Limits()
	report = contracts.RunReport{Resources: map[string]contracts.ResourceUsage{}, APIVersion: contracts.APIVersion, Kind: "RunReport", Identity: contracts.Identity{Binding: contracts.BindingID(req.Binding), Run: contracts.RunID(id()), Batch: contracts.BatchID(id()), Attempt: contracts.AttemptID(id())}, Outcome: "running", Strict: req.Strict, Limits: limits, Inputs: []contracts.InputRef{}, Counts: []contracts.Counts{}, Content: map[string]string{}, Resolution: map[string]contracts.ValueOrigin{}, Diagnostics: []contracts.Diagnostic{}}
	if req.CaseID != "" {
		caseID := req.CaseID
		report.Identity.Case = &caseID
	}
	defer func() {
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			err = failure(130, "cancelled", ctx.Err())
			report.Outcome = "cancelled"
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
	}()
	if ctx.Err() != nil {
		return report, ctx.Err()
	}

	artifact := filepath.Join(req.Workspace, ".batz")
	if err = os.MkdirAll(artifact, 0700); err != nil {
		return report, err
	}
	artifact, err = filepath.EvalSymlinks(artifact)
	if err != nil {
		return report, err
	}
	lease, e := os.OpenFile(filepath.Join(artifact, "owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return report, e
	}
	defer lease.Close()
	if e = syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return report, failure(5, "busy", e)
	}
	db, e := openState(artifact)
	if e != nil {
		return report, e
	}
	defer db.Close()
	selection := string(encode([]string{req.Definition, req.Route, req.Source}))
	var previousSelection string
	var priorParsed, priorNormalized sql.NullString
	e = db.QueryRow("SELECT selection,parsed,normalized FROM bindings WHERE id=?", req.Binding).Scan(&previousSelection, &priorParsed, &priorNormalized)
	if e == sql.ErrNoRows {
		_, e = db.Exec("INSERT INTO bindings(id,selection) VALUES (?,?)", req.Binding, selection)
	} else if e == nil && previousSelection != selection {
		return report, failure(2, "invalid-input", fmt.Errorf("incompatible binding reuse"))
	}
	if e != nil {
		return report, e
	}
	if priorNormalized.Valid {
		v := contracts.RevisionID(priorNormalized.String)
		report.NormalizedRevision = &v
	}
	stage := filepath.Join(artifact, "staging", string(report.Identity.Attempt))
	if e = os.MkdirAll(stage, 0700); e != nil {
		return report, e
	}
	before, e := os.Stat(req.Source)
	if e != nil {
		return report, e
	}
	if !before.Mode().IsRegular() || before.Size() > limits.InputBytes {
		return report, failure(2, "invalid-input", fmt.Errorf("source must be one regular file within byte budget"))
	}
	f, e := os.Open(req.Source)
	if e != nil {
		return report, e
	}
	data, e := io.ReadAll(io.LimitReader(f, limits.InputBytes+1))
	f.Close()
	if e != nil {
		return report, e
	}
	after, e := os.Stat(req.Source)
	if e != nil {
		return report, e
	}
	if int64(len(data)) > limits.InputBytes || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return report, failure(1, "execution", fmt.Errorf("source changed during snapshot"))
	}
	input := contracts.InputRef{Locator: req.Source, SHA256: hash(data), Bytes: int64(len(data))}
	input.ID = hash(encode([]string{input.Locator, input.SHA256}))
	report.Inputs = []contracts.InputRef{input}
	if e = durableWrite(filepath.Join(stage, "input.json"), data); e != nil {
		return report, e
	}
	collectorRequest := plugins.Request{Identity: report.Identity, Input: input, Stage: stage, Source: filepath.Join(stage, "input.json"), AcquiredAt: time.Now().UTC().Format(time.RFC3339Nano), Limits: limits}
	if e = writeJSON(filepath.Join(stage, "context.json"), collectorRequest); e != nil {
		return report, e
	}
	if e = os.Chmod(filepath.Join(stage, "context.json"), 0400); e != nil {
		return report, e
	}
	environment := []string{
		"BATZ_STAGE_DIR=" + stage, "BATZ_SOURCE_PATH=" + collectorRequest.Source,
		"BATZ_RUN_ID=" + string(report.Identity.Run), "BATZ_BINDING_ID=" + string(report.Identity.Binding),
		"BATZ_BATCH_ID=" + string(report.Identity.Batch), "BATZ_ATTEMPT_ID=" + string(report.Identity.Attempt),
	}
	if report.Identity.Case != nil {
		environment = append(environment, "BATZ_CASE_ID="+*report.Identity.Case)
	}
	report.Outcome = "acquisition-failed"
	collectCtx, cancel := context.WithTimeout(ctx, time.Duration(limits.CollectionSeconds)*time.Second)
	collectionUsage, e := sandboxedChild(req.PrivateScratch, req.SandboxProfile, collectCtx, lease, stage, filepath.Join(stage, "collector.log"), childBudget{limits.ScratchBytes, []string{stage}}, environment, filepath.Join(req.Root, "runtime/.local/batz-collector"), req.NativeConfig, filepath.Join(stage, "context.json"))
	report.Resources["collection"] = collectionUsage
	cancel()
	if e != nil {
		var childExit *exec.ExitError
		if errors.As(e, &childExit) && childExit.ExitCode() == 2 {
			return report, failure(1, "execution", fmt.Errorf("unsupported-shape: expected CloudTrail Records JSON (%w)", e))
		}
		return report, e
	}
	verified, e := verifyReceipt(stage, collectorRequest)
	if e != nil {
		return report, failure(4, "integrity", e)
	}
	inventory, e := commitParsed(db, artifact, verified, priorParsed)
	if e != nil {
		return report, e
	}
	report.ParsedRevision = &inventory.revision
	report.AcquisitionComplete = true
	report.Outcome = "parsed-committed"
	parseCounts := contracts.Counts{Boundary: "parse"}
	report.Inputs = []contracts.InputRef{}
	for _, r := range inventory.receipts {
		parseCounts.Accepted += r.Counts.Accepted
		parseCounts.Rejected += r.Counts.Rejected
		report.Inputs = append(report.Inputs, r.Inputs...)
	}
	report.Counts = append(report.Counts, parseCounts)
	attempt := filepath.Join(artifact, "builds", string(report.Identity.Attempt))
	if e = os.MkdirAll(attempt, 0700); e != nil {
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
	for name, path := range map[string]string{"pipeline": req.Definition, "collector-config": req.NativeConfig, "runtime-lock": filepath.Join(req.Root, "runtime/dependencies.lock.json"), "model-runner": filepath.Join(req.Root, "runtime/sqlmesh/runner.py"), "coordinator": executable} {
		ref, e := fileRef(req.Root, path, 0)
		if e != nil {
			return report, e
		}
		content[name] = ref.SHA256
	}
	report.Content = content
	revision := contracts.RevisionID(id())
	nonce := id()
	bindingHash := hash([]byte(req.Binding))
	output := filepath.Join(artifact, "normalized", string(revision))
	if e = os.MkdirAll(filepath.Dir(output), 0700); e != nil {
		return report, e
	}
	buildRequest := map[string]any{"artifact_root": artifact, "project": filepath.Join(attempt, "project"), "model_state": filepath.Join(artifact, "models", bindingHash), "output": output, "members": inventory.members, "input_revision": inventory.revision, "input_digest": inventory.digest, "attempt_id": report.Identity.Attempt, "evaluation_nonce": nonce, "binding_id": req.Binding, "revision": revision}
	requestPath := filepath.Join(attempt, "request.json")
	if e = writeJSON(requestPath, buildRequest); e != nil {
		return report, e
	}
	report.Outcome = "normalization-failed"
	modelCtx, modelCancel := context.WithTimeout(ctx, time.Duration(limits.ModelSeconds)*time.Second)
	modelUsage, e := sandboxedChild(req.PrivateScratch, req.SandboxProfile, modelCtx, lease, attempt, filepath.Join(attempt, "model.log"), childBudget{limits.ScratchBytes, []string{attempt, output, filepath.Join(artifact, "models", bindingHash), filepath.Join(artifact, "parsed", string(inventory.revision))}}, nil, filepath.Join(req.Root, "runtime/.local/venv/bin/python"), "-I", filepath.Join(req.Root, "runtime/sqlmesh/runner.py"), requestPath)
	report.Resources["model"] = modelUsage
	modelCancel()
	if e != nil {
		var childExit *exec.ExitError
		if errors.As(e, &childExit) && childExit.ExitCode() == 4 {
			return report, failure(4, "assertion", e)
		}
		if errors.As(e, &childExit) && childExit.ExitCode() == 3 {
			return report, &Failure{Code: 3, Category: "incomplete-capability", DiagnosticCode: "timestamp-representation-unavailable", Message: "timestamp-representation-unavailable: supplied timestamp needs an owner-approved representation; " + e.Error()}
		}
		return report, e
	}
	var built builtRevision
	b, e := os.ReadFile(filepath.Join(output, "built.json"))
	if e != nil {
		return report, e
	}
	if e = json.Unmarshal(b, &built); e != nil {
		return report, e
	}
	if !built.Checks || built.InputRevision != inventory.revision || built.InputDigest != inventory.digest || built.AttemptID != report.Identity.Attempt || built.Nonce != nonce {
		return report, failure(4, "integrity", fmt.Errorf("stale evaluated build"))
	}
	for _, dataset := range built.Datasets {
		var rows int64
		for _, file := range dataset.Files {
			if e = verifyFile(artifact, file); e != nil {
				return report, failure(4, "integrity", e)
			}
			rows += file.Rows
		}
		if rows != dataset.Rows {
			return report, failure(4, "integrity", fmt.Errorf("dataset counts"))
		}
	}
	if e = verifyFile(artifact, built.Catalog); e != nil {
		return report, failure(4, "integrity", e)
	}
	for _, file := range built.Rejects {
		if e = verifyFile(artifact, file); e != nil {
			return report, failure(4, "integrity", e)
		}
	}
	quality := "complete"
	allInputs := []contracts.InputRef{}
	sources := []contracts.FileRef{}
	rejects := append([]contracts.FileRef(nil), built.Rejects...)
	for _, r := range inventory.receipts {
		allInputs = append(allInputs, r.Inputs...)
		sources = append(sources, r.SourceFiles...)
		rejects = append(rejects, r.RejectFiles...)
		if r.Counts.Rejected > 0 {
			quality = "partial"
		}
	}
	if built.Counts.Rejected > 0 {
		quality = "partial"
	}
	report.Counts = append(report.Counts, built.Counts)
	published := contracts.PublishedInventory{APIVersion: contracts.APIVersion, Kind: "PublishedInventory", Revision: revision, Identity: report.Identity, InputRevision: inventory.revision, InputDigest: contracts.MembershipDigest(inventory.digest), Inputs: allInputs, Profile: contracts.Profile, Content: content, Datasets: built.Datasets, SourceFiles: sources, RejectFiles: rejects, Counts: report.Counts, Quality: quality, Catalog: built.Catalog}
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
	tx, e := db.Begin()
	if e != nil {
		return report, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("INSERT INTO normalized(revision,binding,inventory) VALUES (?,?,?)", revision, req.Binding, encode(published)); e != nil {
		return report, e
	}
	result, e := tx.Exec("UPDATE bindings SET normalized=? WHERE id=? AND parsed=? AND normalized IS ?", revision, req.Binding, inventory.revision, priorNormalized)
	if e != nil {
		return report, e
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		return report, failure(4, "integrity", fmt.Errorf("normalized compare-and-swap failed"))
	}
	if e = tx.Commit(); e != nil {
		return report, e
	}
	report.Outcome = "published"
	report.Quality = &quality
	report.NormalizedRevision = &revision
	catalog := filepath.Join(artifact, built.Catalog.Path)
	report.Catalog = &catalog
	report.Coverage = contracts.Coverage{Mapped: built.Counts.Accepted, Unsupported: built.Counts.Unsupported}
	if req.Strict && quality == "partial" {
		report.Exit = 6
	}
	if e = os.MkdirAll(filepath.Join(artifact, "reports"), 0700); e != nil {
		return report, e
	}
	if e = writeJSON(filepath.Join(artifact, "reports", string(report.Identity.Run)+".json"), report); e != nil {
		return report, e
	}
	return report, nil
}

// NewBindingID returns a fresh independent binding scope for a direct run.
func NewBindingID() string { return id() }

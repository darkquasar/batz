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
	"path/filepath"
	"reflect"
	"strings"
	"syscall"

	"batz/collector/plugins"
	"batz/internal/contracts"
	"github.com/parquet-go/parquet-go"
	_ "modernc.org/sqlite"
)

type Request struct {
	Scope          contracts.NativeScope            `json:"scope"`
	Contract       contracts.ExecutionContract      `json:"contract"`
	PrivateScratch string                           `json:"private_scratch"`
	CaseID         string                           `json:"case_id"`
	Root           string                           `json:"root"`
	Workspace      string                           `json:"workspace"`
	Definition     string                           `json:"definition"`
	Route          string                           `json:"route"`
	Source         string                           `json:"source"`
	Binding        string                           `json:"binding"`
	NativeConfig   string                           `json:"native_config"`
	Project        string                           `json:"project"`
	SandboxProfile string                           `json:"sandbox_profile"`
	Strict         bool                             `json:"strict"`
	Retry          bool                             `json:"retry"`
	RetryRun       contracts.RunID                  `json:"retry_run_id,omitempty"`
	Resolution     map[string]contracts.ValueOrigin `json:"resolution,omitempty"`
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
	records, err := plugins.SourceRecords(sourceBytes, expected.Framing)
	if err != nil {
		return verifiedReceipt{}, err
	}
	if int64(len(records)) != receipt.Counts.Accepted+receipt.Counts.Rejected {
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
				if row.BindingID != string(expected.Identity.Binding) || row.RunID != string(expected.Identity.Run) || row.BatchID != string(expected.Identity.Batch) || row.AttemptID != string(expected.Identity.Attempt) || row.InputID != expected.Input.ID || row.InputSHA256 != expected.Input.SHA256 || row.Ordinal < 0 || row.Ordinal >= int64(len(records)) || ordinals[row.Ordinal] {
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
				src := json.NewDecoder(bytes.NewReader(records[row.Ordinal]))
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
func copyProject(source, destination string) (map[string]string, error) {
	hashes := map[string]string{}
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if !contracts.ProductionProjectPath(filepath.ToSlash(rel)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in native project")
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
func Execute(ctx context.Context, req Request) (contracts.RunReport, error) {
	return execute(ctx, req, nil)
}

func checkpointAt(checkpoint func(string), name string) {
	if checkpoint != nil {
		checkpoint(name)
	}
}

func execute(ctx context.Context, req Request, checkpoint func(string)) (report contracts.RunReport, err error) {
	limits := Limits()
	report = contracts.RunReport{Resources: map[string]contracts.ResourceUsage{}, APIVersion: contracts.APIVersion, Kind: "RunReport", Identity: contracts.Identity{Binding: contracts.BindingID(req.Binding), Run: contracts.RunID(id()), Batch: contracts.BatchID(id()), Attempt: contracts.AttemptID(id())}, Outcome: "running", Strict: req.Strict, Limits: limits, Inputs: []contracts.InputRef{}, Counts: []contracts.Counts{}, Content: map[string]string{}, Resolution: map[string]contracts.ValueOrigin{}, Diagnostics: []contracts.Diagnostic{}}
	if req.Resolution != nil {
		report.Resolution = req.Resolution
	}
	if req.CaseID != "" {
		caseID := req.CaseID
		report.Identity.Case = &caseID
	}
	var state *sql.DB
	var owner *os.File
	started := false
	defer func() {
		err = finishFailure(ctx, &report, err)
		if started {
			err = persistOutcome(state, artifactRoot(req), req, &report, err)
		}
		if state != nil {
			state.Close()
		}
		if owner != nil {
			owner.Close()
		}
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
	owner = lease
	if e = syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return report, failure(5, "busy", e)
	}
	db, e := openState(artifact)
	if e != nil {
		return report, e
	}
	state = db
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
	if req.Retry && req.RetryRun != "" {
		var raw []byte
		if e := db.QueryRow("SELECT report FROM runs WHERE id=? AND binding=?", req.RetryRun, req.Binding).Scan(&raw); e != nil {
			return report, failure(2, "invalid-input", e)
		}
		var selected contracts.RunReport
		if e := json.Unmarshal(raw, &selected); e != nil {
			return report, failure(4, "integrity", e)
		}
		if selected.ParsedRevision == nil || !priorParsed.Valid || string(*selected.ParsedRevision) != priorParsed.String {
			return report, failure(2, "invalid-input", errors.New("requested run does not match current parsed inventory"))
		}
	}
	started = true
	if e = saveRun(db, artifact, req, report); e != nil {
		return report, e
	}
	var inventory committedInventory
	recovered, e := recoverParsed(db, artifact, req, report.Identity.Run)
	if e != nil {
		return report, e
	}
	if e = exportReports(db, artifact, req.Binding); e != nil {
		return report, e
	}
	if recovered != nil {
		priorParsed = sql.NullString{String: string(recovered.revision), Valid: true}
	}
	if recovered != nil {
		inventory = *recovered
	} else if req.Retry {
		if !priorParsed.Valid {
			return report, failure(2, "invalid-input", errors.New("no committed parsed inventory to retry"))
		}
		inventory, e = loadInventory(db, report.Identity.Binding, contracts.RevisionID(priorParsed.String))
	} else {
		var verified verifiedReceipt
		verified, e = collect(ctx, req, artifact, lease, &report, checkpoint)
		if e == nil {
			inventory, e = publishParsed(db, artifact, verified, priorParsed, checkpoint)
		}
	}
	if e != nil {
		return report, e
	}
	applyInventory(&report, inventory)
	if e := verifyInventory(artifact, inventory); e != nil {
		report.Outcome = "normalization-failed"
		return report, e
	}
	if req.Retry || recovered != nil {
		if e := saveRun(db, artifact, req, report); e != nil {
			return report, e
		}
	}
	checkpointAt(checkpoint, "after-parsed-commit")
	return normalize(ctx, req, db, artifact, lease, inventory, priorNormalized, report, checkpoint)
}

// NewBindingID returns a fresh independent binding scope for a direct run.
func NewBindingID() string { return id() }

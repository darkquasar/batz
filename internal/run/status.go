package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"batz/internal/contracts"
)

// Status keeps the command's recorded outcome separate from a successful read.
type Status struct {
	Run               contracts.RunReport   `json:"run"`
	ParsedFiles       []contracts.FileRef   `json:"parsed_files"`
	CurrentNormalized *contracts.RevisionID `json:"current_normalized_revision"`
	RetryEligible     bool                  `json:"retry_eligible"`
}

func artifactRoot(req Request) string { return filepath.Join(req.Workspace, ".batz") }

func saveRun(db *sql.DB, root string, req Request, report contracts.RunReport) error {
	if err := recordRun(db, req, report); err != nil {
		return err
	}
	return exportReport(root, report)
}

func recordRun(db *sql.DB, req Request, report contracts.RunReport) error {
	_, err := db.Exec("INSERT INTO runs(id,binding,request,report,prior_parsed) VALUES (?,?,?,?,(SELECT parsed FROM bindings WHERE id=?)) ON CONFLICT(id) DO UPDATE SET request=excluded.request,prior_parsed=CASE WHEN json_extract(runs.report,'$.identity.batch_id')<>json_extract(excluded.report,'$.identity.batch_id') THEN excluded.prior_parsed ELSE runs.prior_parsed END,report=excluded.report", report.Identity.Run, req.Binding, encode(req), encode(report), req.Binding)
	return err
}

func exportReport(root string, report contracts.RunReport) error {
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0700); err != nil {
		return err
	}
	return replaceJSON(filepath.Join(root, "reports", string(report.Identity.Run)+".json"), report)
}

func replaceJSON(path string, value any) error {
	tmp := path + "." + id() + ".tmp"
	if err := writeJSON(tmp, value); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func exportReports(db *sql.DB, root, binding string) error {
	rows, err := db.Query("SELECT id,report FROM runs WHERE binding=?", binding)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var runID string
		var raw []byte
		if err := rows.Scan(&runID, &raw); err != nil {
			return err
		}
		path := filepath.Join(root, "reports", runID+".json")
		if existing, err := os.ReadFile(path); err == nil && string(existing) == string(raw) {
			continue
		}
		if err := replaceJSON(path, json.RawMessage(raw)); err != nil {
			return err
		}
	}
	return rows.Err()
}

func readState(workspace string) (*sql.DB, error) {
	path, err := filepath.Abs(filepath.Join(workspace, ".batz", "state.sqlite"))
	if err != nil {
		return nil, err
	}
	location := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err == nil && version != 2 {
		err = fmt.Errorf("unsupported metadata version %d", version)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// ReadStatus opens existing coordinator state without taking the mutation lease.
// An empty run ID selects the latest recorded run.
func ReadStatus(workspace, runID string) (Status, error) {
	db, err := readState(workspace)
	if err != nil {
		return Status{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Status{}, err
	}
	defer tx.Rollback()
	var raw []byte
	err = tx.QueryRow("SELECT report FROM runs WHERE (?='' OR id=?) ORDER BY seq DESC LIMIT 1", runID, runID).Scan(&raw)
	if err != nil {
		return Status{}, err
	}
	status := Status{ParsedFiles: []contracts.FileRef{}}
	if err = json.Unmarshal(raw, &status.Run); err != nil {
		return Status{}, err
	}
	var parsed, current sql.NullString
	if err = tx.QueryRow("SELECT parsed,normalized FROM bindings WHERE id=?", status.Run.Identity.Binding).Scan(&parsed, &current); err != nil {
		return Status{}, err
	}
	if current.Valid {
		revision := contracts.RevisionID(current.String)
		status.CurrentNormalized = &revision
	}
	if status.Run.ParsedRevision != nil {
		inventory, err := loadInventory(tx, status.Run.Identity.Binding, *status.Run.ParsedRevision)
		if err != nil {
			return Status{}, err
		}
		for _, m := range inventory.members {
			status.ParsedFiles = append(status.ParsedFiles, m.File)
		}
		status.RetryEligible = status.Run.Outcome != "published" && parsed.Valid && parsed.String == string(*status.Run.ParsedRevision)
	}
	return status, nil
}

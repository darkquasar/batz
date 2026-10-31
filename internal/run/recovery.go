package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"batz/collector/plugins"
	"batz/internal/contracts"
)

type collectionCompletion struct {
	ContextSHA256 string `json:"context_sha256"`
	ReceiptSHA256 string `json:"receipt_sha256"`
}

func markCollectionComplete(stage string) error {
	contextBytes, err := os.ReadFile(filepath.Join(stage, "context.json"))
	if err != nil {
		return err
	}
	receiptBytes, err := os.ReadFile(filepath.Join(stage, "receipt.json"))
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(stage, "completed.json"), collectionCompletion{ContextSHA256: hash(contextBytes), ReceiptSHA256: hash(receiptBytes)}); err != nil {
		return err
	}
	return syncDir(stage)
}

// Only the owner calls recovery. Receipts without recorded clean child exit
// remain diagnostic evidence and cannot advance a checkpoint.
func recoverParsed(db *sql.DB, root string, req Request, current contracts.RunID) (*committedInventory, error) {
	rows, err := db.Query("SELECT request,report,prior_parsed FROM runs WHERE binding=? AND id<>? AND (?='' OR id=?) AND json_extract(report,'$.outcome') IN ('running','parsed-committed') ORDER BY seq", req.Binding, current, req.RetryRun, req.RetryRun)
	if err != nil {
		return nil, err
	}
	type pending struct {
		request, report []byte
		prior           sql.NullString
	}
	var work []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.request, &item.report, &item.prior); err != nil {
			rows.Close()
			return nil, err
		}
		work = append(work, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var recovered *committedInventory
	for _, item := range work {
		var original Request
		var report contracts.RunReport
		if err := json.Unmarshal(item.request, &original); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(item.report, &report); err != nil {
			return nil, err
		}
		if original.Definition != req.Definition || original.Route != req.Route || original.Source != req.Source {
			return nil, failure(4, "integrity", errors.New("interrupted run has incompatible selection"))
		}
		if report.AcquisitionComplete && report.ParsedRevision != nil {
			var latest string
			var published int
			if err := db.QueryRow("SELECT parsed FROM bindings WHERE id=?", req.Binding).Scan(&latest); err != nil {
				return nil, err
			}
			if err := db.QueryRow("SELECT count(*) FROM normalized WHERE binding=? AND json_extract(inventory,'$.input_revision')=?", req.Binding, *report.ParsedRevision).Scan(&published); err != nil {
				return nil, err
			}
			report.Outcome, report.Exit = "cancelled", 130
			report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: "cancelled", Code: "interrupted", Message: "owner stopped after parsed commit; model work remains retryable"})
			if err := saveRun(db, root, original, report); err != nil {
				return nil, err
			}
			if latest == string(*report.ParsedRevision) && published == 0 {
				inventory, err := loadInventory(db, report.Identity.Binding, *report.ParsedRevision)
				if err != nil {
					return nil, err
				}
				recovered = &inventory
			}
			continue
		}
		stage := filepath.Join(root, "staging", string(report.Identity.Attempt))
		revision := parsedRevision(report.Identity)
		if _, err := os.Lstat(stage); os.IsNotExist(err) {
			stage = filepath.Join(root, "parsed", string(revision))
		}
		markerPath, err := confined(stage, "completed.json")
		if os.IsNotExist(err) {
			report.Outcome, report.Exit = "cancelled", 130
			report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: "cancelled", Code: "interrupted", Message: "owner stopped before durable collection completion"})
			if err := saveRun(db, root, original, report); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		marker, err := os.ReadFile(markerPath)
		if err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		var completion collectionCompletion
		if err := decodeWire(marker, &completion); err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		readEvidence := func(name, expected string) ([]byte, error) {
			path, err := confined(stage, name)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(path)
			if err == nil && hash(data) != expected {
				err = fmt.Errorf("changed completed %s", name)
			}
			return data, err
		}
		contextBytes, err := readEvidence("context.json", completion.ContextSHA256)
		if err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		if _, err := readEvidence("receipt.json", completion.ReceiptSHA256); err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		var expected plugins.Request
		if err := decodeWire(contextBytes, &expected); err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		if !reflect.DeepEqual(expected.Identity, report.Identity) || expected.Input.Locator != original.Source {
			return recoveryFailure(db, root, original, report, errors.New("interrupted collection identity mismatch"))
		}
		expected.Stage, expected.Source = stage, filepath.Join(stage, "input.json")
		verified, err := verifyReceipt(stage, expected)
		if err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		var inventory committedInventory
		if stage == filepath.Join(root, "parsed", string(revision)) {
			inventory, err = commitPlaced(db, root, placedReceipt{revision: revision, receipt: relocatedReceipt(verified.receipt, revision)}, item.prior)
		} else {
			inventory, err = commitParsed(db, root, verified, item.prior)
		}
		if err != nil {
			return recoveryFailure(db, root, original, report, err)
		}
		recovered = &inventory
	}
	return recovered, nil
}

func recoveryFailure(db *sql.DB, root string, req Request, report contracts.RunReport, cause error) (*committedInventory, error) {
	report.Outcome = "acquisition-failed"
	if report.AcquisitionComplete {
		report.Outcome = "normalization-failed"
	}
	cause = finishFailure(context.Background(), &report, failure(4, "integrity", cause))
	if err := saveRun(db, root, req, report); err != nil {
		return nil, errors.Join(cause, err)
	}
	return nil, cause
}

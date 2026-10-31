package run

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"batz/internal/contracts"
)

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
	if version < 0 || version > 2 {
		db.Close()
		return nil, fmt.Errorf("unsupported metadata version %d", version)
	}
	_, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=1000; CREATE TABLE IF NOT EXISTS bindings(id TEXT PRIMARY KEY,selection TEXT NOT NULL,parsed TEXT,normalized TEXT); CREATE TABLE IF NOT EXISTS parsed(seq INTEGER PRIMARY KEY AUTOINCREMENT,revision TEXT UNIQUE NOT NULL,binding TEXT NOT NULL,receipt BLOB NOT NULL); CREATE TABLE IF NOT EXISTS normalized(revision TEXT PRIMARY KEY,binding TEXT NOT NULL,inventory BLOB NOT NULL); CREATE UNIQUE INDEX IF NOT EXISTS parsed_batch ON parsed(binding,json_extract(receipt,'$.identity.batch_id')); CREATE TABLE IF NOT EXISTS runs(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT UNIQUE NOT NULL,binding TEXT NOT NULL,request BLOB NOT NULL,report BLOB NOT NULL,prior_parsed TEXT); PRAGMA user_version=2")
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type placedReceipt struct {
	revision contracts.RevisionID
	receipt  contracts.CollectionReceipt
}

func parsedRevision(identity contracts.Identity) contracts.RevisionID {
	return contracts.RevisionID(hash(encode([]any{identity.Binding, identity.Batch})))
}

func relocatedReceipt(receipt contracts.CollectionReceipt, revision contracts.RevisionID) contracts.CollectionReceipt {
	relocate := func(files []contracts.FileRef) []contracts.FileRef {
		out := append([]contracts.FileRef{}, files...)
		for i := range out {
			out[i].Path = filepath.ToSlash(filepath.Join("parsed", string(revision), out[i].Path))
		}
		return out
	}
	receipt.ParsedFiles = relocate(receipt.ParsedFiles)
	receipt.RejectFiles = relocate(receipt.RejectFiles)
	receipt.SourceFiles = relocate(receipt.SourceFiles)
	return receipt
}

func existingParsed(db *sql.DB, root string, verified verifiedReceipt) (committedInventory, bool, error) {
	var revision contracts.RevisionID
	var raw []byte
	err := db.QueryRow("SELECT revision,receipt FROM parsed WHERE binding=? AND json_extract(receipt,'$.identity.batch_id')=?", verified.receipt.Identity.Binding, verified.receipt.Identity.Batch).Scan(&revision, &raw)
	if err == sql.ErrNoRows {
		return committedInventory{}, false, nil
	}
	if err != nil {
		return committedInventory{}, false, err
	}
	var prior contracts.CollectionReceipt
	if err := json.Unmarshal(raw, &prior); err != nil {
		return committedInventory{}, false, err
	}
	if !reflect.DeepEqual(prior, relocatedReceipt(verified.receipt, revision)) {
		return committedInventory{}, false, errors.New("conflicting receipt for committed batch")
	}
	inventory, err := loadInventory(db, verified.receipt.Identity.Binding, revision)
	if err == nil {
		err = verifyInventory(root, inventory)
	}
	return inventory, true, err
}

func placeReceipt(root string, verified verifiedReceipt) (placedReceipt, error) {
	if verified.stage == "" {
		return placedReceipt{}, errors.New("unverified receipt")
	}
	// A batch has a stable destination across delivery/restart, independently of
	// an invocation's random run ID. Contents are always verified before reuse.
	revision := parsedRevision(verified.receipt.Identity)
	dest := filepath.Join(root, "parsed", string(revision))
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return placedReceipt{}, err
	}
	if err := os.Rename(verified.stage, dest); err != nil {
		return placedReceipt{}, err
	}
	if err := syncDir(filepath.Dir(dest)); err != nil {
		return placedReceipt{}, err
	}
	return placedReceipt{revision: revision, receipt: relocatedReceipt(verified.receipt, revision)}, nil
}

func commitParsed(db *sql.DB, root string, verified verifiedReceipt, prior sql.NullString) (committedInventory, error) {
	return publishParsed(db, root, verified, prior, nil)
}

func publishParsed(db *sql.DB, root string, verified verifiedReceipt, prior sql.NullString, checkpoint func(string)) (committedInventory, error) {
	if inventory, exists, err := existingParsed(db, root, verified); exists || err != nil {
		return inventory, err
	}
	placed, err := placeReceipt(root, verified)
	if err != nil {
		return committedInventory{}, err
	}
	checkpointAt(checkpoint, "after-placement")
	checkpointAt(checkpoint, "before-parsed-commit")
	return commitPlaced(db, root, placed, prior)
}

func commitPlaced(db *sql.DB, root string, placed placedReceipt, prior sql.NullString) (committedInventory, error) {
	receipt, revision := placed.receipt, placed.revision
	if err := verifyInventory(root, committedInventory{receipts: []contracts.CollectionReceipt{receipt}}); err != nil {
		return committedInventory{}, err
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
		return committedInventory{}, errors.New("parsed revision compare-and-swap failed")
	}
	inventory, err := loadInventory(tx, receipt.Identity.Binding, revision)
	if err != nil {
		return committedInventory{}, err
	}
	var raw []byte
	if err = tx.QueryRow("SELECT report FROM runs WHERE id=?", receipt.Identity.Run).Scan(&raw); err != sql.ErrNoRows {
		if err != nil {
			return committedInventory{}, err
		}
		var report contracts.RunReport
		if err = json.Unmarshal(raw, &report); err != nil {
			return committedInventory{}, err
		}
		applyInventory(&report, inventory)
		if _, err = tx.Exec("UPDATE runs SET report=? WHERE id=?", encode(report), receipt.Identity.Run); err != nil {
			return committedInventory{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return committedInventory{}, err
	}
	return inventory, nil
}

type inventoryReader interface {
	Query(string, ...any) (*sql.Rows, error)
}

func loadInventory(db inventoryReader, binding contracts.BindingID, revision contracts.RevisionID) (committedInventory, error) {
	rows, err := db.Query("SELECT revision,receipt FROM parsed WHERE binding=? AND seq <= (SELECT seq FROM parsed WHERE revision=? AND binding=?) ORDER BY seq", binding, revision, binding)
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
	inventory.digest = hash(encode(map[string]any{"binding_id": binding, "input_revision": revision, "files": files}))
	return inventory, nil
}

func verifyInventory(root string, inventory committedInventory) error {
	if len(inventory.receipts) == 0 {
		return failure(4, "integrity", fmt.Errorf("missing committed inventory %s", inventory.revision))
	}
	for _, receipt := range inventory.receipts {
		for _, files := range [][]contracts.FileRef{receipt.ParsedFiles, receipt.SourceFiles, receipt.RejectFiles} {
			for _, file := range files {
				if err := verifyFile(root, file); err != nil {
					return failure(4, "integrity", err)
				}
			}
		}
	}
	return nil
}

func commitNormalized(db *sql.DB, published contracts.PublishedInventory, report contracts.RunReport, priorNormalized sql.NullString) error {
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("INSERT INTO normalized(revision,binding,inventory) VALUES (?,?,?)", published.Revision, published.Identity.Binding, encode(published)); e != nil {
		return e
	}
	result, e := tx.Exec("UPDATE bindings SET normalized=? WHERE id=? AND parsed=? AND normalized IS ?", published.Revision, published.Identity.Binding, published.InputRevision, priorNormalized)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		return failure(4, "integrity", errors.New("normalized compare-and-swap failed"))
	}
	result, e = tx.Exec("UPDATE runs SET report=? WHERE id=?", encode(report), report.Identity.Run)
	if e != nil {
		return e
	}
	n, e = result.RowsAffected()
	if e != nil || n != 1 {
		return errors.New("missing run for normalized publication")
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	return nil
}

package run

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"batz/internal/contracts"
)

// snapshotSource never treats a path or timestamp as content identity. The
// collector reads a second snapshot and compares it with this frozen digest.
func snapshotSource(path string, byteLimit int64) (contracts.InputRef, []byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return contracts.InputRef{}, nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > byteLimit {
		return contracts.InputRef{}, nil, errors.New("source must be a regular file within the byte budget; symlinks are refused")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return contracts.InputRef{}, nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, byteLimit+1))
	if err != nil {
		return contracts.InputRef{}, nil, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return contracts.InputRef{}, nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		return contracts.InputRef{}, nil, err
	}
	if int64(len(data)) > byteLimit || !os.SameFile(before, opened) || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || opened.Size() != int64(len(data)) {
		return contracts.InputRef{}, nil, errors.New("source changed during snapshot")
	}
	input := contracts.InputRef{Locator: path, SHA256: hash(data), Bytes: int64(len(data))}
	input.ID = hash(encode([]string{input.Locator, input.SHA256}))
	return input, data, nil
}

func freezeSelection(source, framing string, limits contracts.Limits) ([]contracts.InputRef, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	paths := []string{source}
	if info.IsDir() {
		entries, err := os.ReadDir(source)
		if err != nil {
			return nil, err
		}
		paths = nil
		extension := ".json"
		if framing == "jsonl" {
			extension = ".jsonl"
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), extension) {
				paths = append(paths, filepath.Join(source, entry.Name()))
			}
		}
	}
	if int64(len(paths)) > limits.InputFiles {
		return nil, fmt.Errorf("selected input file limit exceeded: %d", limits.InputFiles)
	}
	inputs := make([]contracts.InputRef, 0, len(paths))
	remaining := limits.InputBytes
	for _, path := range paths {
		input, _, err := snapshotSource(path, remaining)
		if err != nil {
			return nil, fmt.Errorf("select %s: %w", path, err)
		}
		remaining -= input.Bytes
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func classifySelection(db *sql.DB, binding contracts.BindingID, selected []contracts.InputRef, replay bool) (contracts.InputAccounting, error) {
	accounting := contracts.InputAccounting{New: []contracts.InputRef{}, Changed: []contracts.InputRef{}, Skipped: []contracts.InputRef{}, Replayed: []contracts.InputRef{}, Acquired: []contracts.InputRef{}}
	known := map[string]map[string]bool{}
	rows, err := db.Query("SELECT receipt FROM parsed WHERE binding=?", binding)
	if err != nil {
		return accounting, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return accounting, err
		}
		var receipt contracts.CollectionReceipt
		if err := json.Unmarshal(raw, &receipt); err != nil {
			return accounting, err
		}
		for _, input := range receipt.Inputs {
			if known[input.Locator] == nil {
				known[input.Locator] = map[string]bool{}
			}
			known[input.Locator][input.SHA256] = true
		}
	}
	if err := rows.Err(); err != nil {
		return accounting, err
	}
	for _, input := range selected {
		switch {
		case replay:
			accounting.Replayed = append(accounting.Replayed, input)
		case known[input.Locator][input.SHA256]:
			accounting.Skipped = append(accounting.Skipped, input)
		case known[input.Locator] != nil:
			accounting.Changed = append(accounting.Changed, input)
		default:
			accounting.New = append(accounting.New, input)
		}
	}
	return accounting, nil
}

func checkBindingContract(db *sql.DB, req Request) error {
	var raw []byte
	err := db.QueryRow("SELECT request FROM runs WHERE binding=? ORDER BY seq LIMIT 1", req.Binding).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var original Request
	if err := json.Unmarshal(raw, &original); err != nil {
		return err
	}
	if original.Contract.Pipeline != req.Contract.Pipeline || original.Contract.Version != req.Contract.Version || original.Contract.Framing != req.Contract.Framing {
		return failure(2, "invalid-input", errors.New("incompatible binding source/version; select a new binding"))
	}
	return nil
}

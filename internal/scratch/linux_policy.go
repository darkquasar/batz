package scratch

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// The launcher is part of the admitted Go executable, not a caller-selected
// script. Python only applies the kernel policy and execs the supervised child.
//
//go:embed linux_sandbox.py
var linuxLauncher string

type linuxPolicy struct {
	Root   string   `json:"root"`
	Reads  []string `json:"reads"`
	Python string   `json:"python"`
}

func linuxProfile(root string, reads []string) (string, error) {
	private, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	policy := linuxPolicy{Root: private}
	for _, path := range reads {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", err
		}
		policy.Reads = append(policy.Reads, resolved)
		if filepath.Base(absolute) == "venv" {
			python := filepath.Join(absolute, "bin/python")
			if info, err := os.Stat(python); err == nil && info.Mode().IsRegular() {
				policy.Python = python
			}
		}
	}
	if policy.Python == "" || !filepath.IsAbs(policy.Root) {
		return "", errors.New("Linux sandbox requires an admitted private interpreter and absolute scratch root")
	}
	data, err := json.Marshal(policy)
	return string(data), err
}

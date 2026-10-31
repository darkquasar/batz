package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type runtimeIntegrityError struct {
	cause error
}

func (e *runtimeIntegrityError) Error() string { return e.cause.Error() }
func (e *runtimeIntegrityError) Unwrap() error { return e.cause }

func verifyRuntime(root string) error {
	return verifyRuntimeArtifacts(root, "collector", "python")
}

func verifyRuntimeArtifacts(root string, names ...string) error {
	data, err := os.ReadFile(filepath.Join(root, "runtime/dependencies.lock.json"))
	if err != nil {
		return fmt.Errorf("missing runtime lock: %w", err)
	}
	var lock struct {
		Version  string                                              `json:"version"`
		Binaries map[string]map[string]struct{ Path, SHA256 string } `json:"installed_binaries"`
	}
	if err = json.Unmarshal(data, &lock); err != nil {
		return &runtimeIntegrityError{cause: fmt.Errorf("invalid runtime lock: %w", err)}
	}
	if lock.Version != "batz-runtime-lock/v0alpha1" {
		return &runtimeIntegrityError{cause: errors.New("unsupported runtime lock")}
	}
	target := runtime.GOOS + "/" + runtime.GOARCH
	for _, name := range names {
		artifact, ok := lock.Binaries[target][name]
		if !ok {
			return fmt.Errorf("%s has no verified installation for %s", name, target)
		}
		expectedPath := "runtime/.local/batz-collector"
		if name == "python" {
			expectedPath = "runtime/.local/venv/bin/python"
		}
		if artifact.Path != expectedPath {
			return &runtimeIntegrityError{cause: fmt.Errorf("%s has unexpected runtime path", name)}
		}
		b, err := os.ReadFile(filepath.Join(root, artifact.Path))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != artifact.SHA256 {
			return &runtimeIntegrityError{cause: fmt.Errorf("%s dependency-integrity mismatch", name)}
		}
	}
	return nil
}

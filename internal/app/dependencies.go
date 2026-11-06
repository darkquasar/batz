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
	if err := verifyReleaseContent(root); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "runtime/dependencies.lock.json"))
	if err != nil {
		return fmt.Errorf("missing runtime lock: %w", err)
	}
	var lock struct {
		Version  string                                              `json:"version"`
		Binaries map[string]map[string]struct{ Path, SHA256 string } `json:"installed_binaries"`
		Python   struct {
			Version      string            `json:"version"`
			BinarySHA256 map[string]string `json:"binary_sha256"`
		} `json:"python"`
		UV struct {
			Version string `json:"version"`
		} `json:"uv"`
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
		if name == "collector" {
			// The archive inventory binds the collector built from this source;
			// legacy checkout hashes do not describe newly built release binaries.
			if b, e := os.ReadFile(filepath.Join(root, "release-content.json")); e == nil {
				var content struct {
					Files map[string]string `json:"files"`
				}
				if e := json.Unmarshal(b, &content); e != nil {
					return &runtimeIntegrityError{cause: e}
				}
				artifact.Path = "runtime/.local/batz-collector"
				artifact.SHA256, ok = content.Files[artifact.Path]
			} else if _, e := os.Stat(filepath.Join(root, ".git")); e == nil {
				if b, e := os.ReadFile(filepath.Join(root, "runtime/.local/installation.json")); e == nil {
					var receipt struct {
						Schema                string                                   `json:"schema"`
						Target                string                                   `json:"target"`
						RuntimeLockSHA256     string                                   `json:"runtime_lock_sha256"`
						CollectorSourceSHA256 string                                   `json:"collector_source_sha256"`
						Binaries              map[string]struct{ Path, SHA256 string } `json:"binaries"`
					}
					if e := json.Unmarshal(b, &receipt); e != nil {
						return &runtimeIntegrityError{cause: e}
					}
					if receipt.CollectorSourceSHA256 != "" {
						source, e := collectorSourceSHA256(root)
						if e != nil {
							return &runtimeIntegrityError{cause: e}
						}
						sum := sha256.Sum256(data)
						if receipt.Schema != "batz-installation/v0alpha1" || receipt.Target != target || receipt.RuntimeLockSHA256 != hex.EncodeToString(sum[:]) || receipt.CollectorSourceSHA256 != source {
							return &runtimeIntegrityError{cause: errors.New("collector installation receipt does not match current source; repeat mise install")}
						}
						artifact, ok = receipt.Binaries[name]
					}
				}
			}
		}
		if name == "python" && lock.Python.BinarySHA256 != nil {
			expected := lock.Python.BinarySHA256[target]
			if decoded, e := hex.DecodeString(expected); e != nil || len(decoded) != sha256.Size {
				return &runtimeIntegrityError{cause: fmt.Errorf("python has no immutable digest for %s", target)}
			}
			b, e := os.ReadFile(filepath.Join(root, "runtime/.local/installation.json"))
			if e != nil {
				return fmt.Errorf("python installation receipt: %w", e)
			}
			var receipt struct {
				Schema            string                                   `json:"schema"`
				Target            string                                   `json:"target"`
				RuntimeLockSHA256 string                                   `json:"runtime_lock_sha256"`
				Binaries          map[string]struct{ Path, SHA256 string } `json:"binaries"`
			}
			if e := json.Unmarshal(b, &receipt); e != nil {
				return &runtimeIntegrityError{cause: fmt.Errorf("python installation receipt: %w", e)}
			}
			sum := sha256.Sum256(data)
			artifact, ok = receipt.Binaries[name]
			if receipt.Schema != "batz-installation/v0alpha1" || receipt.Target != target || receipt.RuntimeLockSHA256 != hex.EncodeToString(sum[:]) || !ok || artifact.SHA256 != expected {
				return &runtimeIntegrityError{cause: fmt.Errorf("python installation receipt does not match immutable runtime lock")}
			}
			if err := verifyPythonEnvironment(root, lock.Python.Version, lock.UV.Version); err != nil {
				return &runtimeIntegrityError{cause: err}
			}
		}
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
		digest, err := artifactSHA256(filepath.Join(root, artifact.Path))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if digest != artifact.SHA256 {
			return &runtimeIntegrityError{cause: fmt.Errorf("%s dependency-integrity mismatch", name)}
		}
	}
	return nil
}

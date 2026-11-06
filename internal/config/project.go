package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ProjectRoot selects explicit content first, otherwise the containing checkout.
// executable is the running program's path, supplied by its caller.
func ProjectRoot(explicit, start, executable string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	candidate, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		candidate = parent
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	if filepath.Base(resolved) != "batz" || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return "", fmt.Errorf("no content checkout or installed root; use --project")
	}
	return filepath.Dir(filepath.Dir(resolved)), nil
}

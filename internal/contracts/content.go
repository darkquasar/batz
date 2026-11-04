package contracts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProductionProjectPath admits SQLMesh execution inputs. Test fixtures, caches,
// logs and unrelated contributor metadata never enter a production snapshot.
func ProductionProjectPath(relative string) bool {
	if relative == "." || relative == "config.yaml" {
		return true
	}
	first, _, _ := strings.Cut(relative, "/")
	switch first {
	case "models", "audits", "macros", "seeds":
		return true
	default:
		return false
	}
}

// ValidateProjectContent checks only native loader inputs, before discovery.
// Implementation tooling elsewhere in a root project is not SQLMesh content.
func ValidateProjectContent(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		first, _, _ := strings.Cut(rel, "/")
		switch first {
		case "config.py", "config.yml", "external_models.yaml", "external_models.yml", "metrics", "signals", "materializations", "linter", "external_models", "schema.yaml", "hooks":
			return fmt.Errorf("unsupported-content: native execution input %s", rel)
		}
		if !ProductionProjectPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in native content: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		ext := filepath.Ext(rel)
		if ext != ".sql" && ext != ".yaml" && ext != ".yml" && ext != ".csv" {
			return fmt.Errorf("unsupported-content: %s", rel)
		}
		return nil
	})
}

// NativeScope is private discovery output, admitted against one content snapshot.
type NativeScope struct {
	Roots   []string          `json:"roots"`
	Models  []string          `json:"models"`
	Sources map[string]string `json:"sources"`
	Globals []string          `json:"globals"`
}

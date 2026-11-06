package app

import (
	"batz/internal/buildinfo"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// verifyReleaseContent binds installed native inputs and both executables before
// any framework loads them. Source checkouts retain their existing development
// receipts; packaged builds always require an inventory.
func verifyReleaseContent(root string) error {
	identity := buildinfo.Current()
	data, err := os.ReadFile(filepath.Join(root, "release-content.json"))
	if os.IsNotExist(err) && identity.SourceSHA == "unknown" {
		return nil
	}
	failure := func(err error) error { return &runtimeIntegrityError{cause: fmt.Errorf("release-content: %w", err)} }
	if err != nil {
		return failure(err)
	}
	var content struct {
		Schema    string            `json:"schema"`
		Version   string            `json:"version"`
		SourceSHA string            `json:"source_sha"`
		Target    string            `json:"target"`
		Files     map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return failure(err)
	}
	if content.Schema != "batz-release-content/v0alpha1" || content.Target != identity.Target || content.Version != identity.Version || content.SourceSHA != identity.SourceSHA {
		return failure(fmt.Errorf("build identity mismatch"))
	}
	for _, name := range []string{"bin/batz", "runtime/.local/batz-collector", "runtime/dependencies.lock.json", "config.yaml", "runtime/sqlmesh/runner.py", "runtime/sqlmesh/catalog.py", "runtime/sqlmesh/inspect_preview.py"} {
		if content.Files[name] == "" {
			return failure(fmt.Errorf("missing inventory entry %s", name))
		}
	}
	for name, expected := range content.Files {
		if name == "." || filepath.ToSlash(filepath.Clean(name)) != name || strings.Contains(name, "\\") || filepath.IsAbs(name) || strings.HasPrefix(name, "../") || name == "release-content.json" || (strings.HasPrefix(name, "runtime/.local/") && name != "runtime/.local/batz-collector") {
			return failure(fmt.Errorf("invalid inventory path %q", name))
		}
		if decoded, err := hex.DecodeString(expected); err != nil || len(decoded) != sha256.Size {
			return failure(fmt.Errorf("invalid digest for %s", name))
		}
		path := root
		for _, part := range strings.Split(name, "/") {
			path = filepath.Join(path, part)
			info, err := os.Lstat(path)
			if err != nil {
				return failure(err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return failure(fmt.Errorf("symlink in %s", name))
			}
		}
		digest, err := artifactSHA256(filepath.Join(root, name))
		if err != nil {
			return failure(err)
		}
		if digest != expected {
			return failure(fmt.Errorf("%s dependency-integrity mismatch", name))
		}
	}
	// An added native model/route is also a changed release, even when all
	// original files remain intact. Environment and receipt writes are excluded.
	for _, dir := range []string{"models", "pipelines", "schemas", "runtime/sqlmesh", "audits", "seeds", "macros"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if os.IsNotExist(err) && path == filepath.Join(root, dir) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if content.Files[filepath.ToSlash(rel)] == "" {
				return fmt.Errorf("unlisted native content %s", rel)
			}
			return nil
		})
		if err != nil {
			return failure(err)
		}
	}
	return nil
}

func artifactSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("non-regular artifact %s", path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

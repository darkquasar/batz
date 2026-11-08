package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A matching interpreter digest does not make a foreign environment trustworthy:
// Python imports site packages relative to the environment surrounding that file.
func verifyPythonEnvironment(root, version, uvVersion string) error {
	local, err := filepath.EvalSymlinks(filepath.Join(root, "runtime/.local"))
	if err != nil {
		return fmt.Errorf("python installation root: %w", err)
	}
	for _, name := range []string{"venv", "python"} {
		info, err := os.Lstat(filepath.Join(local, name))
		if err != nil {
			return fmt.Errorf("python %s: %w", name, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("python %s must be an owned installation directory", name)
		}
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return fmt.Errorf("python runtime lock has invalid version")
	}
	base, err := filepath.EvalSymlinks(filepath.Join(local, "python/bin/python"+strings.Join(parts[:2], ".")))
	if err != nil {
		return fmt.Errorf("python owned base: %w", err)
	}
	baseRoot := filepath.Join(local, "python") + string(filepath.Separator)
	if !strings.HasPrefix(base, baseRoot) {
		return fmt.Errorf("python base escapes owned installation")
	}
	interpreter, err := filepath.EvalSymlinks(filepath.Join(local, "venv/bin/python"))
	if err != nil {
		return fmt.Errorf("python environment interpreter: %w", err)
	}
	if interpreter != base {
		return fmt.Errorf("python environment uses foreign interpreter base")
	}
	data, err := os.ReadFile(filepath.Join(local, "venv/pyvenv.cfg"))
	if err != nil {
		return fmt.Errorf("python environment configuration: %w", err)
	}
	config := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, exists := config[key]; exists {
			return fmt.Errorf("duplicate python environment configuration %s", key)
		}
		config[key] = strings.TrimSpace(value)
	}
	if strings.ToLower(config["include-system-site-packages"]) != "false" || uvVersion == "" || config["uv"] != uvVersion {
		return fmt.Errorf("python environment must be private and uv-created")
	}
	home, err := filepath.EvalSymlinks(config["home"])
	if err != nil || home != filepath.Dir(base) {
		return fmt.Errorf("python environment has foreign home")
	}
	return nil
}

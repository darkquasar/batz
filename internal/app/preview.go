package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"batz/collector/plugins"
	"batz/internal/contracts"
	pipeline "batz/internal/run"
	"batz/internal/scratch"
)

func preview(root, definition, workspace string, manifest contracts.SourcePipeline, example contracts.Example, report *contracts.ExplainReport) (int, error) {
	selectedRoute := manifest.Routes[example.Route]
	if err := verifyRuntime(root); err != nil {
		var integrity *runtimeIntegrityError
		if errors.As(err, &integrity) {
			return 4, err
		}
		return 3, err
	}
	native := resolve(manifest.Routes[example.Route].Input.Config, root, filepath.Dir(definition), workspace)
	source := resolve(example.Input, root, filepath.Dir(definition), workspace)
	models := resolve(manifest.Normalization.Project, root, filepath.Dir(definition), workspace)
	expected := ""
	if example.Expectations != "" {
		expected = resolve(example.Expectations, root, filepath.Dir(definition), workspace)
	}
	for _, path := range []string{native, source, models, expected} {
		if strings.HasPrefix(path, "workspace://") {
			return 3, errors.New("preview assets require an explicitly selected workspace")
		}
	}
	if err := validateNative(native); err != nil {
		return 3, err
	}
	f, err := os.Open(source)
	if err != nil {
		return 3, err
	}
	stat, err := f.Stat()
	f.Close()
	if err != nil {
		return 3, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > pipeline.Limits().InputBytes {
		return 2, fmt.Errorf("example exceeds input budget")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return 3, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != example.SHA256 {
		return 4, fmt.Errorf("example-integrity mismatch")
	}
	records, err := plugins.SourceRecords(data, selectedRoute.Input.Framing)
	if err != nil {
		return 2, err
	}
	if int64(len(records)) > pipeline.Limits().Records {
		return 2, fmt.Errorf("example exceeds row budget")
	}
	if err = admitProject(models); err != nil {
		return 3, err
	}
	if err = admitPreviewConnections(filepath.Join(models, "config.yaml")); err != nil {
		return 3, err
	}
	dir, err := os.MkdirTemp("", "batz-preview-")
	if err != nil {
		return 1, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return 1, err
	}
	// Retain scratch for explicit evidence and troubleshooting; no production root is used.
	report.Runtime["scratch"] = dir
	fixture := filepath.Join(dir, "source.json")
	if err = os.WriteFile(fixture, data, 0600); err != nil {
		return 1, err
	}
	copied := filepath.Join(dir, "project")
	if err = copyProject(models, copied); err != nil {
		return 3, err
	}
	reads := []string{filepath.Join(root, "runtime/.local/python"), filepath.Join(root, "runtime/.local/venv"), filepath.Join(root, "runtime/.local/batz-collector"), filepath.Join(root, "runtime/sqlmesh")}
	profile, err := scratch.Profile(dir, reads)
	if err != nil {
		return 3, err
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 430*time.Second)
	defer cancel()
	python := filepath.Join(root, "runtime/.local/venv/bin/python")
	if err = scratch.Qualify(ctx, profile, python, dir, definition); err != nil {
		return 3, err
	}
	// Configuration is snapshotted into the same read/write boundary.
	configData, err := os.ReadFile(native)
	if err != nil {
		return 3, err
	}
	config := filepath.Join(dir, "collector.yaml")
	if err = os.WriteFile(config, configData, 0600); err != nil {
		return 1, err
	}
	inspection := filepath.Join(dir, "discovery")
	if err = os.MkdirAll(inspection, 0700); err != nil {
		return 1, err
	}
	scope, err := inspectModelScope(ctx, root, copied, inspection, profile, manifest.Execution(example.Route))
	if err != nil {
		return 3, err
	}
	runReport, err := pipeline.Execute(ctx, pipeline.Request{Scope: scope, Contract: manifest.Execution(example.Route), Root: root, Workspace: filepath.Join(dir, "work"), Definition: definition, Route: example.Route, Source: fixture, Binding: "preview", NativeConfig: config, Project: copied, PrivateScratch: dir, SandboxProfile: profile})
	report.Runtime["resources"] = runReport.Resources
	report.Assets[example.Input] = example.SHA256
	report.Runtime["run"] = runReport
	if err != nil {
		return runReport.Exit, err
	}
	if example.Expectations != "" {
		content, e := os.ReadFile(expected)
		if e != nil {
			return 3, e
		}
		digest := sha256.Sum256(content)
		report.Assets[example.Expectations] = hex.EncodeToString(digest[:])
		expected = filepath.Join(dir, "expectations.json")
		if e = os.WriteFile(expected, content, 0600); e != nil {
			return 1, e
		}
	}
	request := map[string]any{"framing": selectedRoute.Input.Framing, "datasets": runReportDatasetNames(manifest.Execution(example.Route)), "source": fixture, "catalog": *runReport.Catalog, "expectations": expected, "counts": runReport.Counts, "result": filepath.Join(dir, "result.json")}
	raw, err := json.Marshal(request)
	if err != nil {
		return 1, err
	}
	requestPath := filepath.Join(dir, "inspect.json")
	if err = os.WriteFile(requestPath, raw, 0600); err != nil {
		return 1, err
	}
	usage, err := pipeline.InspectPrivate(ctx, dir, profile, python, filepath.Join(root, "runtime/sqlmesh/inspect_preview.py"), requestPath)
	report.Runtime["inspection_resources"] = usage
	if err != nil {
		code := 1
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 4 {
			code = 4
		}
		return code, fmt.Errorf("native sample inspection: %w", err)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		return 1, err
	}
	var result struct {
		Samples    []contracts.ExplainSample    `json:"samples"`
		Structures []contracts.ExplainStructure `json:"structures"`
		Runtime    map[string]any               `json:"runtime"`
		Unobserved []string                     `json:"unobserved"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return 4, err
	}
	report.Samples = result.Samples
	report.Structures = result.Structures
	report.Runtime["values"] = result.Runtime
	report.Runtime["unobserved"] = result.Unobserved
	for i := range report.Stages {
		if report.Stages[i].Route != example.Route {
			continue
		}
		report.Stages[i].Basis = "sample-executed"
		report.Stages[i].Reason = "Actual native preview in qualified private scratch"
	}
	return 0, nil
}

func runReportDatasetNames(contract contracts.ExecutionContract) []string {
	names := []string{"parsed", "source_staging"}
	outputs := make([]string, 0, len(contract.Outputs))
	for name := range contract.Outputs {
		outputs = append(outputs, name)
	}
	sort.Strings(outputs)
	return append(names, outputs...)
}

type previewConnection struct {
	Type            string            `json:"type"`
	ConcurrentTasks int               `json:"concurrent_tasks"`
	Extensions      []json.RawMessage `json:"extensions"`
	ConnectorConfig struct {
		Threads     int   `json:"threads"`
		AutoInstall *bool `json:"autoinstall_known_extensions"`
		AutoLoad    *bool `json:"autoload_known_extensions"`
	} `json:"connector_config"`
}

// The first native adapter uses a JSON-compatible SQLMesh configuration. Check
// both DuckDB connections before launch; filesystem/network isolation is still
// enforced independently on the actual children.
func admitPreviewConnections(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read preview connection configuration: %w", err)
	}
	var config struct {
		DefaultGateway string `json:"default_gateway"`
		Gateways       map[string]struct {
			Connection      previewConnection `json:"connection"`
			StateConnection previewConnection `json:"state_connection"`
		} `json:"gateways"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("unsupported preview connection configuration: %w", err)
	}
	gateway, ok := config.Gateways["local"]
	if !ok || len(config.Gateways) != 1 || config.DefaultGateway != "local" {
		return errors.New("offline preview requires the qualified local gateway")
	}
	for _, connection := range []previewConnection{gateway.Connection, gateway.StateConnection} {
		if connection.Type != "duckdb" || connection.ConcurrentTasks < 0 || connection.ConcurrentTasks > 1 || connection.ConnectorConfig.Threads != 1 {
			return errors.New("offline preview requires bounded local DuckDB connections")
		}
		settings := connection.ConnectorConfig
		if len(connection.Extensions) != 0 || settings.AutoInstall == nil || *settings.AutoInstall || settings.AutoLoad == nil || *settings.AutoLoad {
			return errors.New("offline preview requires extension installation and automatic loading to be disabled")
		}
	}
	return nil
}

// templateMarkers are Jinja delimiters and SQLMesh Jinja block markers, matched
// case-insensitively. The scan is deliberately conservative: markers inside SQL
// strings or comments are refused too. It is an admission rule, not a sandbox.
var templateMarkers = [][]byte{
	[]byte("{{"), []byte("{%"), []byte("{#"),
	[]byte("JINJA_QUERY_BEGIN"), []byte("JINJA_STATEMENT_BEGIN"), []byte("JINJA_END"),
}

func admitProject(root string) error {
	if err := validateProjectContent(root); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !contracts.ProductionProjectPath(filepath.ToSlash(rel)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || filepath.Ext(path) == ".csv" {
			return nil
		}
		return admitTemplateFree(root, path)
	})
}

func admitTemplateFree(root, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read normalization content: %w", err)
	}
	content = bytes.ToUpper(content)
	for _, marker := range templateMarkers {
		if !bytes.Contains(content, marker) {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// Name the file only; project content is never echoed.
		return fmt.Errorf("unsupported Jinja templating in normalization content: %s", relative)
	}
	return nil
}
func copyProject(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, relative)
		if !contracts.ProductionProjectPath(filepath.ToSlash(relative)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in native project")
		}
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, content, 0600)
	})
}

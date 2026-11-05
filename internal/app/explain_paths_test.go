package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func workspaceAssetDefinition(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("repo://."), []byte("workspace://models"))
	data = bytes.ReplaceAll(data, []byte("repo://internal/app/testdata/synthetic/collector.yaml"), []byte("workspace://collector.yaml"))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUnresolvedWorkspaceAssetsRemainSymbolic(t *testing.T) {
	root := nativeContent(t)
	definition := workspaceAssetDefinition(t, root)
	// A literal workspace: directory must not become an implicit URI fallback.
	fake := filepath.Join(root, "pipelines/workspace:")
	copyNativeCatalog(t, root, filepath.Join(fake, "models"))
	data, err := os.ReadFile(filepath.Join(root, "internal/app/testdata/synthetic/collector.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fake, "collector.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	caller := t.TempDir()
	copyNativeCatalog(t, root, filepath.Join(caller, "workspace:/models"))
	t.Chdir(caller)
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", definition, "--project", root, "--level", "2", "--format", "json", "--strict"}, &out, &diagnostics)
	var report contracts.ExplainReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 3 || report.Project != "workspace://models" || report.Assets["workspace://models/config.yaml"] != "" || len(report.Runtime) != 0 {
		t.Fatalf("unresolved workspace URI was treated as a filesystem path: %d %s %s", code, &out, &diagnostics)
	}
}

func TestCallerRelativeWorkspaceOverridesTheBindingSelector(t *testing.T) {
	root := nativeContent(t)
	caller := t.TempDir()
	t.Chdir(caller)
	workspace := filepath.Join(caller, "selected")
	data, err := os.ReadFile(filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	definition := filepath.Join(workspace, "source.yaml")
	writeContextFile(t, definition, string(data))
	writeContextFile(t, filepath.Join(workspace, "workspace.yaml"), "apiVersion: batz/v0alpha1\nkind: Workspace\nstorage: {parsed: workspace://parsed}\n")
	binding := filepath.Join(root, "binding.yaml")
	writeContextFile(t, binding, "apiVersion: batz/v0alpha1\nkind: PipelineBinding\nid: example\nworkspace: ./unselected\ndefinition: workspace://source.yaml\nversion: 1\nroute: local_raw\n")
	t.Run("explain", func(t *testing.T) {
		var out, diagnostics bytes.Buffer
		code := app.Execute([]string{"explain", definition, "--project", root, "--binding", binding, "--workspace", "./selected", "--level", "2", "--format", "json", "--strict"}, &out, &diagnostics)
		if code != 0 {
			t.Fatalf("caller-relative workspace rejected: %d %s %s", code, &out, &diagnostics)
		}
		var report contracts.ExplainReport
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		var parsed string
		if err := json.Unmarshal(report.Resolution["storage.parsed"].Value, &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed != filepath.Join(workspace, "parsed") || report.Resolution["storage.parsed"].Source != filepath.Join(workspace, "workspace.yaml") {
			t.Fatalf("workspace values/origins are not caller-relative: %s", &out)
		}
	})
	t.Run("run preflight", func(t *testing.T) {
		var out, diagnostics bytes.Buffer
		code := app.Execute([]string{"run", "--project", root, "--binding", binding, "--workspace", "./selected", "--input", "path=repo://internal/app/testdata/synthetic/events.jsonl"}, &out, &diagnostics)
		if code != 3 || !strings.Contains(diagnostics.String(), "storage configuration") {
			t.Fatalf("definition mismatch hid the selected unsupported capability: %d %s %s", code, &out, &diagnostics)
		}
		if _, err := os.Stat(filepath.Join(workspace, ".batz")); !os.IsNotExist(err) {
			t.Fatalf("preflight created state: %v", err)
		}
	})
}

func TestPreviewRuntimeChecksumMismatchIsAlwaysAnIntegrityFailure(t *testing.T) {
	root := nativeContent(t)
	path := filepath.Join(root, "runtime/dependencies.lock.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	artifact := lock
	for _, key := range []string{"installed_binaries", runtime.GOOS + "/" + runtime.GOARCH, "collector"} {
		nested, ok := artifact[key].(map[string]any)
		if !ok {
			t.Fatalf("missing installed receipt at %s", key)
		}
		artifact = nested
	}
	artifact["sha256"] = strings.Repeat("0", 64)
	changed, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
	}{
		{name: "default"},
		{name: "strict", args: []string{"--strict"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"explain", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--level", "3", "--format", "json"}
			args = append(args, tt.args...)
			var out, diagnostics bytes.Buffer
			code := app.Execute(args, &out, &diagnostics)
			var report contracts.ExplainReport
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if code != 4 || report.Runtime["run"] != nil || report.Runtime["scratch"] != nil || len(report.Diagnostics) == 0 {
				t.Fatalf("checksum mismatch was treated as optional unavailability: %d %s %s", code, &out, &diagnostics)
			}
			if report.Diagnostics[len(report.Diagnostics)-1].Category != "integrity" {
				t.Fatalf("wrong checksum diagnostic: %s", &out)
			}
		})
	}
}

func TestDeclaredOutputDoesNotInventNativeFilesOrStructures(t *testing.T) {
	root := nativeContent(t)
	definition := filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml")
	data, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("findings: probe.findings"), []byte("authentication: probe.authentication"))
	if err := os.WriteFile(definition, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", definition, "--project", root, "--level", "2", "--format", "json"}, &out, &diagnostics)
	var report contracts.ExplainReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(report.Samples) != 0 || len(report.Structures) != 0 || len(report.Runtime) != 0 {
		t.Fatalf("fabricated runtime shape: %d %s %s", code, &out, &diagnostics)
	}
	found := false
	for _, stage := range report.Stages {
		if strings.HasSuffix(stage.ID, "/findings") {
			t.Fatalf("invented output: %+v", stage)
		}
		if strings.HasSuffix(stage.ID, "/authentication") {
			found = stage.Basis == "declared" && len(stage.Native) == 0
		}
	}
	if !found {
		t.Fatalf("declared output absent: %s", &out)
	}
}

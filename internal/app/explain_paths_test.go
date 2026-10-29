package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"batz/internal/app"
	"batz/internal/contracts"
)

func workspaceAssetDefinition(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "pipelines/aws-cloudtrail.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("repo://normalization"), []byte("workspace://models"))
	data = bytes.ReplaceAll(data, []byte("repo://collector/configs/cloudtrail/local-records.yaml"), []byte("workspace://collector.yaml"))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreviewUsesAssetsFromTheExplicitWorkspace(t *testing.T) {
	root := previewContent(t)
	definition := workspaceAssetDefinition(t, root)
	declaration, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}
	declaration = bytes.Replace(declaration, []byte("explain:\n"), []byte(`  unsupported:
    input:
      engine: bento
      config: workspace://collector.yaml
      params: {}
    output:
      parsed: {format: parquet}
      normalized: {models: {authentication: cloudtrail.authentication}}
explain:
`), 1)
	if err := os.WriteFile(definition, declaration, 0600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	copyTree(t, filepath.Join(root, "normalization"), filepath.Join(workspace, "models"))
	data, err := os.ReadFile(filepath.Join(root, "collector/configs/cloudtrail/local-records.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "collector.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"explain", definition, "--project", root, "--workspace", workspace, "--level", "3", "--format", "json"}, &out, &diagnostics)
	var report contracts.ExplainReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(report.Samples) != 5 || report.Assets["workspace://collector.yaml"] == "" || report.Assets["workspace://models/config.yaml"] == "" {
		t.Fatalf("selected workspace assets were not used: %d %s %s", code, &out, &diagnostics)
	}
	for _, stage := range report.Stages {
		if stage.Route != "local_raw" && stage.Basis == "sample-executed" {
			t.Fatalf("unexecuted route acquired another route's sample evidence: %+v", stage)
		}
	}
}

func TestUnresolvedWorkspaceAssetsRemainSymbolic(t *testing.T) {
	root := previewContent(t)
	definition := workspaceAssetDefinition(t, root)
	// A literal workspace: directory must not become an implicit URI fallback.
	fake := filepath.Join(root, "pipelines/workspace:")
	copyTree(t, filepath.Join(root, "normalization"), filepath.Join(fake, "models"))
	data, err := os.ReadFile(filepath.Join(root, "collector/configs/cloudtrail/local-records.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fake, "collector.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	caller := t.TempDir()
	copyTree(t, filepath.Join(root, "normalization"), filepath.Join(caller, "workspace:/models"))
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
	data, err := os.ReadFile(filepath.Join(root, "pipelines/aws-cloudtrail.yaml"))
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
		code := app.Execute([]string{"run", "--project", root, "--binding", binding, "--workspace", "./selected", "--input", "path=repo://fixtures/cloudtrail/api-read.json"}, &out, &diagnostics)
		if code != 3 || !strings.Contains(diagnostics.String(), "storage configuration") {
			t.Fatalf("definition mismatch hid the selected unsupported capability: %d %s %s", code, &out, &diagnostics)
		}
		if _, err := os.Stat(filepath.Join(workspace, ".batz")); !os.IsNotExist(err) {
			t.Fatalf("preflight created state: %v", err)
		}
	})
}

func TestPreviewRuntimeChecksumMismatchIsAlwaysAnIntegrityFailure(t *testing.T) {
	root := previewContent(t)
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
			args := []string{"explain", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--level", "3", "--format", "json"}
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

func TestUnsupportedDeclaredOutputDoesNotInventTheTracerGraph(t *testing.T) {
	root := previewContent(t)
	definition := filepath.Join(root, "pipelines/aws-cloudtrail.yaml")
	data, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("api_activity: cloudtrail.api_activity"), []byte("authentication: cloudtrail.authentication"))
	if err := os.WriteFile(definition, data, 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "level 2", args: []string{"--level", "2"}},
		{name: "strict level 2", args: []string{"--level", "2", "--strict"}, want: 3},
		{name: "level 3", args: []string{"--level", "3"}},
		{name: "strict level 3", args: []string{"--level", "3", "--strict"}, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"explain", definition, "--project", root, "--format", "json"}
			args = append(args, tt.args...)
			var out, diagnostics bytes.Buffer
			code := app.Execute(args, &out, &diagnostics)
			var report contracts.ExplainReport
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if code != tt.want || len(report.Samples) != 0 || len(report.Runtime) != 0 {
				t.Fatalf("unknown topology was executed or not disclosed: %d %s %s", code, &out, &diagnostics)
			}
			found := false
			for _, stage := range report.Stages {
				if strings.HasSuffix(stage.ID, "/api_activity") || strings.HasSuffix(stage.ID, "/source_staging") {
					t.Fatalf("invented tracer stage: %+v", stage)
				}
				if strings.HasSuffix(stage.ID, "/authentication") {
					found = stage.Basis == "unavailable" && len(stage.Native) == 0 && strings.Contains(stage.Reason, "cloudtrail.authentication")
				}
			}
			if !found || len(report.Diagnostics) == 0 {
				t.Fatalf("declared output was not retained with honest unavailable evidence: %s", &out)
			}
		})
	}
}

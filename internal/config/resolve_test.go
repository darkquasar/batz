package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"batz/internal/config"
	"batz/internal/contracts"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestPathsRetainDeclaringOriginAcrossLayers(t *testing.T) {
	project := t.TempDir()
	workspace := t.TempDir()
	write(t, filepath.Join(project, "workspace.yaml"), "apiVersion: batz/v0alpha1\nkind: Workspace\nnormalization:\n  project: models\n")
	write(t, filepath.Join(workspace, "workspace.yaml"), "apiVersion: batz/v0alpha1\nkind: Workspace\nstorage:\n  parsed: workspace://parsed\n")
	c := config.Context{Project: project, Workspace: workspace}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err = config.Resolve(c, s, contracts.SourcePipeline{})
	if err != nil {
		t.Fatal(err)
	}
	var actual string
	if err = json.Unmarshal(s.Origins["normalization.project"].Value, &actual); err != nil {
		t.Fatal(err)
	}
	if actual != filepath.Join(project, "models") {
		t.Fatalf("lost declaring path: %s", actual)
	}
	if s.Origins["normalization.project"].Source != filepath.Join(project, "workspace.yaml") {
		t.Fatal("lost origin")
	}
}

func TestSoleRouteResolvesItsDefaultsWithoutAnExplicitSelector(t *testing.T) {
	c := config.Context{Project: t.TempDir()}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	p := contracts.SourcePipeline{Routes: map[string]contracts.Route{
		"local": {Input: contracts.RouteInput{Params: map[string]contracts.Parameter{
			"name": {Type: "string", Default: "example"},
		}}},
	}}
	s, err = config.Resolve(c, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Route != "local" || string(s.Origins["input.name"].Value) != `"example"` {
		t.Fatalf("sole route and defaults unresolved: %+v", s)
	}
}

func TestSixLayerPrecedenceAndAtomicCredentialReplacement(t *testing.T) {
	project := t.TempDir()
	workspace := t.TempDir()
	dir := t.TempDir()
	write(t, filepath.Join(project, "workspace.yaml"), "apiVersion: batz/v0alpha1\nkind: Workspace\ncredentials:\n  aws: {provider: file, path: old, format: json}\npipeline_defaults:\n  cloudtrail:\n    raw:\n      input: {bucket: project, prefixes: [old, retained]}\n")
	write(t, filepath.Join(workspace, "workspace.yaml"), "apiVersion: batz/v0alpha1\nkind: Workspace\npipeline_defaults:\n  cloudtrail:\n    raw:\n      input: {bucket: workspace}\n")
	binding := filepath.Join(dir, "binding.yaml")
	write(t, binding, "apiVersion: batz/v0alpha1\nkind: PipelineBinding\nid: chosen\ndefinition: repo://source.yaml\nversion: 1\nroute: raw\ncredentials:\n  aws: {provider: env, variable: AWS_KEY}\npipeline_defaults:\n  cloudtrail:\n    raw:\n      input: {bucket: shared}\ninput: {bucket: binding, prefixes: [new]}\n")
	p := contracts.SourcePipeline{Name: "cloudtrail", Routes: map[string]contracts.Route{"raw": {Input: contracts.RouteInput{Params: map[string]contracts.Parameter{"bucket": {Type: "string", Default: "route"}, "prefixes": {Type: "string-list"}}}}}}
	c := config.Context{Project: project, Workspace: workspace, Binding: binding, Input: map[string]string{"bucket": "cli"}}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err = config.Resolve(c, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if string(s.Origins["input.bucket"].Value) != `"cli"` || s.Origins["input.bucket"].Source != "cli" {
		t.Fatalf("CLI precedence: %v", s.Origins)
	}
	if string(s.Origins["input.prefixes"].Value) != `["new"]` {
		t.Fatal("lists merged")
	}
	credential := s.Values["credentials"].(map[string]any)["aws"].(map[string]any)
	if len(credential) != 2 || credential["provider"] != "env" {
		t.Fatalf("provider failed atomic replacement: %v", credential)
	}
}

func TestExplicitNullDeletesAndBlankIsRejected(t *testing.T) {
	project := t.TempDir()
	binding := filepath.Join(t.TempDir(), "binding.yaml")
	p := contracts.SourcePipeline{Name: "source", Routes: map[string]contracts.Route{"raw": {Input: contracts.RouteInput{Params: map[string]contracts.Parameter{"prefix": {Type: "string", Required: true, Default: "old"}}}}}}
	write(t, binding, "apiVersion: batz/v0alpha1\nkind: PipelineBinding\nid: demo\ndefinition: repo://source.yaml\nversion: 1\nroute: raw\ninput: {prefix: null}\n")
	c := config.Context{Project: project, Binding: binding}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err = config.Resolve(c, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Origins["input.prefix"]; ok {
		t.Fatal("explicit null inherited default")
	}
	write(t, binding, "apiVersion: batz/v0alpha1\nkind: PipelineBinding\nid: demo\ndefinition: repo://source.yaml\nversion: 1\nroute: raw\ninput:\n  prefix:\n")
	if _, err = config.Select(c); err == nil {
		t.Fatal("blank value accepted")
	}
}

func TestExplicitCaseSelectsOnlyRequestedBinding(t *testing.T) {
	project := t.TempDir()
	caseFile := filepath.Join(t.TempDir(), "case.yaml")
	write(t, caseFile, "apiVersion: batz/v0alpha1\nkind: Case\nid: case-a\nworkspace: work\ncredentials:\n  aws: {provider: aws_profile, profile: case-readonly}\npipelines:\n  - {id: chosen, definition: repo://source.yaml, version: 1, route: raw, input: {bucket: case-evidence}}\n  - {id: other, definition: repo://other.yaml, version: 1, route: raw}\n")
	c := config.Context{Project: project, Case: caseFile, BindingID: "chosen"}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	p := contracts.SourcePipeline{Name: "source", Routes: map[string]contracts.Route{"raw": {Input: contracts.RouteInput{Params: map[string]contracts.Parameter{"bucket": {Type: "string"}}}}}}
	s, err = config.Resolve(c, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "chosen" || s.CaseID != "case-a" || s.Definition != filepath.Join(project, "source.yaml") {
		t.Fatalf("wrong case selection: %+v", s)
	}
	if string(s.Origins["input.bucket"].Value) != `"case-evidence"` {
		t.Fatal("case binding lost")
	}
	if string(s.Origins["credentials.aws"].Value) != `{"profile":"case-readonly","provider":"aws_profile"}` {
		t.Fatal("case profile lost")
	}
}

func TestBindingDefinitionUsesItsSelectedWorkspace(t *testing.T) {
	dir := t.TempDir()
	binding := filepath.Join(dir, "binding.yaml")
	write(t, binding, `apiVersion: batz/v0alpha1
kind: PipelineBinding
id: example
workspace: ./evidence
definition: workspace://pipelines/source.yaml
version: 1
route: local
`)
	c := config.Context{Project: t.TempDir(), Binding: binding}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "evidence/pipelines/source.yaml"); s.Definition != want {
		t.Fatalf("definition resolved before its workspace: got %q want %q", s.Definition, want)
	}
}

func TestYAMLTimestampInputPreservesItsSuppliedText(t *testing.T) {
	const timestamp = "2026-10-25T12:00:00.120000000+10:00"
	binding := filepath.Join(t.TempDir(), "binding.yaml")
	write(t, binding, `apiVersion: batz/v0alpha1
kind: PipelineBinding
id: example
definition: repo://source.yaml
version: 1
route: local
input:
  since: `+timestamp+"\n")
	c := config.Context{Project: t.TempDir(), Binding: binding}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	p := contracts.SourcePipeline{Routes: map[string]contracts.Route{
		"local": {Input: contracts.RouteInput{Params: map[string]contracts.Parameter{
			"since": {Type: "timestamp"},
		}}},
	}}
	s, err = config.Resolve(c, s, p)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := json.Unmarshal(s.Origins["input.since"].Value, &got); err != nil {
		t.Fatal(err)
	}
	if got != timestamp {
		t.Fatalf("timestamp text changed: got %q want %q", got, timestamp)
	}
}

func TestStringListRequiresAJSONArray(t *testing.T) {
	c := config.Context{Project: t.TempDir(), Route: "local", Input: map[string]string{"prefixes": "null"}}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	p := contracts.SourcePipeline{Routes: map[string]contracts.Route{
		"local": {Input: contracts.RouteInput{Params: map[string]contracts.Parameter{
			"prefixes": {Type: "string-list"},
		}}},
	}}
	if _, err := config.Resolve(c, s, p); err == nil || !strings.Contains(err.Error(), "JSON string list") {
		t.Fatalf("non-array JSON admitted as a string list: %v", err)
	}
}

func TestResolutionDoesNotSearchAncestorWorkspacesOrNearbyCases(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "project")
	workspace := filepath.Join(parent, "evidence/selected")
	for _, dir := range []string{project, workspace} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	// Poison documents outside the exact selected paths must never be loaded.
	write(t, filepath.Join(parent, "workspace.yaml"), "invalid: [document\n")
	write(t, filepath.Join(parent, "evidence/workspace.yaml"), "invalid: [document\n")
	write(t, filepath.Join(project, "case.yaml"), "invalid: [document\n")
	c := config.Context{Project: project, Workspace: workspace}
	s, err := config.Select(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err = config.Resolve(c, s, contracts.SourcePipeline{})
	if err != nil {
		t.Fatal(err)
	}
	if s.CaseID != "" || len(s.Origins) != 0 {
		t.Fatalf("unselected context leaked into resolution: %+v", s)
	}
}

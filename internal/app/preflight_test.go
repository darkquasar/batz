package app_test

import (
	"batz/internal/app"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnqualifiedNativeInputRefusedBeforeWorkspaceMutation(t *testing.T) {
	root := nativeContent(t)
	native := filepath.Join(root, "internal/app/testdata/synthetic/collector.yaml")
	config := `input:
  generate:
    count: 1
    mapping: 'root = {"payload":"{}","ordinal":0}'
output:
  batz_parquet_receipt: {}
`
	if err := os.WriteFile(native, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	input, err := filepath.Abs("../../internal/app/testdata/synthetic/events.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--input", "path=" + input}, &out, &diagnostics)
	if code != 2 {
		t.Fatalf("unqualified route reached execution: %d %s %s", code, &out, &diagnostics)
	}
	if _, err = os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("preflight mutated workspace: %v", err)
	}
}

func TestStaticRefusalsDoNotStartAcquisition(t *testing.T) {
	for _, name := range []string{"missing-workspace", "unknown-version", "duplicate-field", "missing-asset", "project-python", "reserved-handoff", "unknown-route"} {
		t.Run(name, func(t *testing.T) {
			root := nativeContent(t)
			workspace := filepath.Join(t.TempDir(), "workspace")
			definition := filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml")
			input, err := filepath.Abs("../../internal/app/testdata/synthetic/events.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"run", definition, "--project", root, "--route", "local_raw", "--input", "path=" + input}
			expected := 2
			if name != "missing-workspace" {
				args = append(args, "--workspace", workspace)
			}
			switch name {
			case "unknown-version", "duplicate-field":
				b, err := os.ReadFile(definition)
				if err != nil {
					t.Fatal(err)
				}
				changed := strings.Replace(string(b), "batz/v0alpha2", "batz/v9", 1)
				if name == "duplicate-field" {
					changed = string(b) + "version: 1\n"
				}
				if err = os.WriteFile(definition, []byte(changed), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-asset":
				expected = 3
				if err = os.Remove(filepath.Join(root, "internal/app/testdata/synthetic/collector.yaml")); err != nil {
					t.Fatal(err)
				}
			case "project-python":
				if err = os.WriteFile(filepath.Join(root, "config.py"), []byte("raise RuntimeError('PROJECT PYTHON EXECUTED')\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "reserved-handoff":
				args = append(args, "--input", "BATZ_STAGE_DIR=/tmp/override")
			case "unknown-route":
				args = append(args, "--route", "early_ack")
			}
			var out, diagnostics bytes.Buffer
			code := app.Execute(args, &out, &diagnostics)
			if code != expected {
				t.Fatalf("%s: exit=%d expected=%d %s %s", name, code, expected, &out, &diagnostics)
			}
			if _, err = os.Stat(workspace); !os.IsNotExist(err) {
				t.Fatalf("preflight mutated workspace: %v", err)
			}
			if strings.Contains(diagnostics.String(), "PROJECT PYTHON EXECUTED") {
				t.Fatal("project Python ran")
			}
		})
	}
}

func TestBothCommandsValidateTheCompletePipelineSchema(t *testing.T) {
	for _, command := range []string{"run", "explain"} {
		for _, mutation := range []string{"missing-name", "invalid-name", "missing-required", "invalid-route-id", "unknown-unselected-field"} {
			t.Run(command+"/"+mutation, func(t *testing.T) {
				root := nativeContent(t)
				definition := filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml")
				b, err := os.ReadFile(definition)
				if err != nil {
					t.Fatal(err)
				}
				changed := string(b)
				switch mutation {
				case "missing-name":
					changed = strings.Replace(changed, "name: synthetic-jsonl\n", "", 1)
				case "invalid-name":
					changed = strings.Replace(changed, "name: synthetic-jsonl", "name: '../invalid'", 1)
				case "missing-required":
					changed = strings.Replace(changed, "required: true, ", "", 1)
				case "invalid-route-id":
					changed = strings.Replace(changed, "local_raw:", "'invalid/route':", 1)
				case "unknown-unselected-field":
					changed += "  unused:\n    input: {engine: bento, config: x, params: {}, unexpected: true}\n    output: {parsed: {format: parquet}, normalized: {models: {findings: probe.findings}}}\n"
				}
				if err = os.WriteFile(definition, []byte(changed), 0600); err != nil {
					t.Fatal(err)
				}
				workspace := filepath.Join(t.TempDir(), "workspace")
				args := []string{command, definition, "--project", root}
				if command == "run" {
					input, err := filepath.Abs("../../internal/app/testdata/synthetic/events.jsonl")
					if err != nil {
						t.Fatal(err)
					}
					args = append(args, "--workspace", workspace, "--route", "local_raw", "--input", "path="+input)
				}
				var out, diagnostics bytes.Buffer
				code := app.Execute(args, &out, &diagnostics)
				if code != 2 {
					t.Fatalf("invalid schema accepted: %d %s %s", code, &out, &diagnostics)
				}
				if _, err = os.Stat(workspace); !os.IsNotExist(err) {
					t.Fatal("workspace mutated")
				}
			})
		}
	}
}

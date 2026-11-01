package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"batz/internal/config"
	"batz/internal/contracts"
	pipeline "batz/internal/run"
)

type inputs map[string]string

func (i inputs) String() string { return "" }
func (i inputs) Set(raw string) error {
	k, v, ok := strings.Cut(raw, "=")
	if !ok || k == "" || v == "" {
		return fmt.Errorf("input must be key=value")
	}
	if _, ok = i[k]; ok {
		return fmt.Errorf("duplicate input %s", k)
	}
	i[k] = v
	return nil
}
func resolve(value, root, declaring, workspace string) string {
	return config.ResolvePath(value, root, workspace, declaring)
}
func Execute(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "status" {
		return statusCommand(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "explain" {
		return explain(args[1:], stdout, stderr)
	}
	diagnostic := func(code int, message string) int { return preflightDiagnostic(stderr, code, errors.New(message)) }
	if len(args) < 2 || (args[0] != "run" && args[0] != "retry") {
		return diagnostic(2, "usage: batz run <pipeline.yaml> --workspace PATH --route ROUTE --input path=FILE")
	}
	modelsOnly := args[0] == "retry"
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	project := fs.String("project", "", "content checkout")
	workspace := fs.String("workspace", "", "execution workspace")
	route := fs.String("route", "", "selected route")
	binding := fs.String("binding-id", "", "stable binding ID")
	bindingFile := fs.String("binding", "", "saved binding")
	caseFile := fs.String("case", "", "explicit case")
	interactive := fs.Bool("interactive", false, "prompt for missing values")
	strict := fs.Bool("strict", false, "fail command for published partial")
	retryRun := fs.String("run-id", "", "pin retry to the failed run's current parsed inventory")
	values := inputs{}
	fs.Var(values, "input", "key=value")
	definitionArg := ""
	options := args[1:]
	if len(options) > 0 && !strings.HasPrefix(options[0], "-") {
		definitionArg = options[0]
		options = options[1:]
	}
	if err := fs.Parse(options); err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if fs.NArg() != 0 {
		return diagnostic(2, "unexpected positional arguments")
	}
	if *retryRun != "" && !modelsOnly {
		return diagnostic(2, "--run-id is only valid for retry")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return diagnostic(2, err.Error())
	}
	if *project == "" {
		candidate := cwd
		for {
			if _, err = os.Stat(filepath.Join(candidate, ".git")); err == nil {
				*project = candidate
				break
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				return diagnostic(2, "missing project checkout")
			}
			candidate = parent
		}
	}
	*project, err = filepath.Abs(*project)
	if err != nil {
		return diagnostic(2, err.Error())
	}
	contextInput := config.Context{Project: *project, Workspace: *workspace, Binding: *bindingFile, Case: *caseFile, BindingID: *binding, Route: *route, Input: values}
	selection, err := config.Select(contextInput)
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if definitionArg == "" {
		definitionArg = selection.Definition
	}
	if definitionArg == "" {
		return diagnostic(2, "missing pipeline definition or explicit binding")
	}
	definition, err := filepath.Abs(definitionArg)
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if selection.Definition != "" && filepath.Clean(selection.Definition) != definition {
		return diagnostic(2, "binding definition mismatch")
	}
	*workspace = selection.Workspace
	reader := bufio.NewReader(os.Stdin)
	if *workspace == "" && *interactive {
		fmt.Fprint(stderr, "Workspace: ")
		line, e := reader.ReadString('\n')
		err = e
		*workspace = strings.TrimSpace(line)
		if err != nil {
			return diagnostic(2, "missing-workspace")
		}
		selection.Workspace = *workspace
	}
	staticIssues := []string{}
	if *workspace == "" {
		staticIssues = append(staticIssues, "missing-workspace: execution requires an explicit workspace")
	} else {
		*workspace, err = filepath.Abs(*workspace)
		if err != nil {
			return preflightDiagnostic(stderr, 2, err)
		}
	}
	selection.Workspace = *workspace
	manifest, err := loadPipeline(definition)
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	contextInput.Definition = definition
	selection, err = config.Resolve(contextInput, selection, manifest)
	if err != nil {
		staticIssues = append(staticIssues, err.Error())
	}
	*binding = selection.ID
	*route = selection.Route
	if effective, ok := selection.Values["input"].(map[string]any); ok {
		values = inputs{}
		for key, value := range effective {
			if str, ok := value.(string); ok {
				values[key] = str
			} else {
				raw, e := json.Marshal(value)
				if e != nil {
					return preflightDiagnostic(stderr, 2, e)
				}
				values[key] = string(raw)
			}
		}
	}
	if effective, ok := selection.Values["normalization"].(map[string]any); ok {
		if path, ok := effective["project"].(string); ok {
			manifest.Normalization.Project = path
		}
	}
	selected, ok := manifest.Routes[*route]
	if !ok {
		return diagnostic(2, "unknown-route")
	}
	if selected.Input.Engine != "bento" || selected.Output.Parsed.Format != "parquet" {
		return diagnostic(2, "unsupported native route")
	}
	names := make([]string, 0, len(selected.Input.Params))
	for name := range selected.Input.Params {
		names = append(names, name)
	}
	sort.Strings(names)
	missing := append([]string{}, staticIssues...)
	for _, name := range names {
		param := selected.Input.Params[name]
		if param.Required && values[name] == "" {
			if *interactive {
				fmt.Fprintf(stderr, "Input %s: ", name)
				line, e := reader.ReadString('\n')
				if e != nil {
					return preflightDiagnostic(stderr, 2, e)
				}
				values[name] = strings.TrimSpace(line)
			}
			if values[name] == "" {
				missing = append(missing, "missing input."+name)
			}
		}
	}
	if len(missing) > 0 {
		native := resolve(selected.Input.Config, *project, filepath.Dir(definition), *workspace)
		models := resolve(manifest.Normalization.Project, *project, filepath.Dir(definition), *workspace)
		for _, path := range executionAssets(*project, native, models, modelsOnly) {
			if _, e := os.Stat(path); e != nil {
				missing = append(missing, "missing-asset: "+path)
			}
		}
	}
	if len(missing) > 0 {
		return diagnostic(2, strings.Join(missing, "; "))
	}
	if storage, ok := selection.Values["storage"].(map[string]any); ok && len(storage) > 0 {
		return diagnostic(3, "selected storage configuration is not yet qualified for native execution")
	}
	if len(values) != 1 || values["path"] == "" {
		return diagnostic(2, "local route requires only input path; reserved handoff cannot be supplied")
	}
	if len(selected.Input.Params) != 1 || selected.Input.Params["path"].Type != "path" || selected.Input.Params["path"].Env != "BATZ_SOURCE_PATH" {
		return diagnostic(2, "unsupported local parameter contract")
	}
	source := values["path"]
	if strings.HasPrefix(source, "repo://") {
		source = resolve(source, *project, cwd, *workspace)
	} else if !filepath.IsAbs(source) {
		source = filepath.Join(cwd, source)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return diagnostic(2, err.Error())
	}
	native := resolve(selected.Input.Config, *project, filepath.Dir(definition), *workspace)
	models := resolve(manifest.Normalization.Project, *project, filepath.Dir(definition), *workspace)
	for _, path := range executionAssets(*project, native, models, modelsOnly) {
		if _, err = os.Stat(path); err != nil {
			return diagnostic(3, "missing-asset: "+path)
		}
	}
	if err = validateNative(native); err != nil {
		return diagnostic(2, err.Error())
	}
	if modelsOnly {
		err = verifyRuntimeArtifacts(*project, "python")
	} else {
		err = verifyRuntime(*project)
	}
	if err != nil {
		return diagnostic(4, err.Error())
	}
	// Admit only native loader inputs before creating a private snapshot.
	err = validateProjectContent(models)
	if err != nil {
		return diagnostic(2, err.Error())
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	scopedProject, scope, cleanup, err := validateModelReferences(ctx, *project, models, manifest.Execution(*route))
	if err != nil {
		code := 2
		if errors.Is(err, context.Canceled) {
			code = 130
		}
		if errors.Is(err, context.DeadlineExceeded) {
			code = 1
		}
		return diagnostic(code, err.Error())
	}
	defer cleanup()
	models = scopedProject
	if *binding == "" {
		if modelsOnly {
			return diagnostic(2, "retry requires an existing binding ID")
		}
		tmp := pipeline.NewBindingID()
		*binding = tmp
	}
	if !regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$").MatchString(*binding) {
		return diagnostic(2, "invalid binding ID")
	}
	report, err := pipeline.Execute(ctx, pipeline.Request{Scope: scope, Contract: manifest.Execution(*route), CaseID: selection.CaseID, Root: *project, Workspace: *workspace, Definition: definition, Route: *route, Source: source, Binding: *binding, NativeConfig: native, Project: models, Strict: *strict, Retry: modelsOnly, RetryRun: contracts.RunID(*retryRun), Resolution: selection.Origins})
	report.Resolution = selection.Origins
	if e := json.NewEncoder(stdout).Encode(report); e != nil {
		return diagnostic(1, e.Error())
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return report.Exit
	}
	return report.Exit
}

func executionAssets(root, native, models string, modelsOnly bool) []string {
	paths := []string{native, filepath.Join(models, "config.yaml"), filepath.Join(root, "runtime/.local/venv/bin/python"), filepath.Join(root, "runtime/sqlmesh/runner.py")}
	if !modelsOnly {
		paths = append(paths, filepath.Join(root, "runtime/.local/batz-collector"))
	}
	return paths
}

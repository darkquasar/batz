package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"batz/internal/config"
	"batz/internal/contracts"
)

func explain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		return preflightDiagnostic(stderr, 2, errors.New("missing pipeline definition"))
	}
	flags := flag.NewFlagSet("explain", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "content checkout")
	route := flags.String("route", "", "selected route")
	level := flags.Int("level", 1, "explanation level")
	format := flags.String("format", "text", "text or json")
	workspace := flags.String("workspace", "", "explicit execution context")
	bindingFile := flags.String("binding", "", "saved binding")
	caseFile := flags.String("case", "", "selected case")
	bindingID := flags.String("binding-id", "", "selected case binding")
	values := inputs{}
	flags.Var(values, "input", "key=value")
	exampleID := flags.String("example", "", "registered local example ID or explicitly associated path")
	strict := flags.Bool("strict", false, "require requested assets")
	if err := flags.Parse(args[1:]); err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if flags.NArg() != 0 || (*format != "json" && *format != "text") || *level < 1 || *level > 3 {
		return preflightDiagnostic(stderr, 2, errors.New("invalid explain options"))
	}
	definition, err := filepath.Abs(args[0])
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if *project == "" {
		p := filepath.Dir(definition)
		for {
			if _, err = os.Stat(filepath.Join(p, ".git")); err == nil {
				*project = p
				break
			}
			parent := filepath.Dir(p)
			if parent == p {
				return preflightDiagnostic(stderr, 2, errors.New("no content checkout; use --project"))
			}
			p = parent
		}
	}
	*project, err = filepath.Abs(*project)
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	pipeline, err := loadPipeline(definition)
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	contextInput := config.Context{Project: *project, Workspace: *workspace, Binding: *bindingFile, Case: *caseFile, BindingID: *bindingID, Route: *route, Input: values}
	selection, err := config.Select(contextInput)
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if selection.Definition != "" && filepath.Clean(selection.Definition) != definition {
		return preflightDiagnostic(stderr, 2, errors.New("binding definition does not match positional definition"))
	}
	if selection.Route == "" && len(pipeline.Routes) == 1 {
		for name := range pipeline.Routes {
			selection.Route = name
		}
	}
	contextInput.Definition = definition
	selection, err = config.Resolve(contextInput, selection, pipeline)
	if err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if normalization, ok := selection.Values["normalization"].(map[string]any); ok {
		if selectedProject, ok := normalization["project"].(string); ok {
			pipeline.Normalization.Project = selectedProject
		}
	}
	if *bindingFile != "" || *caseFile != "" {
		*route = selection.Route
	}
	if *route != "" {
		if _, ok := pipeline.Routes[*route]; !ok {
			return preflightDiagnostic(stderr, 2, errors.New("unknown-route"))
		}
	}
	report := contracts.ExplainReport{APIVersion: contracts.APIVersion, Kind: "ExplainReport", Pipeline: pipeline.Name, Version: pipeline.Version, Level: *level, Project: pipeline.Normalization.Project, Routes: []contracts.ExplainRoute{}, Assets: map[string]string{}, Diagnostics: []contracts.Diagnostic{}, Stages: []contracts.ExplainStage{}, Structures: []contracts.ExplainStructure{}, Samples: []contracts.ExplainSample{}, Resolution: map[string]contracts.ValueOrigin{}, Runtime: map[string]any{}}
	report.Resolution = selection.Origins
	asset := func(label, path string) {
		if strings.HasPrefix(path, "workspace://") {
			report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: "incomplete-capability", Code: "unresolved-path", Source: label, Message: "asset requires an explicitly selected workspace"})
			return
		}
		b, e := os.ReadFile(path)
		if e != nil {
			report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: "incomplete-capability", Code: "missing-asset", Source: label, Message: "declared asset unavailable"})
			return
		}
		sum := sha256.Sum256(b)
		report.Assets[label] = hex.EncodeToString(sum[:])
	}
	asset("definition", definition)
	names := []string{}
	for name := range pipeline.Routes {
		if *route == "" || name == *route {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		r := pipeline.Routes[name]
		report.Routes = append(report.Routes, contracts.ExplainRoute{Name: name, Engine: r.Input.Engine, Config: r.Input.Config, ParsedFormat: r.Output.Parsed.Format, Models: r.Output.Normalized.Models, Basis: "declared"})
		asset(r.Input.Config, resolve(r.Input.Config, *project, filepath.Dir(definition), selection.Workspace))
	}
	modelRoot := resolve(pipeline.Normalization.Project, *project, filepath.Dir(definition), selection.Workspace)
	asset(pipeline.Normalization.Project+"/config.yaml", resolve(pipeline.Normalization.Project+"/config.yaml", *project, filepath.Dir(definition), selection.Workspace))
	if !strings.HasPrefix(modelRoot, "workspace://") {
		_ = filepath.WalkDir(modelRoot, func(path string, entry os.DirEntry, e error) error {
			if e != nil {
				return nil
			}
			if !entry.IsDir() && (filepath.Ext(path) == ".py" || filepath.Ext(path) == ".pyc") {
				rel, _ := filepath.Rel(modelRoot, path)
				report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: "incomplete-capability", Code: "unsupported-content", Source: rel, Message: "project Python content is unsupported"})
			}
			return nil
		})
	}
	if *level >= 2 {
		type stageSpec struct {
			id, kind                string
			inputs, native, outputs []string
		}
		for _, r := range report.Routes {
			specs := []stageSpec{
				{id: "source", kind: "source-document", inputs: []string{}, native: []string{}, outputs: []string{"decoded"}},
				{id: "decoded", kind: "native-decoding", inputs: []string{"source"}, native: []string{r.Config}, outputs: []string{"parsed"}},
				{id: "parsed", kind: "parsed-publication", inputs: []string{"decoded"}, native: []string{r.Config}, outputs: []string{"normalization"}},
			}
			if isTracerModelSelection(r.Models) {
				specs[2].outputs = []string{"source_staging"}
				specs = append(specs,
					stageSpec{id: "source_staging", kind: "sql-staging", inputs: []string{"parsed"}, native: []string{pipeline.Normalization.Project + "/models/source_staging.sql"}, outputs: []string{"api_activity"}},
					stageSpec{id: "api_activity", kind: "normalized-output", inputs: []string{"source_staging"}, native: []string{pipeline.Normalization.Project + "/models/api_activity.sql"}, outputs: []string{"reader"}},
				)
			}
			for _, stage := range specs {
				report.Stages = append(report.Stages, contracts.ExplainStage{ID: r.Name + "/" + stage.id, Route: r.Name, Kind: stage.kind, Inputs: stage.inputs, Native: stage.native, Outputs: stage.outputs, Basis: "declared", Reason: "Declared local tracer boundaries; no sample executed"})
				for _, native := range stage.native {
					asset(native, resolve(native, *project, filepath.Dir(definition), selection.Workspace))
				}
			}
			if !isTracerModelSelection(r.Models) {
				outputs := make([]string, 0, len(r.Models))
				for name := range r.Models {
					outputs = append(outputs, name)
				}
				sort.Strings(outputs)
				report.Stages = append(report.Stages, contracts.ExplainStage{ID: r.Name + "/normalization", Route: r.Name, Kind: "sql-normalization", Inputs: []string{"parsed"}, Native: []string{}, Outputs: outputs, Basis: "unavailable", Reason: "Native dependencies and topology require a qualified model adapter"})
				for _, name := range outputs {
					report.Stages = append(report.Stages, contracts.ExplainStage{ID: r.Name + "/" + name, Route: r.Name, Kind: "normalized-output", Inputs: []string{"normalization"}, Native: []string{}, Outputs: []string{name}, Basis: "unavailable", Reason: "Declared model " + r.Models[name] + "; no qualified topology or projection adapter"})
				}
				report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: "incomplete-capability", Code: "unavailable-topology", Source: r.Name, Message: "Declared outputs retained; native model topology is unavailable"})
			}
		}
	}
	previewCode := 0
	if *level == 3 {
		examples := pipeline.Explain.Examples
		selected := []contracts.Example{}
		seen := map[string]bool{}
		for _, ex := range examples {
			if seen[ex.ID] {
				return preflightDiagnostic(stderr, 2, errors.New("duplicate example ID"))
			}
			seen[ex.ID] = true
			if _, ok := pipeline.Routes[ex.Route]; !ok {
				return preflightDiagnostic(stderr, 2, errors.New("example references unknown route"))
			}
			if (*route == "" || *route == ex.Route) && (*exampleID == "" || *exampleID == ex.ID || resolve(ex.Input, *project, filepath.Dir(definition), selection.Workspace) == resolve(*exampleID, *project, filepath.Dir(definition), selection.Workspace)) {
				selected = append(selected, ex)
			}
		}
		if len(selected) == 0 && *exampleID != "" && *route != "" {
			file, err := filepath.Abs(*exampleID)
			if err == nil {
				info, e := os.Lstat(file)
				if e == nil && info.Mode().IsRegular() && info.Size() <= 16<<20 {
					content, e := os.ReadFile(file)
					if e != nil {
						return preflightDiagnostic(stderr, 2, e)
					}
					digest := sha256.Sum256(content)
					selected = append(selected, contracts.Example{ID: "explicit", Route: *route, Input: file, SHA256: hex.EncodeToString(digest[:])})
				}
			}
		}
		if len(selected) != 1 {
			if *exampleID != "" {
				return preflightDiagnostic(stderr, 2, errors.New("example-route association is missing or ambiguous"))
			}
			report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: "incomplete-capability", Code: "unavailable-example", Message: "No unambiguous registered local example"})
		} else {
			code, e := preview(*project, definition, selection.Workspace, pipeline, selected[0], &report)
			if e != nil {
				category := "execution"
				if code == 3 {
					category = "incomplete-capability"
				}
				if code == 4 {
					category = "integrity"
				}
				report.Diagnostics = append(report.Diagnostics, contracts.Diagnostic{Category: category, Code: "preview-failed", Message: e.Error()})
				if code != 3 {
					previewCode = code
				}
			}
		}
	}

	if *format == "json" {
		if err = json.NewEncoder(stdout).Encode(report); err != nil {
			return preflightDiagnostic(stderr, 1, err)
		}
	} else {
		fmt.Fprintf(stdout, "%s version %d (declared)\n", pipeline.Name, pipeline.Version)
		for _, r := range report.Routes {
			fmt.Fprintf(stdout, "  %s: %s -> %s -> native SQL (%s)\n", r.Name, r.Engine, r.ParsedFormat, report.Project)
		}
		for _, stage := range report.Stages {
			fmt.Fprintf(stdout, "  %s [%s]: %s\n", stage.ID, stage.Basis, stage.Kind)
		}
		for _, structure := range report.Structures {
			fmt.Fprintf(stdout, "  %s [%s] types:\n", structure.Stage, structure.Basis)
			for _, field := range structure.Fields {
				fmt.Fprintf(stdout, "    %s: %s (%s)\n", field.Name, field.NativeType, field.Encoding)
			}
		}
		for _, sample := range report.Samples {
			raw, _ := json.MarshalIndent(sample.Records, "    ", "  ")
			fmt.Fprintf(stdout, "  %s [%s] %d rows:\n    %s\n", sample.Stage, sample.Basis, sample.Rows, raw)
		}
		for _, d := range report.Diagnostics {
			fmt.Fprintf(stdout, "  %s: %s %s\n", d.Code, d.Source, d.Message)
		}
	}
	if previewCode != 0 {
		return previewCode
	}
	if *strict && len(report.Diagnostics) > 0 {
		return 3
	}
	return 0
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"batz/internal/contracts"
	pipeline "batz/internal/run"
	schema "batz/schemas/v0alpha1"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

var pipelineSchema struct {
	sync.Once
	value *gojsonschema.Schema
	err   error
}

type documentError struct{ Field, Source, Message string }

func (e *documentError) Error() string { return e.Field + ": " + e.Message }

// The frozen SourcePipeline uses only validation keywords shared by Draft 7 and
// 2020-12. Refuse newer keywords before using the existing pinned Draft-7 engine;
// this is not a general 2020-12 validator. Compilation reads no external schema.
func checkSchemaVocabulary(s map[string]any) error {
	for key, value := range s {
		switch key {
		case "$schema", "$id", "type", "const", "enum", "pattern", "minLength", "minimum", "maximum", "required", "minProperties":
		case "properties":
			for _, child := range value.(map[string]any) {
				if err := checkSchemaVocabulary(child.(map[string]any)); err != nil {
					return err
				}
			}
		case "items", "propertyNames":
			if err := checkSchemaVocabulary(value.(map[string]any)); err != nil {
				return err
			}
		case "additionalProperties":
			if child, ok := value.(map[string]any); ok {
				if err := checkSchemaVocabulary(child); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported embedded schema keyword %s", key)
		}
	}
	return nil
}
func loadPipeline(path string) (contracts.SourcePipeline, error) {
	var result contracts.SourcePipeline
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	var value map[string]any
	d := yaml.NewDecoder(f)
	var node yaml.Node
	if err = d.Decode(&node); err != nil {
		return result, &documentError{Source: path, Field: "document", Message: err.Error()}
	}
	var preserveTimestampText func(*yaml.Node)
	preserveTimestampText = func(n *yaml.Node) {
		if n.Tag == "!!timestamp" {
			n.Tag = "!!str"
		}
		for _, child := range n.Content {
			preserveTimestampText(child)
		}
	}
	preserveTimestampText(&node)
	if err = node.Decode(&value); err != nil {
		return result, &documentError{Source: path, Field: "document", Message: err.Error()}
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return result, &documentError{Source: path, Field: "document", Message: "multiple YAML documents"}
	}
	if value["apiVersion"] != "batz/v0alpha2" {
		return result, &documentError{Source: path, Field: "apiVersion", Message: "invalid-version: unsupported SourcePipeline contract; expected batz/v0alpha2"}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return result, &documentError{Source: path, Field: "document", Message: err.Error()}
	}
	pipelineSchema.Do(func() {
		var s map[string]any
		if pipelineSchema.err = json.Unmarshal([]byte(schema.SourcePipeline()), &s); pipelineSchema.err != nil {
			return
		}
		if pipelineSchema.err = checkSchemaVocabulary(s); pipelineSchema.err != nil {
			return
		}
		loader := gojsonschema.NewSchemaLoader()
		loader.AutoDetect = false
		loader.Validate = false
		loader.Draft = gojsonschema.Draft7
		pipelineSchema.value, pipelineSchema.err = loader.Compile(gojsonschema.NewStringLoader(schema.SourcePipeline()))
	})
	if pipelineSchema.err != nil {
		return result, pipelineSchema.err
	}
	validation, err := pipelineSchema.value.Validate(gojsonschema.NewBytesLoader(raw))
	if err != nil {
		return result, err
	}
	if !validation.Valid() {
		messages := []string{}
		for _, e := range validation.Errors() {
			messages = append(messages, e.Field()+": "+e.Description())
		}
		sort.Strings(messages)
		return result, &documentError{Source: path, Field: strings.SplitN(messages[0], ":", 2)[0], Message: strings.Join(messages, "; ")}
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	if result.Version != 1 {
		return result, &documentError{Source: path, Field: "version", Message: "invalid-version: unsupported pipeline contract"}
	}
	return result, nil
}

// Native discovery validates IDs before acquisition. It runs on a private copy,
// so SQLMesh logs/cache and working/state files never touch contributor content.
func validateModelReferences(parent context.Context, root, project string, contract contracts.ExecutionContract) error {
	dir, err := os.MkdirTemp("", "batz-model-preflight-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	copied := filepath.Join(dir, "project")
	if err := copyProject(project, copied); err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]any{"validate_only": true, "project": copied, "contract": contract})
	if err != nil {
		return err
	}
	request := filepath.Join(dir, "request.json")
	if err := os.WriteFile(request, raw, 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	_, err = pipeline.InspectPrivate(ctx, dir, "", filepath.Join(root, "runtime/.local/venv/bin/python"), filepath.Join(root, "runtime/sqlmesh/runner.py"), request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("invalid native model references: %w", err)
	}

	return nil
}

func validateProjectContent(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in normalization content")
		}
		relative, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		relative = filepath.ToSlash(relative)
		if !entry.IsDir() && filepath.Ext(path) == ".py" {
			return fmt.Errorf("unsupported-content: Python in %s", path)
		}
		// These SQLMesh inputs are unsupported by the production contract. Ignoring
		// them would make contributor tests and actual execution disagree.
		first, _, _ := strings.Cut(relative, "/")
		switch first {
		case "external_models.yaml", "external_models.yml", "metrics", "signals", "materializations", "linting":
			return fmt.Errorf("unsupported-content: native execution input %s", path)
		}
		if !entry.IsDir() && contracts.ProductionProjectPath(relative) {
			ext := filepath.Ext(path)
			if ext != ".sql" && ext != ".yaml" && ext != ".yml" && ext != ".csv" {
				return fmt.Errorf("unsupported-content: %s", path)
			}
		}
		return nil
	})
}

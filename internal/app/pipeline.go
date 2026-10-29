package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"batz/internal/contracts"
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

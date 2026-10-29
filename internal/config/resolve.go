// Package config resolves explicit nonsecret configuration without source contact.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"batz/internal/contracts"
	schema "batz/schemas/v0alpha1"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

type Context struct {
	Project, Workspace, Binding, Case, BindingID, Route, Definition string
	Input                                                           map[string]string
}
type Selection struct {
	Definition, Workspace, ID, Route, CaseID string
	Version                                  int
	Values                                   map[string]any
	Origins                                  map[string]contracts.ValueOrigin
}

type document struct {
	path  string
	value map[string]any
}

func read(path string, optional bool) (document, error) {
	d := document{path: path, value: map[string]any{}}
	f, err := os.Open(path)
	if optional && os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	defer f.Close()
	decoder := yaml.NewDecoder(io.LimitReader(f, 4<<20))
	var node yaml.Node
	if err = decoder.Decode(&node); err != nil {
		return d, err
	}
	var inspect func(*yaml.Node) error
	inspect = func(n *yaml.Node) error {
		if n.Kind == yaml.AliasNode {
			return fmt.Errorf("YAML aliases unsupported")
		}
		if n.Tag == "!!null" && n.Value == "" {
			return fmt.Errorf("blank YAML value at line %d; use explicit null", n.Line)
		}
		// A timestamp parameter is delivered as its supplied RFC3339 text,
		// including trailing fractional zeros and the original offset.
		if n.Tag == "!!timestamp" {
			n.Tag = "!!str"
		}
		for _, child := range n.Content {
			if err := inspect(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err = inspect(&node); err != nil {
		return d, err
	}
	if err = node.Decode(&d.value); err != nil {
		return d, err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return d, fmt.Errorf("multiple YAML documents")
	}
	if d.value["apiVersion"] != contracts.APIVersion {
		return d, fmt.Errorf("unsupported config version")
	}
	allowed := map[string]bool{"apiVersion": true, "kind": true, "storage": true, "normalization": true, "credentials": true, "pipeline_defaults": true}
	switch d.value["kind"] {
	case "Workspace":
	case "Case":
		for _, k := range []string{"id", "workspace", "pipelines"} {
			allowed[k] = true
		}
	case "PipelineBinding":
		for _, k := range []string{"id", "definition", "version", "route", "input", "credential_aliases", "workspace"} {
			allowed[k] = true
		}
	default:
		return d, fmt.Errorf("unsupported config kind")
	}
	schemaText := schema.Configuration(text(d.value, "kind"))
	raw, err := json.Marshal(d.value)
	if err != nil {
		return d, err
	}
	loader := gojsonschema.NewSchemaLoader()
	loader.AutoDetect = false
	loader.Draft = gojsonschema.Draft7
	if err = loader.AddSchemas(gojsonschema.NewStringLoader(schema.Common())); err != nil {
		return d, err
	}
	compiled, err := loader.Compile(gojsonschema.NewStringLoader(schemaText))
	if err != nil {
		return d, err
	}
	validation, err := compiled.Validate(gojsonschema.NewBytesLoader(raw))
	if err != nil {
		return d, err
	}
	if !validation.Valid() {
		messages := []string{}
		for _, e := range validation.Errors() {
			messages = append(messages, e.Field()+": "+e.Description())
		}
		sort.Strings(messages)
		return d, fmt.Errorf("%s: %s", path, strings.Join(messages, "; "))
	}
	for k := range d.value {
		if !allowed[k] {
			return d, fmt.Errorf("unknown config field %s", k)
		}
	}
	return d, nil
}
func text(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func mapping(v any) map[string]any           { m, _ := v.(map[string]any); return m }

// ResolvePath expands declared path fields while leaving workspace URIs symbolic
// until an execution workspace is explicitly selected.
func ResolvePath(value, project, workspace, declaring string) string {
	switch {
	case strings.HasPrefix(value, "repo://"):
		return filepath.Join(project, strings.TrimPrefix(value, "repo://"))
	case strings.HasPrefix(value, "workspace://"):
		if workspace == "" {
			return value
		}
		return filepath.Join(workspace, strings.TrimPrefix(value, "workspace://"))
	case strings.HasPrefix(value, "~/"):
		user, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(user, value[2:])
		}
	case filepath.IsAbs(value):
		return filepath.Clean(value)
	}
	return filepath.Join(declaring, value)
}

// Select resolves only explicitly selected documents. It never scans nearby cases.
func Select(c Context) (Selection, error) {
	s := Selection{Workspace: c.Workspace, ID: c.BindingID, Route: c.Route, Values: map[string]any{}, Origins: map[string]contracts.ValueOrigin{}}
	if s.Workspace != "" {
		workspace, err := filepath.Abs(s.Workspace)
		if err != nil {
			return s, err
		}
		s.Workspace = workspace
	}
	if c.Binding != "" && c.Case != "" {
		return s, fmt.Errorf("binding and case are mutually exclusive")
	}
	selected := c.Binding
	if selected == "" {
		selected = c.Case
	}
	if selected == "" {
		return s, nil
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return s, err
	}
	d, err := read(absolute, false)
	if err != nil {
		return s, err
	}
	b := d.value
	if c.Case != "" {
		if b["kind"] != "Case" {
			return s, fmt.Errorf("expected Case")
		}
		s.CaseID = text(b, "id")
		entries, ok := b["pipelines"].([]any)
		if !ok {
			return s, fmt.Errorf("case pipelines must be a list")
		}
		found := false
		seen := map[string]bool{}
		for _, entry := range entries {
			candidate, ok := entry.(map[string]any)
			if !ok {
				return s, fmt.Errorf("invalid case binding")
			}
			id := text(candidate, "id")
			if id == "" || seen[id] {
				return s, fmt.Errorf("missing/duplicate case binding ID")
			}
			seen[id] = true
			if id == c.BindingID {
				b = candidate
				found = true
			}
		}
		if !found {
			return s, fmt.Errorf("case requires explicit existing --binding-id")
		}
	} else if b["kind"] != "PipelineBinding" {
		return s, fmt.Errorf("expected PipelineBinding")
	}
	for k := range b {
		switch k {
		case "apiVersion", "kind", "id", "definition", "version", "route", "input", "credential_aliases", "workspace", "storage", "normalization", "credentials", "pipeline_defaults":
		default:
			return s, fmt.Errorf("unknown binding field %s", k)
		}
	}
	if text(b, "definition") == "" {
		return s, fmt.Errorf("binding missing definition")
	}
	version, ok := b["version"].(int)
	if !ok || version != 1 {
		return s, fmt.Errorf("unsupported binding version")
	}
	s.Version = version
	if s.ID == "" {
		s.ID = text(b, "id")
	}
	if s.Route == "" {
		s.Route = text(b, "route")
	}
	if s.Workspace == "" {
		w := text(b, "workspace")
		if w == "" {
			w = text(d.value, "workspace")
		}
		if w != "" {
			s.Workspace = ResolvePath(w, c.Project, "", filepath.Dir(absolute))
		}
	}
	s.Definition = ResolvePath(text(b, "definition"), c.Project, s.Workspace, filepath.Dir(absolute))
	s.Values["selected-shared"] = d.value
	s.Values["binding"] = b
	s.Values["declaring"] = absolute
	return s, nil
}

// Resolve merges six ordered layers, retaining the original declaration per leaf.
func Resolve(c Context, s Selection, p contracts.SourcePipeline) (Selection, error) {
	if s.Route == "" && len(p.Routes) == 1 {
		for name := range p.Routes {
			s.Route = name
		}
	}
	route, ok := p.Routes[s.Route]
	if !ok && s.Route != "" {
		return s, fmt.Errorf("unknown route")
	}
	values := map[string]any{}
	origins := s.Origins
	var merge func(map[string]any, map[string]any, string, string)
	merge = func(dst, src map[string]any, prefix, source string) {
		keys := make([]string, 0, len(src))
		for k := range src {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := src[k]
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			if v == nil {
				delete(dst, k)
				for old := range origins {
					if old == key || strings.HasPrefix(old, key+".") {
						delete(origins, old)
					}
				}
				continue
			}
			m, isMap := v.(map[string]any)
			if isMap && prefix != "credentials" {
				existing := mapping(dst[k])
				if existing == nil {
					existing = map[string]any{}
				}
				merge(existing, m, key, source)
				dst[k] = existing
				continue
			}
			if prefix == "credentials" {
				for old := range origins {
					if old == key || strings.HasPrefix(old, key+".") {
						delete(origins, old)
					}
				}
			}
			dst[k] = v
			raw, _ := json.Marshal(v)
			origins[key] = contracts.ValueOrigin{Value: raw, Source: source, Field: key}
		}
	}
	defaults := map[string]any{}
	for name, param := range route.Input.Params {
		if param.Default != nil {
			defaults[name] = param.Default
		}
	}
	merge(values, map[string]any{"input": defaults}, "", "route-defaults")
	layers := []document{}
	for _, file := range []string{filepath.Join(c.Project, "workspace.yaml"), filepath.Join(s.Workspace, "workspace.yaml")} {
		if s.Workspace == "" && file == "workspace.yaml" {
			continue
		}
		d, err := read(file, true)
		if err != nil {
			return s, err
		}
		if len(d.value) > 0 && d.value["kind"] != "Workspace" {
			return s, fmt.Errorf("expected Workspace: %s", file)
		}
		layers = append(layers, d)
	}
	declaring, _ := s.Values["declaring"].(string)
	if shared := mapping(s.Values["selected-shared"]); shared != nil {
		layers = append(layers, document{declaring, shared})
	}
	binding := mapping(s.Values["binding"])
	applyShared := func(d document) {
		shared := map[string]any{}
		for _, key := range []string{"storage", "normalization", "credentials", "pipeline_defaults"} {
			if v, ok := d.value[key]; ok {
				shared[key] = v
			}
		}
		merge(values, shared, "", d.path)
		scoped := mapping(mapping(mapping(d.value["pipeline_defaults"])[p.Name])[s.Route])
		if scoped != nil {
			merge(values, scoped, "", d.path)
		}
	}
	for _, d := range layers {
		applyShared(d)
	}
	if binding != nil {
		applyShared(document{declaring, binding})
		local := map[string]any{}
		for _, k := range []string{"input", "credential_aliases"} {
			if v, ok := binding[k]; ok {
				local[k] = v
			}
		}
		merge(values, local, "", declaring)
	}
	cli := map[string]any{}
	for k, v := range c.Input {
		cli[k] = v
	}
	merge(values, map[string]any{"input": cli}, "", "cli")
	// Only declared path fields expand; other strings (including credentials) stay data.
	for key, origin := range origins {
		isPath := key == "normalization.project" || strings.HasPrefix(key, "storage.")
		if !isPath {
			continue
		}
		var str string
		if json.Unmarshal(origin.Value, &str) != nil {
			continue
		}
		dir := filepath.Dir(origin.Source)
		if origin.Source == "cli" {
			dir, _ = os.Getwd()
		}
		resolved := ResolvePath(str, c.Project, s.Workspace, dir)
		origin.Value, _ = json.Marshal(resolved)
		origins[key] = origin
		parts := strings.Split(key, ".")
		parent := values
		for _, part := range parts[:len(parts)-1] {
			parent = mapping(parent[part])
			if parent == nil {
				break
			}
		}
		if parent != nil {
			parent[parts[len(parts)-1]] = resolved
		}
	}
	var problems []string
	for name, v := range mapping(values["input"]) {
		param, ok := route.Input.Params[name]
		if !ok {
			problems = append(problems, "unknown input "+name)
			continue
		}
		origin := origins["input."+name]
		source := origin.Source
		dir := filepath.Dir(source)
		if source == "route-defaults" && c.Definition != "" {
			dir = filepath.Dir(c.Definition)
		}
		if source == "cli" || (source == "route-defaults" && c.Definition == "") {
			dir, _ = os.Getwd()
		}
		switch param.Type {
		case "path":
			str, ok := v.(string)
			if !ok {
				problems = append(problems, "input."+name+" must be a path")
				continue
			}
			v = ResolvePath(str, c.Project, s.Workspace, dir)
		case "string":
			if _, ok := v.(string); !ok {
				problems = append(problems, "input."+name+" must be string")
			}
		case "timestamp":
			str, ok := v.(string)
			if !ok {
				problems = append(problems, "invalid timestamp "+name)
			} else if _, err := time.Parse(time.RFC3339Nano, str); err != nil {
				problems = append(problems, "invalid timestamp "+name)
			}
		case "string-list":
			if str, ok := v.(string); ok {
				var list []string
				if json.Unmarshal([]byte(str), &list) != nil || list == nil {
					problems = append(problems, "input."+name+" must be a JSON string list")
				} else {
					v = list
				}
			} else {
				raw, _ := json.Marshal(v)
				var list []string
				if json.Unmarshal(raw, &list) != nil || list == nil {
					problems = append(problems, "invalid string list "+name)
				}
			}
		}
		mapping(values["input"])[name] = v
		origin.Value, _ = json.Marshal(v)
		origins["input."+name] = origin
	}
	// Return the resolved nonsecret values even when their validation fails so
	// execution preflight can report the other missing inputs in the same pass.
	s.Values = values
	s.Origins = origins
	if len(problems) > 0 {
		sort.Strings(problems)
		return s, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return s, nil
}

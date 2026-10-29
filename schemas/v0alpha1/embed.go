// Package schema supplies the compiled-in, offline wire contract.
package schema

import _ "embed"

//go:embed source-pipeline.schema.json
var sourcePipeline string

func SourcePipeline() string { return sourcePipeline }

//go:embed pipeline-binding.schema.json
var bindingSchema string

//go:embed workspace.schema.json
var workspaceSchema string

//go:embed case.schema.json
var caseSchema string

func Configuration(kind string) string {
	switch kind {
	case "PipelineBinding":
		return bindingSchema
	case "Workspace":
		return workspaceSchema
	case "Case":
		return caseSchema
	}
	return ""
}

//go:embed common.schema.json
var commonSchema string

func Common() string { return commonSchema }

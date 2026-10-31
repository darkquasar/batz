package app

import (
	"batz/internal/contracts"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

func preflightDiagnostic(out io.Writer, exit int, err error) int {
	d := contracts.Diagnostic{Category: "execution", Code: "execution-failed", Source: "cli", Message: err.Error()}
	switch exit {
	case 2:
		d.Category = "invalid-input"
		d.Code = "invalid-field"
	case 3:
		d.Category = "incomplete-capability"
		d.Code = "missing-asset"
	case 130:
		d.Category = "cancellation"
		d.Code = "cancelled"
	case 4:
		d.Category = "integrity"
		d.Code = "dependency-integrity"
	}
	for _, code := range []string{"missing-workspace", "unknown-route", "invalid-version", "missing-dependency", "missing-asset", "unsupported-content", "reserved-handoff"} {
		if strings.HasPrefix(err.Error(), code) {
			d.Code = code
			break
		}
	}
	if d.Code == "missing-workspace" {
		d.Field = "workspace"
	}
	var document *documentError
	if errors.As(err, &document) {
		d.Source = document.Source
		d.Field = document.Field
		if document.Field == "apiVersion" || document.Field == "kind" || document.Field == "version" {
			d.Code = "invalid-version"
		}
	}
	_ = json.NewEncoder(out).Encode(d)
	return exit
}

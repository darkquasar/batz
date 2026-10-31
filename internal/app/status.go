package app

import (
	"encoding/json"
	"errors"
	"flag"
	"io"

	pipeline "batz/internal/run"
)

func statusCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	workspace := fs.String("workspace", "", "execution workspace")
	runID := fs.String("run-id", "", "recorded run; latest when omitted")
	if err := fs.Parse(args); err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if *workspace == "" || fs.NArg() != 0 {
		return preflightDiagnostic(stderr, 2, errors.New("status requires --workspace and accepts no positional arguments"))
	}
	status, err := pipeline.ReadStatus(*workspace, *runID)
	if err != nil {
		return preflightDiagnostic(stderr, 1, err)
	}
	if err = json.NewEncoder(stdout).Encode(status); err != nil {
		return preflightDiagnostic(stderr, 1, err)
	}
	return 0
}

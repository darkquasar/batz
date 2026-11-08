package app

import (
	"batz/internal/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
)

func version(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "text or json")
	if err := flags.Parse(args); err != nil {
		return preflightDiagnostic(stderr, 2, err)
	}
	if flags.NArg() != 0 || (*format != "text" && *format != "json") {
		return preflightDiagnostic(stderr, 2, errors.New("usage: batz version [--format json]"))
	}
	identity := buildinfo.Current()
	var err error
	if *format == "json" {
		err = json.NewEncoder(stdout).Encode(identity)
	} else {
		_, err = fmt.Fprintf(stdout, "batz %s source=%s target=%s development=%t dirty=%t\n", identity.Version, identity.SourceSHA, identity.Target, identity.Development, identity.Dirty)
	}
	if err != nil {
		return preflightDiagnostic(stderr, 4, err)
	}
	return 0
}

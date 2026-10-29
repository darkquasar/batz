package app

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
)

// S1 admits only the finite local adapter and its receipt-writing sink. Other
// native inputs need their own completion/cursor qualification before execution.
func validateNative(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var config struct {
		Input    map[string]map[string]any `yaml:"input"`
		Pipeline struct {
			Threads    int              `yaml:"threads"`
			Processors []map[string]any `yaml:"processors"`
		} `yaml:"pipeline"`
		Output map[string]map[string]any `yaml:"output"`
	}
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err = d.Decode(&config); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("multiple native documents")
	}
	input, hasInput := config.Input["batz_file"]
	output, hasOutput := config.Output["batz_parquet_receipt"]
	if len(config.Input) != 1 || !hasInput || len(input) != 0 || len(config.Output) != 1 || !hasOutput || len(output) != 0 || config.Pipeline.Threads != 1 {
		return fmt.Errorf("unqualified native route: expected finite batz_file and sole batz_parquet_receipt sink")
	}
	if len(config.Pipeline.Processors) != 3 {
		return fmt.Errorf("unsupported local decode pipeline")
	}
	for i, processor := range config.Pipeline.Processors {
		if len(processor) != 1 {
			return fmt.Errorf("unsupported processor options")
		}
		if i == 1 {
			archive, ok := processor["unarchive"].(map[string]any)
			if !ok || len(archive) != 1 || archive["format"] != "json_array" {
				return fmt.Errorf("expected native JSON array expansion")
			}
		} else if _, ok := processor["mapping"].(string); !ok {
			return fmt.Errorf("expected native mapping")
		}
	}
	return nil
}

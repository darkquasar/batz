package run

import (
	"batz/internal/contracts"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func fixtureContract(t *testing.T, root string) contracts.ExecutionContract {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest contracts.SourcePipeline
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest.Execution("local_raw")
}

package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeCollectorReceivesReservedEnvironmentAndReadonlyContext(t *testing.T) {
	root := nativeContent(t)
	config := filepath.Join(root, "collector/configs/cloudtrail/local-records.yaml")
	b, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	condition := `"${BATZ_STAGE_DIR:}" == "" || "${BATZ_SOURCE_PATH:}" == "" || "${BATZ_RUN_ID:}" == "" || "${BATZ_BINDING_ID:}" == "" || "${BATZ_BATCH_ID:}" == "" || "${BATZ_ATTEMPT_ID:}" == ""`
	changed := strings.Replace(string(b), `root = if this.type()`, `root = if `+condition+` { throw("missing-handoff") } else if this.type()`, 1)
	if changed == string(b) {
		t.Fatal("mutation missing")
	}
	if err = os.WriteFile(config, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	input, err := filepath.Abs("../../fixtures/cloudtrail/api-read.json")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	report := runFixture(t, root, workspace, input)
	contextPath := filepath.Join(workspace, ".batz/parsed", string(*report.ParsedRevision), "context.json")
	stat, err := os.Stat(contextPath)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm()&0222 != 0 {
		t.Fatalf("context document is writable: %v", stat.Mode())
	}
}

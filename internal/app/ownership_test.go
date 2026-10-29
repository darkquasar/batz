package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestBusyWorkspaceStartsNoAcquisition(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	artifact := filepath.Join(workspace, ".batz")
	if err = os.Mkdir(artifact, 0700); err != nil {
		t.Fatal(err)
	}
	lease, err := os.OpenFile(filepath.Join(artifact, "owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err = syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--input", "path=" + filepath.Join(root, "fixtures/cloudtrail/api-read.json")}, &out, &diagnostics)
	if code != 5 {
		t.Fatalf("expected busy: %d %s %s", code, &out, &diagnostics)
	}
	var report contracts.RunReport
	if err = json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Outcome != "blocked" || len(report.Diagnostics) != 1 || report.Diagnostics[0].Category != "busy" || report.Diagnostics[0].Code != "workspace-busy" {
		t.Fatalf("wrong busy diagnostic: %s", &out)
	}
	files, err := os.ReadDir(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != "owner.lock" {
		t.Fatalf("busy execution mutated workspace: %v", files)
	}
}

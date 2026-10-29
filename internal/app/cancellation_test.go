package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRunSignalHelper(t *testing.T) {
	if os.Getenv("BATZ_RUN_SIGNAL_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Exit(app.Execute(os.Args[i+1:], os.Stdout, os.Stderr))
		}
	}
	os.Exit(99)
}
func TestSignalCancellationPreservesParsedEvidenceAndReleasesOwner(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	args := []string{"-test.run=^TestRunSignalHelper$", "--", "run", filepath.Join(root, "pipelines/aws-cloudtrail.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + filepath.Join(root, "fixtures/cloudtrail/api-read.json")}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "BATZ_RUN_SIGNAL_HELPER=1")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for !ready {
		select {
		case e := <-done:
			t.Fatalf("run exited before active cancellation: %v %s %s", e, &out, &stderr)
		case <-deadline.C:
			cmd.Process.Kill()
			<-done
			t.Fatal("model launch not observed")
		case <-ticker.C:
			logs, _ := filepath.Glob(filepath.Join(workspace, ".batz/builds/*/model.log"))
			if len(logs) == 1 {
				info, e := os.Stat(logs[0])
				ready = e == nil && info.Size() > 0
			}
		}
	}
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		cmd.Process.Kill()
		<-done
		t.Fatal(err)
	}
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		<-done
		t.Fatal("cancelled run did not settle")
	}
	if cmd.ProcessState.ExitCode() != 130 {
		t.Fatalf("wrong cancellation exit: %v %s %s", err, &out, &stderr)
	}
	var report contracts.RunReport
	if err = json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Outcome != "cancelled" || !report.AcquisitionComplete || report.ParsedRevision == nil || report.NormalizedRevision != nil || len(report.Diagnostics) != 1 || report.Diagnostics[0].Code != "cancelled" {
		t.Fatalf("wrong cancelled evidence: %s", &out)
	}
	lease, err := os.OpenFile(filepath.Join(workspace, ".batz/owner.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err = syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("cancelled descendant retained lease: %v", err)
	}
}

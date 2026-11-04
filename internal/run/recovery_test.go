//go:build batz_app

package run

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"batz/internal/contracts"
)

func TestAppRecoveryProcess(t *testing.T) {
	path := os.Getenv("BATZ_RECOVERY_REQUEST")
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		os.Exit(90)
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		os.Exit(91)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	report, _ := execute(ctx, req, func(name string) {
		if name == os.Getenv("BATZ_RECOVERY_CHECKPOINT") {
			fmt.Println(name)
			var release [1]byte
			_, _ = os.Stdin.Read(release[:])
		}
	})
	cancel()
	_ = json.NewEncoder(os.Stdout).Encode(report)
	os.Exit(report.Exit)
}

// Acknowledged checkpoints make SIGKILL timing independent of engine startup.
func stopAtCheckpoint(t *testing.T, req Request, checkpoint string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.json")
	if err := writeJSON(path, req); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppRecoveryProcess$")
	cmd.Env = append(os.Environ(), "BATZ_RECOVERY_REQUEST="+path, "BATZ_RECOVERY_CHECKPOINT="+checkpoint)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(syscall.SIGTERM)
		_ = stdin.Close()
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	acknowledged := false
	defer func() {
		if acknowledged {
			_ = cmd.Process.Kill()
		} else {
			_ = cmd.Cancel()
		}
		_ = cmd.Wait()
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != checkpoint+"\n" {
		t.Fatalf("checkpoint %s: %v %s %s", checkpoint, err, line, &diagnostics)
	}
	acknowledged = true
	// The deferred kill/wait runs before this function returns to inspect state.
}

func recoveryRequest(t *testing.T) Request {
	t.Helper()
	root := recoveryContent(t)
	source := filepath.Join(t.TempDir(), "source.jsonl")
	data, err := os.ReadFile(filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "internal/app/testdata/synthetic")
	return Request{Contract: fixtureContract(t, root), Root: root, Workspace: t.TempDir(), Definition: filepath.Join(base, "pipeline.yaml"), Route: "local_raw", Source: source, Binding: "demo", NativeConfig: filepath.Join(base, "collector.yaml"), Project: root}

}

func TestAppDirectoryRecoveryFinishesFrozenSelectionBeforeLaterArrivals(t *testing.T) {
	req := recoveryRequest(t)
	data, err := os.ReadFile(req.Source)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for _, name := range []string{"A.jsonl", "B.jsonl"} {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	req.Source = directory
	stopAtCheckpoint(t, req, "after-receipt")
	// This arrival was not in the interrupted run's finite selection.
	if err := os.WriteFile(filepath.Join(directory, "C.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Execute(context.Background(), req)
	if err != nil || report.Outcome != "published" || report.Counts[0].Accepted != 4 || len(report.Inputs) != 2 {
		t.Fatalf("recovery failed to finish exact frozen membership: %+v %v", report, err)
	}
	stopAtCheckpoint(t, req, "after-parsed-commit")
	status, err := ReadStatus(req.Workspace, "")
	if err != nil || len(status.Run.Inputs) != 3 || status.Run.Counts[0].Accepted != 6 {
		t.Fatalf("later arrival was lost or earlier files reacquired: %+v %v", status, err)
	}
}

func TestAppRestartAtParsedCheckpointsCommitsOneBatch(t *testing.T) {
	for _, point := range []string{"after-receipt", "after-placement", "before-parsed-commit", "after-parsed-commit"} {
		t.Run(point, func(t *testing.T) {
			req := recoveryRequest(t)
			stopAtCheckpoint(t, req, point)
			root := artifactRoot(req)
			db, err := openState(root)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var before int
			if err := db.QueryRow("SELECT count(*) FROM parsed").Scan(&before); err != nil {
				t.Fatal(err)
			}
			want := 0
			if point == "after-parsed-commit" {
				want = 1
			}
			if before != want {
				t.Fatalf("uncommitted files became visible: %d", before)
			}
			if err := os.Remove(req.Source); err != nil {
				t.Fatal(err)
			}
			req.Retry = true
			stopAtCheckpoint(t, req, "after-parsed-commit")
			var count int
			var revision string
			if err := db.QueryRow("SELECT count(*) FROM parsed").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT parsed FROM bindings WHERE id='demo'").Scan(&revision); err != nil {
				t.Fatal(err)
			}
			if count != 1 || revision == "" {
				t.Fatalf("restart duplicated or lost B: count=%d revision=%s", count, revision)
			}
			var raw []byte
			if err := db.QueryRow("SELECT report FROM runs WHERE json_extract(report,'$.outcome')='parsed-committed' LIMIT 1").Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var committed, exported contracts.RunReport
			if err := json.Unmarshal(raw, &committed); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, "reports", string(committed.Identity.Run)+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &exported); err != nil {
				t.Fatal(err)
			}
			if exported.Outcome != committed.Outcome || exported.ParsedRevision == nil || *exported.ParsedRevision != *committed.ParsedRevision {
				t.Fatal("interrupted report export was not rebuilt from committed state")
			}
		})
	}
}

func TestAppReceiptIsNotReusableBeforeCleanCompletion(t *testing.T) {
	req := recoveryRequest(t)
	stopAtCheckpoint(t, req, "before-receipt")
	db, err := openState(artifactRoot(req))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var original string
	if err := db.QueryRow("SELECT id FROM runs").Scan(&original); err != nil {
		t.Fatal(err)
	}
	req.Retry = true
	report, err := Execute(context.Background(), req)
	if err == nil || report.AcquisitionComplete || report.ParsedRevision != nil {
		t.Fatalf("interrupted acquisition became completed evidence: %+v %v", report, err)
	}
	status, err := ReadStatus(req.Workspace, original)
	if err != nil {
		t.Fatal(err)
	}
	if status.Run.Outcome != "cancelled" || status.RetryEligible || len(status.Run.Diagnostics) != 1 || status.Run.Diagnostics[0].Code != "interrupted" {
		t.Fatalf("interrupted attempt lost its diagnostic: %+v", status)
	}
	req.Retry = false
	stopAtCheckpoint(t, req, "after-parsed-commit")
	var count int
	if err := db.QueryRow("SELECT count(*) FROM parsed").Scan(&count); err != nil || count != 1 {
		t.Fatalf("fresh acquisition did not resume safely: count=%d %v", count, err)
	}
}

func TestAppRestartAtNormalizedCheckpointsKeepsOnePublication(t *testing.T) {
	for _, point := range []string{"before-normalized-commit", "after-normalized-commit"} {
		t.Run(point, func(t *testing.T) {
			req := recoveryRequest(t)
			stopAtCheckpoint(t, req, point)
			db, err := openState(artifactRoot(req))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var count int
			if err := db.QueryRow("SELECT count(*) FROM normalized").Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 0
			if point == "after-normalized-commit" {
				want = 1
			}
			if count != want {
				t.Fatalf("uncommitted normalized files became current: %d", count)
			}
			if err := os.Remove(req.Source); err != nil {
				t.Fatal(err)
			}
			req.Retry = true
			stopAtCheckpoint(t, req, "after-normalized-commit")
			if err := db.QueryRow("SELECT count(*) FROM normalized").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("restart duplicated normalized publication: %d", count)
			}
			status, err := ReadStatus(req.Workspace, "")
			if err != nil {
				t.Fatal(err)
			}
			if status.CurrentNormalized == nil || status.Run.Outcome != "published" || status.Run.NormalizedRevision == nil || *status.CurrentNormalized != *status.Run.NormalizedRevision {
				t.Fatalf("committed normalized status is incoherent: %+v", status)
			}
		})
	}
}

func TestAppRecoveryWorkerHandlesTermination(t *testing.T) {
	req := recoveryRequest(t)
	path := filepath.Join(t.TempDir(), "request.json")
	if err := writeJSON(path, req); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppRecoveryProcess$")
	cmd.Env = append(os.Environ(), "BATZ_RECOVERY_REQUEST="+path, "BATZ_RECOVERY_CHECKPOINT=before-receipt")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || line != "before-receipt\n" {
		t.Fatalf("worker checkpoint: %q %v", line, err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	var report contracts.RunReport
	if cmd.ProcessState.ExitCode() != 130 || json.Unmarshal(data, &report) != nil || report.Outcome != "cancelled" {
		t.Fatalf("worker did not settle through cancellation: exit=%d %s", cmd.ProcessState.ExitCode(), data)
	}
	lease, err := os.OpenFile(filepath.Join(artifactRoot(req), "owner.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("terminated worker left a child holding ownership: %v", err)
	}
}

// Copy only this lane's app-owned contract. Native assets are private to ci/test.
func recoveryContent(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"internal/app/testdata/synthetic", "runtime/sqlmesh", "config.yaml", "models/synthetic-jsonl"} {
		err := filepath.WalkDir(filepath.Join(source, name), func(path string, entry os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			relative, e := filepath.Rel(source, path)
			if e != nil {
				return e
			}
			target := filepath.Join(root, relative)
			if entry.IsDir() {
				return os.MkdirAll(target, 0700)
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				return e
			}
			return os.WriteFile(target, data, 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	local := os.Getenv("BATZ_TEST_RUNTIME")
	if local == "" {
		t.Fatal("native lane requires ci/test app")
	}
	data, err := os.ReadFile(filepath.Join(local, "dependencies.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime/dependencies.lock.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(local, filepath.Join(root, "runtime/.local")); err != nil {
		t.Fatal(err)
	}
	return root
}

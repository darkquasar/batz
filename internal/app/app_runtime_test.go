//go:build batz_app

package app_test

import (
	"batz/internal/app"
	"batz/internal/contracts"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAppRunSignalHelper(t *testing.T) {
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

func TestAppSignalCancellationPreservesParsedEvidenceAndReleasesOwner(t *testing.T) {
	root := nativeContent(t)
	var err error
	workspace := t.TempDir()
	args := []string{"-test.run=^TestAppRunSignalHelper$", "--", "run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl")}
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
				ready = e == nil && info.Mode().IsRegular()
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

func TestAppCancellationDuringNativeDiscoverySettlesDescendants(t *testing.T) {
	root := nativeContent(t)
	ready := filepath.Join(t.TempDir(), "ready.json")
	script := `import json,os,subprocess,time
child=subprocess.Popen(['/bin/sleep','120'])
with open(` + fmt.Sprintf("%q", ready) + `, 'w') as f: json.dump([os.getpid(),child.pid],f)
time.sleep(120)
`
	if err := os.WriteFile(filepath.Join(root, "runtime/sqlmesh/runner.py"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl")
	workspace := filepath.Join(t.TempDir(), "workspace")
	args := []string{"-test.run=^TestAppRunSignalHelper$", "--", "run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "BATZ_RUN_SIGNAL_HELPER=1")
	var out, diagnostics bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostics
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	settled := false
	defer func() {
		if !settled {
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var pids []int
	for len(pids) == 0 {
		select {
		case <-done:
			settled = true
			t.Fatalf("discovery exited before readiness: %s %s", &out, &diagnostics)
		case <-deadline.C:
			t.Fatal("native discovery readiness not observed")
		case <-ticker.C:
			if raw, err := os.ReadFile(ready); err == nil {
				_ = json.Unmarshal(raw, &pids)
			}
		}
	}
	defer func() {
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		settled = true
	case <-time.After(5 * time.Second):
		t.Fatal("discovery cancellation did not settle")
	}
	if cmd.ProcessState.ExitCode() != 130 {
		t.Fatalf("discovery cancellation exit=%d %s %s", cmd.ProcessState.ExitCode(), &out, &diagnostics)
	}
	for _, pid := range pids {
		state, _ := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
		if value := strings.TrimSpace(string(state)); value != "" && !strings.HasPrefix(value, "Z") {
			t.Fatalf("native discovery descendant %d survived: %s", pid, value)
		}
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("preflight mutated execution workspace: %v", err)
	}
}

func TestAppBusyWorkspaceStartsNoAcquisition(t *testing.T) {
	root := nativeContent(t)
	var err error
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
	code := app.Execute([]string{"run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--input", "path=" + filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl")}, &out, &diagnostics)
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

func TestAppUndeclaredModelSelectionRefusedBeforeAcquisition(t *testing.T) {
	root := nativeContent(t)
	definition := filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml")
	b, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"probe.missing", "cloudtrail.api_activity"} {
		t.Run(model, func(t *testing.T) {
			changed := strings.Replace(string(b), "probe.findings", model, 1)
			if changed == string(b) {
				t.Fatal("test mutation missing")
			}
			if err := os.WriteFile(definition, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			workspace := filepath.Join(t.TempDir(), "workspace")
			input := filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl")
			var out, diagnostics bytes.Buffer
			code := app.Execute([]string{"run", definition, "--project", root, "--route", "local_raw", "--workspace", workspace, "--input", "path=" + input}, &out, &diagnostics)
			if code != 2 {
				t.Fatalf("model selection ignored: exit=%d %s %s", code, &diagnostics, &out)
			}
			if model == "cloudtrail.api_activity" && !strings.Contains(diagnostics.String(), "unsupported-input-scope") {
				t.Fatalf("foreign input diagnostic lost: %s", &diagnostics)
			}
			if _, err := os.Stat(workspace); !os.IsNotExist(err) {
				t.Fatalf("preflight mutated workspace: %v", err)
			}
		})
	}
}

func TestAppSharedRecoveryJourney(t *testing.T) {
	root := nativeContent(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/app/testdata/synthetic/events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "source.json")
	if err := os.WriteFile(input, bytes.SplitAfterN(data, []byte("\n"), 2)[0], 0600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	// The output depends on an undeclared FULL side branch. Membership changes
	// within one processed interval must refresh that branch as well.
	modelPath := filepath.Join(root, "models/synthetic-jsonl/findings.sql")
	outputSQL, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	outputSQL = bytes.Replace(outputSQL, []byte("SELECT\n"), []byte("SELECT\n h.members AS shared_count,\n"), 1)
	outputSQL = bytes.Replace(outputSQL, []byte("CROSS JOIN batz_input.build_context c"), []byte("CROSS JOIN batz_input.build_context c CROSS JOIN probe.member_count h"), 1)
	if err := os.WriteFile(modelPath, outputSQL, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(modelPath), "member_count.sql"), []byte("MODEL (name probe.member_count, kind FULL); SELECT count(*)::BIGINT AS members FROM batz_input.parsed;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(root, "models/shared")
	if err := os.MkdirAll(shared, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(filepath.Dir(modelPath), "member_count.sql"), filepath.Join(shared, "member_count.sql")); err != nil {
		t.Fatal(err)
	}
	poison := filepath.Join(root, "models/aws-cloudtrail/unrelated.sql")
	if err := os.WriteFile(poison, []byte("MODEL (name cloudtrail.unrelated, kind FULL); SELECT error('unrelated model executed') AS value FROM probe.member_count;"), 0600); err != nil {
		t.Fatal(err)
	}
	prior := runSharedFixture(t, root, workspace, input)
	assertNoModelState := func() {
		t.Helper()
		paths, err := filepath.Glob(filepath.Join(workspace, ".batz/builds/*/model-state"))
		if err != nil || len(paths) != 0 {
			t.Fatalf("disposable state retained: %v %v", paths, err)
		}
	}
	assertNoModelState()
	finishReader := holdReader(t, root, workspace, *prior.Catalog)
	// An uncommitted closed file must never become selected SQL membership.
	parsed, err := os.ReadFile(filepath.Join(workspace, ".batz/parsed", string(*prior.ParsedRevision), "parsed.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".batz/parsed/uncommitted/parsed.parquet"), parsed, 0600); err != nil {
		t.Fatal(err)
	}
	other := runSharedFixture(t, root, workspace, input, "--binding-id", "other")
	assertNativeResources(t, prior)
	var records bytes.Buffer
	for i := range 6 {
		fmt.Fprintf(&records, "{\"id\":\"member-%d\",\"observed_at\":\"2020-01-01T00:00:00.000000001\",\"subject\":\"invented\"}\n", i)
	}
	if err := os.WriteFile(input, records.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "models/synthetic-jsonl/required_checks.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := "MODEL (name probe.required_checks, kind FULL); SELECT 0::BIGINT AS wrong_column;\n"
	if wrong == string(b) {
		t.Fatal("test did not mutate native SQL")
	}
	if err = os.WriteFile(path, []byte(wrong), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := app.Execute([]string{"run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &out, &diagnostics)
	var failed contracts.RunReport
	if err = json.Unmarshal(out.Bytes(), &failed); err != nil {
		t.Fatal(err)
	}
	if code == 0 || failed.Outcome != "normalization-failed" || !failed.AcquisitionComplete || failed.ParsedRevision == nil || failed.NormalizedRevision == nil || *failed.NormalizedRevision != *prior.NormalizedRevision {
		t.Fatalf("wrong instant published or evidence lost: exit=%d report=%s diagnostics=%s", code, &out, &diagnostics)
	}
	assertNoModelState()
	if code != 4 || len(failed.Diagnostics) != 1 || failed.Diagnostics[0].Category != "assertion" || failed.Diagnostics[0].Code != "required-check" {
		t.Fatalf("wrong assertion classification: %d %s", code, &out)
	}
	if got := string(query(t, root, *prior.Catalog, "SELECT count(*) FROM demo.findings")); got != "[[1]]" {
		t.Fatal(got)
	}
	var status, statusErrors bytes.Buffer
	if code := app.Execute([]string{"status", "--workspace", workspace, "--run-id", string(failed.Identity.Run)}, &status, &statusErrors); code != 0 {
		t.Fatalf("failed run status must be readable: exit=%d %s %s", code, &status, &statusErrors)
	}
	var observed struct {
		Run           contracts.RunReport `json:"run"`
		RetryEligible bool                `json:"retry_eligible"`
		ParsedFiles   []contracts.FileRef `json:"parsed_files"`
	}
	if err := json.Unmarshal(status.Bytes(), &observed); err != nil {
		t.Fatal(err)
	}
	if observed.Run.Outcome != failed.Outcome || observed.Run.Exit != 4 || !observed.RetryEligible || len(observed.ParsedFiles) != 2 {
		t.Fatalf("status lost committed B or failure: %s", &status)
	}
	// A stopped workspace can be copied and restored coherently at the same
	// root, including SQLMesh state. Reuse this recovery journey to prove it.
	backup := t.TempDir()
	copyTree(t, filepath.Join(workspace, ".batz"), filepath.Join(backup, ".batz"))
	if err := os.Rename(filepath.Join(workspace, ".batz"), filepath.Join(t.TempDir(), "original")); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join(backup, ".batz"), filepath.Join(workspace, ".batz"))
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	tools, err := filepath.EvalSymlinks(filepath.Join(root, "runtime/.local"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "runtime/.local")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime/.local"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(tools, "venv"), filepath.Join(root, "runtime/.local/venv")); err != nil {
		t.Fatal(err)
	}
	var retried, retryErrors bytes.Buffer
	code = app.Execute([]string{"retry", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--run-id", string(failed.Identity.Run), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &retried, &retryErrors)
	var recovered contracts.RunReport
	if err := json.Unmarshal(retried.Bytes(), &recovered); err != nil {
		t.Fatalf("retry report: %v %s %s", err, &retried, &retryErrors)
	}
	if code != 0 || recovered.ParsedRevision == nil || *recovered.ParsedRevision != *failed.ParsedRevision || recovered.Outcome != "published" {
		t.Fatalf("B was not retried from committed evidence: %d %s %s", code, &retried, &retryErrors)
	}
	if _, collected := recovered.Resources["collection"]; collected {
		t.Fatal("model-only retry collected source data")
	}
	if got := string(query(t, root, *recovered.Catalog, "SELECT count(*) FROM demo.findings")); got != "[[7]]" {
		t.Fatal(got)
	}
	if got := string(query(t, root, *prior.Catalog, "SELECT count(*) FROM demo.findings")); got != "[[1]]" {
		t.Fatal(got)
	}
	if got := string(query(t, root, *other.Catalog, "SELECT count(*) FROM other.findings")); got != "[[1]]" {
		t.Fatal(got)
	}
	if got := string(query(t, root, *recovered.Catalog, `SELECT count(*),count(DISTINCT batz.origins[1].source_event_id),count(DISTINCT batz.input_digest),count(DISTINCT batz.evaluation_nonce),min(year("@timestamp")) FROM demo.findings`)); got != "[[7,7,1,1,2020]]" {
		t.Fatalf("exact fresh membership: %s", got)
	}
	if got := string(query(t, root, *recovered.Catalog, "SELECT DISTINCT shared_count FROM demo.findings")); got != "[[7]]" {
		t.Fatalf("undeclared FULL model kept stale membership: %s", got)
	}
	finishReader(*recovered.Catalog, 7)
	testDir := filepath.Join(root, "tests/synthetic-jsonl")
	if err := os.MkdirAll(testDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "test_never_execute.yaml"), []byte("invalid contributor test: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "README.md"), []byte("contributor notes"), 0600); err != nil {
		t.Fatal(err)
	}
	definition := filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml")
	manifest, err := os.ReadFile(definition)
	if err != nil {
		t.Fatal(err)
	}
	manifest = bytes.ReplaceAll(manifest, []byte("c3f364dcc3a4dfc28c5c675e4ef6635b312ae988e1bf30d5d1639bb4573f566b"), []byte(strings.Repeat("a", 64)))
	if err := os.WriteFile(definition, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(poison, []byte("MODEL (name cloudtrail.unrelated, kind FULL); SELECT error('B-only edit executed') AS value;"), 0600); err != nil {
		t.Fatal(err)
	}
	retried.Reset()
	retryErrors.Reset()
	code = app.Execute([]string{"retry", definition, "--run-id", string(failed.Identity.Run), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}, &retried, &retryErrors)
	var unchanged contracts.RunReport
	if err := json.Unmarshal(retried.Bytes(), &unchanged); err != nil {
		t.Fatal(err)
	}
	if code != 0 || unchanged.NormalizedRevision == nil || *unchanged.NormalizedRevision != *recovered.NormalizedRevision {
		t.Fatalf("test/example edit rebuilt production: %d %s %s", code, &retried, &retryErrors)
	}
	if _, modeled := unchanged.Resources["model"]; modeled {
		t.Fatal("test-only content reran production models")
	}

}

func runSharedFixture(t *testing.T, root, workspace, input string, extra ...string) contracts.RunReport {
	t.Helper()
	args := []string{"run", filepath.Join(root, "internal/app/testdata/synthetic/pipeline.yaml"), "--project", root, "--route", "local_raw", "--workspace", workspace, "--binding-id", "demo", "--input", "path=" + input}
	args = append(args, extra...)
	var out, diagnostics bytes.Buffer
	code := app.Execute(args, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("shared run: %d %s %s", code, &out, &diagnostics)
	}
	var report contracts.RunReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	return report
}

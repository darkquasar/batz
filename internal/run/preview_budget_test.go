//go:build batz_app

package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"batz/internal/scratch"
)

func TestAppPreviewBudgetIncludesWritableSiblingPaths(t *testing.T) {
	root := os.Getenv("BATZ_TEST_RUNTIME")
	if root == "" {
		t.Fatal("native lane requires ci/test app")
	}
	private, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(private, "stage")
	if err = os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(root, "venv/bin/python")
	profile, err := scratch.Profile(private, []string{filepath.Join(root, "python"), filepath.Join(root, "venv")})
	if err != nil {
		t.Fatal(err)
	}
	script := `import os,sys,time
from pathlib import Path
Path(sys.argv[1]+'/writer.pid').write_text(str(os.getpid()))
with open(sys.argv[1]+'/unmeasured.bin','wb') as f:
 for i in range(64):
  f.write(bytes(8192));f.flush();time.sleep(.02)
`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	usage, err := sandboxedChild(private, profile, ctx, resourceLease(t, private), stage, filepath.Join(stage, "child.log"), childBudget{64 << 10, []string{stage}}, nil, python, "-I", "-c", script, private)
	if err == nil || !strings.Contains(err.Error(), "scratch-limit") {
		t.Fatalf("writable sibling was unmeasured: usage=%+v err=%v", usage, err)
	}
	info, e := os.Stat(filepath.Join(private, "unmeasured.bin"))
	if e != nil {
		t.Fatal(e)
	}
	if info.Size() >= 64*8192 {
		t.Fatal("sibling writer completed despite threshold")
	}
	t.Logf("sibling writer stopped at %d bytes; observed %d bytes", info.Size(), usage.ScratchPeakBytes)
}

func TestAppPrivateInspectionBudgetIncludesWritableSiblingPaths(t *testing.T) {
	runtime := os.Getenv("BATZ_TEST_RUNTIME")
	if runtime == "" {
		t.Fatal("native lane requires ci/test app")
	}
	private, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(private, "discovery")
	if err = os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	profile, err := scratch.Profile(private, []string{filepath.Join(runtime, "python"), filepath.Join(runtime, "venv")})
	if err != nil {
		t.Fatal(err)
	}
	// A sparse sibling exceeds the finite budget without writing a GiB. The
	// inspector's nested working directory must not hide this writable path.
	sibling, err := os.Create(filepath.Join(private, "sibling.bin"))
	if err != nil {
		t.Fatal(err)
	}
	err = sibling.Truncate(Limits().ScratchBytes + 1)
	_ = sibling.Close()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(stage, "inspect.py")
	if err = os.WriteFile(script, []byte("print('inspection ran')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	usage, err := InspectPrivate(ctx, stage, private, profile, filepath.Join(runtime, "venv/bin/python"), script, filepath.Join(stage, "request.json"))
	if err == nil || !strings.Contains(err.Error(), "scratch-limit") || usage.ScratchPeakBytes <= Limits().ScratchBytes {
		t.Fatalf("private inspection ignored writable sibling: usage=%+v err=%v", usage, err)
	}
}

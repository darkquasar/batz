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

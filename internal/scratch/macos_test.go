//go:build batz_app

package scratch_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"batz/internal/scratch"
)

func TestAppNativeSandboxDeniesOutsideDataAndNetwork(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("native macOS qualification only")
	}
	root := os.Getenv("BATZ_TEST_RUNTIME")
	if root == "" {
		t.Fatal("native lane requires ci/test app")
	}
	private, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	forbidden := filepath.Join(t.TempDir(), "secret")
	if err = os.WriteFile(forbidden, []byte("invented"), 0600); err != nil {
		t.Fatal(err)
	}
	reads := []string{filepath.Join(root, "python"), filepath.Join(root, "venv")}
	profile, err := scratch.Profile(private, reads)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	python := filepath.Join(root, "venv/bin/python")
	if err = scratch.Qualify(ctx, profile, python, private, forbidden); err != nil {
		t.Fatal(err)
	}
	// DuckDB external readers and SQL COPY are denied by the same actual policy.
	script := `import duckdb,sys
c=duckdb.connect()
for query in ["SELECT * FROM read_text('"+sys.argv[1]+"')", "COPY (SELECT 1) TO '"+sys.argv[1]+"-copy'"]:
 try:c.execute(query).fetchall()
 except duckdb.Error:pass
 else:raise RuntimeError('SQL escaped sandbox')
print('SQL denied')`
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", "-p", profile, python, "-I", "-c", script, forbidden)
	cmd.Dir = private
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native SQL isolation: %v %s", err, output)
	}
	if string(output) != "SQL denied\n" {
		t.Fatalf("unexpected qualification: %s", output)
	}
}

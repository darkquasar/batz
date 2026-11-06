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
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		t.Skip("supported native targets only")
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
assert c.execute('SELECT 1').fetchone() == (1,)
for query in ["SELECT * FROM read_text('"+sys.argv[1]+"')", "COPY (SELECT 1) TO '"+sys.argv[1]+"-copy'"]:
 try:c.execute(query).fetchall()
 except duckdb.Error:pass
 else:raise RuntimeError('SQL escaped sandbox')
if sys.platform == 'linux':
 for path in ['/proc/meminfo', '/proc/self/status']:
  try:open(path).read()
  except PermissionError:pass
  else:raise RuntimeError('unrelated kernel file readable: ' + path)
print('SQL denied')`
	program, args, err := scratch.Command(profile, python, "-I", "-B", "-c", script, forbidden)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = private
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + private, "TMPDIR=" + private, "PYTHONDONTWRITEBYTECODE=1", "MAX_FORK_WORKERS=1", "SQLMESH__DISABLE_ANONYMIZED_ANALYTICS=true", "NO_COLOR=1"}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native SQL isolation: %v %s", err, output)
	}
	if string(output) != "SQL denied\n" {
		t.Fatalf("unexpected qualification: %s", output)
	}
}

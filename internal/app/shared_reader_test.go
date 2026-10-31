package app_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func holdReader(t *testing.T, root, workspace, catalog string) func(string, int64) {
	t.Helper()
	orphan := filepath.Join(workspace, ".batz/parsed/uncommitted")
	if err := os.MkdirAll(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	script := `import duckdb,json,sys
c=duckdb.connect(sys.argv[1],read_only=True)
assert c.execute("SELECT count(*) FROM demo.findings").fetchone()[0]==1
print("READY",flush=True)
path=sys.stdin.readline().strip()
c.execute("ATTACH '"+path.replace("'","''")+"' AS revision_b (READ_ONLY)")
print(json.dumps(c.execute("SELECT (SELECT count(*) FROM demo.findings),(SELECT count(*) FROM revision_b.demo.findings)").fetchall(),separators=(',',':')),flush=True)
c.close()`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime/.local/venv/bin/python"), "-I", "-c", script, catalog)
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	reader := bufio.NewReader(stdout)
	ready, err := reader.ReadString('\n')
	if err != nil || ready != "READY\n" {
		t.Fatalf("reader setup: %v %s %s", err, ready, &diagnostics)
	}
	return func(next string, rows int64) {
		t.Helper()
		if _, err := io.WriteString(stdin, next+"\n"); err != nil {
			t.Fatal(err)
		}
		got, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("held reader: %v %s", err, &diagnostics)
		}
		if got != fmt.Sprintf("[[1,%d]]\n", rows) {
			t.Fatalf("mixed revision or stray membership: %s", got)
		}
		err = cmd.Wait()
		waited = true
		if err != nil {
			t.Fatalf("reader: %v %s", err, &diagnostics)
		}
	}
}

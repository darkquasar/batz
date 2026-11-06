//go:build linux && amd64 && batz_app

package scratch_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"batz/internal/scratch"
)

func linuxRuntime(t *testing.T) (string, string) {
	t.Helper()
	root := os.Getenv("BATZ_TEST_RUNTIME")
	if root == "" {
		t.Fatal("native lane requires ci/test app")
	}
	private, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root, private
}

func TestAppLinuxSandboxStartsBeforeVenvHooks(t *testing.T) {
	root, private := linuxRuntime(t)
	venv := filepath.Join(t.TempDir(), "venv")
	site := filepath.Join(venv, "lib/python3.12/site-packages")
	if err := os.MkdirAll(site, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(venv, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(root, "venv/pyvenv.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(venv, "pyvenv.cfg"), config, 0600); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(venv, "bin/python")
	if err = os.Symlink(filepath.Join(root, "venv/bin/python"), python); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "outside-startup-write")
	hook := "import pathlib; pathlib.Path(" + strconv.Quote(marker) + ").write_text('escaped')\n"
	if err = os.WriteFile(filepath.Join(site, "invented.pth"), []byte(hook), 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := scratch.Profile(private, []string{filepath.Join(root, "python"), venv})
	if err != nil {
		t.Fatal(err)
	}
	// The real child suppresses hooks to isolate the launcher's startup behavior.
	program, args, err := scratch.Command(profile, python, "-I", "-S", "-B", "-c", "print('child ran')")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, program, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("confined child: %v %s", err, output)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("venv startup wrote outside confinement: %v", err)
	}
}

func TestAppLinuxSandboxDeniesReadonlyFileMetadataIoctl(t *testing.T) {
	root, private := linuxRuntime(t)
	file := filepath.Join(t.TempDir(), "admitted-readonly-data")
	if err := os.WriteFile(file, []byte("invented"), 0600); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(root, "venv/bin/python")
	baseline := `import array,fcntl,sys
with open(sys.argv[1],'rb') as f:
 flags=array.array('L',[0]);fcntl.ioctl(f,0x80086601,flags,True)
 original=flags[0];flags[0]=original ^ 64
 fcntl.ioctl(f,0x40086602,flags)
 flags[0]=original;fcntl.ioctl(f,0x40086602,flags)
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, python, "-I", "-B", "-c", baseline, file).CombinedOutput(); err != nil {
		t.Fatalf("host must support the invented file metadata oracle: %v %s", err, output)
	}
	profile, err := scratch.Profile(private, []string{filepath.Join(root, "python"), filepath.Join(root, "venv"), file})
	if err != nil {
		t.Fatal(err)
	}
	script := `import array,fcntl,sys
with open(sys.argv[1],'rb') as f:
 try:fcntl.ioctl(f,0x40086602,array.array('L',[64]))
 except PermissionError:pass
 else:raise RuntimeError('outside metadata ioctl permitted')
`
	program, args, err := scratch.Command(profile, python, "-I", "-B", "-c", script, file)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, program, args...).CombinedOutput(); err != nil {
		t.Fatalf("metadata must be denied on an admitted read-only file: %v %s", err, output)
	}
}

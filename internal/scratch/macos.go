// Package scratch qualifies and describes the native offline execution boundary.
package scratch

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Profile allows writes only below a private root and reads from admitted inputs.
// Network, Mach service lookup (including credential services), and other reads
// remain denied. The caller owns scratch and the lifetime of all children.
func Profile(root string, reads []string) (string, error) {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return linuxProfile(root, reads)
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return "", fmt.Errorf("native offline sandbox unavailable on this target")
	}
	quote := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	var p strings.Builder
	p.WriteString(`(version 1)(deny default)(allow process*)(allow sysctl-read)(allow file-read-metadata)(allow file-read-data (literal "/"))(allow file-read* (subpath "/System") (subpath "/usr/lib") (subpath "/usr/share") (literal "/dev/urandom") (literal "/dev/null"))`)
	for _, path := range append([]string{root}, reads...) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&p, "(allow file-read* (subpath %s))", quote(resolved))
	}
	fmt.Fprintf(&p, `(allow file-write* (subpath %s) (literal "/dev/null"))`, quote(root))
	return p.String(), nil
}

// Command wraps one native program with the platform's inherited kernel policy.
func Command(profile, program string, args ...string) (string, []string, error) {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		var policy linuxPolicy
		if err := json.Unmarshal([]byte(profile), &policy); err != nil || policy.Python == "" {
			return "", nil, fmt.Errorf("invalid Linux sandbox profile")
		}
		return policy.Python, append([]string{"-I", "-S", "-B", "-c", linuxLauncher, profile, program}, args...), nil
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return "", nil, fmt.Errorf("native offline sandbox unavailable on this target")
	}
	return "/usr/bin/sandbox-exec", append([]string{"-p", profile, program}, args...), nil
}

// Qualify checks the actual host/child rather than treating a policy as proof.
func Qualify(ctx context.Context, profile, python, root, forbidden string) error {
	script := `import os,socket,sys
p=sys.argv[1]
with open(os.path.join(p,'sandbox-positive'),'w') as f:f.write('scratch')
try:
 open(sys.argv[2]).read()
except PermissionError:pass
else:raise RuntimeError('outside read permitted')
try:
 open(sys.argv[2]+'-write','w').write('bad')
except PermissionError:pass
else:raise RuntimeError('outside write permitted')
try:
 socket.socket().connect(("127.0.0.1",9))
except PermissionError:pass
else:raise RuntimeError('network permitted')
print('qualified')
`
	program, args, err := Command(profile, python, "-I", "-B", "-c", script, root, forbidden)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = root
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + root, "TMPDIR=" + root, "PYTHONDONTWRITEBYTECODE=1"}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("native sandbox qualification: %w: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "qualified" {
		return fmt.Errorf("native sandbox qualification missing evidence")
	}
	return nil
}

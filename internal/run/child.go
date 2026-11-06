package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"batz/internal/contracts"
)

// processRSS samples the child's process group plus current descendants. RSS is
// summed per process; shared pages can be counted more than once. It is telemetry,
// not a physical-memory quota or a sandbox against deliberately escaping code.
func processRSS(rootPID int) (int64, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,pgid=,rss=,stat=").Output()
	if err != nil {
		return 0, 0, err
	}
	type process struct {
		pid, parent, group int
		rss                int64
		zombie             bool
	}
	processes := []process{}
	selected := map[int]bool{rootPID: true}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 5 {
			return 0, 0, fmt.Errorf("unexpected process measurement")
		}
		pid, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		group, e3 := strconv.Atoi(fields[2])
		rss, e4 := strconv.ParseInt(fields[3], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || rss < 0 {
			return 0, 0, fmt.Errorf("invalid process measurement")
		}
		processes = append(processes, process{pid, parent, group, rss * 1024, strings.HasPrefix(fields[4], "Z")})
		if group == rootPID {
			selected[pid] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, p := range processes {
			if selected[p.parent] && !selected[p.pid] {
				selected[p.pid] = true
				changed = true
			}
		}
	}
	var total int64
	count := 0
	for _, p := range processes {
		if selected[p.pid] && !p.zombie {
			total += p.rss
			count++
		}
	}
	return total, count, nil
}

type childBudget struct {
	ScratchBytes int64
	Roots        []string
}

func scratchBytes(roots []string) (int64, error) {
	var total int64
	seen := map[string]bool{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, e error) error {
			if os.IsNotExist(e) {
				return nil
			}
			if e != nil {
				return e
			}
			if seen[path] {
				return nil
			}
			seen[path] = true
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in private scratch")
			}
			if entry.IsDir() {
				return nil
			}
			info, e := entry.Info()
			if os.IsNotExist(e) {
				return nil
			}
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("nonregular private scratch file")
			}
			if info.Size() > int64(^uint64(0)>>1)-total {
				return fmt.Errorf("scratch size overflow")
			}
			total += info.Size()
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}
func settleProcessGroup(pid int) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
			return nil
		}
		_, count, e := processRSS(pid)
		if e == nil && count == 0 {
			return nil
		} // Exited zombies hold no writer or lease.
		if time.Now().After(deadline) {
			return fmt.Errorf("child process group did not settle")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// unsettledProcessError prevents callers from removing files a child may still use.
type unsettledProcessError struct{ error }

func child(ctx context.Context, lease *os.File, dir, logpath string, budget childBudget, environment []string, program string, args ...string) (usage contracts.ResourceUsage, err error) {
	usage.SampleIntervalMS = 100
	start := time.Now()
	defer func() { usage.WallMS = time.Since(start).Milliseconds() }()
	checkScratch := func() error {
		size, e := scratchBytes(budget.Roots)
		if e != nil {
			return fmt.Errorf("scratch measurement failed: %w", e)
		}
		if size > usage.ScratchPeakBytes {
			usage.ScratchPeakBytes = size
		}
		if size > budget.ScratchBytes {
			return fmt.Errorf("scratch-limit: observed %d bytes exceeds %d-byte threshold", size, budget.ScratchBytes)
		}
		return nil
	}
	if err = checkScratch(); err != nil {
		return usage, err
	}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir, "PYTHONDONTWRITEBYTECODE=1", "SQLMESH__DISABLE_ANONYMIZED_ANALYTICS=true", "NO_COLOR=1"}
	cmd.Env = append(cmd.Env, environment...)
	cmd.ExtraFiles = []*os.File{lease}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	log, err := os.OpenFile(logpath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return usage, err
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		return usage, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stop := func(cause error) error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		if e := settleProcessGroup(cmd.Process.Pid); e != nil {
			return unsettledProcessError{fmt.Errorf("%v; %w", cause, e)}
		}
		return cause
	}
	var sum, peak int64
	sample := func() error {
		rss, count, e := processRSS(cmd.Process.Pid)
		if e != nil {
			usage.UnavailableSamples++
			message := e.Error()
			usage.MeasurementError = &message
		} else if count > 0 {
			usage.Samples++
			sum += rss
			if rss > peak {
				peak = rss
			}
			average := sum / usage.Samples
			usage.AverageRSSBytes = &average
			copyPeak := peak
			usage.PeakRSSBytes = &copyPeak
		}
		return checkScratch()
	}
	if e := sample(); e != nil {
		return usage, stop(e)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err = <-done:
			// Never return success while a descendant can still write outputs.
			if !errors.Is(syscall.Kill(-cmd.Process.Pid, 0), syscall.ESRCH) {
				_, count, e := processRSS(cmd.Process.Pid)
				if count > 0 || e != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					if err == nil {
						err = fmt.Errorf("native process left unsettled descendants")
					}
				}
				if e = settleProcessGroup(cmd.Process.Pid); e != nil {
					return usage, unsettledProcessError{e}
				}
			}
			if e := checkScratch(); e != nil {
				return usage, e
			}
			if err != nil {
				return usage, fmt.Errorf("%s failed: %w (log %s)", filepath.Base(program), err, logpath)
			}
			return usage, log.Sync()
		case <-ticker.C:
			if e := sample(); e != nil {
				return usage, stop(e)
			}
		}
	}
}

// sandboxedChild retains the same supervision and resource accounting while the
// kernel policy is inherited by the native process and all its descendants.
func sandboxedChild(privateRoot, profile string, ctx context.Context, lease *os.File, dir, logpath string, budget childBudget, environment []string, program string, args ...string) (contracts.ResourceUsage, error) {
	if profile != "" {
		if privateRoot == "" {
			return contracts.ResourceUsage{}, errors.New("missing private scratch root")
		}
		// The kernel permits writes throughout privateRoot, including siblings
		// of the current stage. Every writable path belongs to the same budget.
		budget.Roots = []string{privateRoot}
		environment = append(environment, "MAX_FORK_WORKERS=1")
		args = append([]string{"-p", profile, program}, args...)
		program = "/usr/bin/sandbox-exec"
	}
	return child(ctx, lease, dir, logpath, budget, environment, program, args...)
}

// InspectPrivate runs a Batz-owned inspector with the same process settlement and
// resource supervision as collection/modeling. A nonempty profile additionally
// confines descendant writes with the macOS kernel sandbox. Discovery uses an
// empty profile after content admission; its writes are not kernel-confined.
func InspectPrivate(ctx context.Context, root, profile, python, script, request string) (contracts.ResourceUsage, error) {
	lease, err := os.OpenFile(filepath.Join(root, "inspection.lease"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return contracts.ResourceUsage{}, err
	}
	defer lease.Close()
	return sandboxedChild(root, profile, ctx, lease, root, filepath.Join(root, "inspection.log"), childBudget{Limits().ScratchBytes, []string{root}}, nil, python, "-I", "-B", script, request)
}

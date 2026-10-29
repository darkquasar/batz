package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This helper is a real bounded process tree, not a simulated process inspector.
func TestResourceProcessHelper(t *testing.T) {
	mode := os.Getenv("BATZ_RESOURCE_PROBE")
	if mode == "" {
		return
	}
	switch mode {
	case "ram-parent", "disk-parent":
		bytes := make([]byte, 32<<20)
		for i := range bytes {
			bytes[i] = 1
		}
		childMode := strings.Replace(mode, "parent", "leaf", 1)
		cmd := exec.Command(os.Args[0], "-test.run=^TestResourceProcessHelper$")
		cmd.Env = append(os.Environ(), "BATZ_RESOURCE_PROBE="+childMode)
		cmd.ExtraFiles = []*os.File{os.NewFile(3, "owner")}
		if err := cmd.Run(); err != nil {
			os.Exit(9)
		}
		runtime.KeepAlive(bytes)
	case "ram-leaf":
		bytes := make([]byte, 64<<20)
		for i := range bytes {
			bytes[i] = 1
		}
		if err := os.WriteFile("leaf.pid", []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(8)
		}
		time.Sleep(650 * time.Millisecond)
		runtime.KeepAlive(bytes)
	case "disk-leaf":
		if err := os.WriteFile("leaf.pid", []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(8)
		}
		file, err := os.Create("growing.bin")
		if err != nil {
			os.Exit(8)
		}
		block := make([]byte, 8192)
		for i := 0; i < 64; i++ {
			if _, err = file.Write(block); err != nil {
				os.Exit(8)
			}
			time.Sleep(20 * time.Millisecond)
		}
		file.Close()
	case "sleep":
		time.Sleep(5 * time.Second)
	default:
		os.Exit(7)
	}
	os.Exit(0)
}
func resourceLease(t *testing.T, dir string) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, "owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}
func TestRAMMeasurementIncludesDescendantsWithoutCutoff(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	usage, err := child(ctx, resourceLease(t, dir), dir, filepath.Join(dir, "child.log"), childBudget{1 << 30, []string{dir}}, []string{"BATZ_RESOURCE_PROBE=ram-parent"}, os.Args[0], "-test.run=^TestResourceProcessHelper$")
	if err != nil {
		t.Fatal(err)
	}
	if usage.Samples < 2 || usage.PeakRSSBytes == nil || *usage.PeakRSSBytes < 80<<20 || usage.AverageRSSBytes == nil || *usage.AverageRSSBytes <= 0 || usage.UnavailableSamples != 0 {
		t.Fatalf("missing actual tree memory: %+v", usage)
	}
	t.Logf("tree RSS samples=%d average=%d peak=%d", usage.Samples, *usage.AverageRSSBytes, *usage.PeakRSSBytes)
}
func TestScratchWatchdogStopsAndSettlesActualDescendants(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	usage, err := child(ctx, resourceLease(t, dir), dir, filepath.Join(dir, "child.log"), childBudget{64 << 10, []string{dir}}, []string{"BATZ_RESOURCE_PROBE=disk-parent"}, os.Args[0], "-test.run=^TestResourceProcessHelper$")
	if err == nil || !strings.Contains(err.Error(), "scratch-limit") {
		t.Fatalf("scratch bound was not enforced: %+v %v", usage, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "leaf.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, count, err := processRSS(pid)
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("descendant did not settle")
		}
		time.Sleep(25 * time.Millisecond)
	}
	info, err := os.Stat(filepath.Join(dir, "growing.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= 64*8192 {
		t.Fatal("writer completed despite resource failure")
	}
	t.Logf("writer stopped at %d bytes for %d-byte threshold", info.Size(), 64<<10)
}
func TestChildDeadlineRemainsOperationalFailure(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := child(ctx, resourceLease(t, dir), dir, filepath.Join(dir, "child.log"), childBudget{1 << 30, []string{dir}}, []string{"BATZ_RESOURCE_PROBE=sleep"}, os.Args[0], "-test.run=^TestResourceProcessHelper$")
	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Fatal(fmt.Sprint("deadline not enforced: ", err))
	}
}

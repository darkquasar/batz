//go:build batz_app

package run

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestAppOwnerProcess(t *testing.T) {
	mode := os.Getenv("BATZ_OWNER_HELPER")
	if mode == "" {
		return
	}
	if mode == "leaf" {
		lease := os.NewFile(3, "inherited-owner")
		connection, err := net.Dial("unix", os.Getenv("BATZ_OWNER_SOCKET"))
		if err != nil {
			os.Exit(90)
		}
		_, _ = fmt.Fprintln(connection, os.Getpid())
		var release [1]byte
		_, _ = connection.Read(release[:])
		_ = lease.Close()
		_ = connection.Close()
		os.Exit(0)
	}
	root := os.Getenv("BATZ_OWNER_ROOT")
	lease, err := os.OpenFile(filepath.Join(root, "owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil || syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		os.Exit(91)
	}
	_, err = child(context.Background(), lease, root, filepath.Join(root, "child.log"), childBudget{Limits().ScratchBytes, []string{root}}, []string{"BATZ_OWNER_HELPER=leaf", "BATZ_OWNER_SOCKET=" + os.Getenv("BATZ_OWNER_SOCKET")}, os.Args[0], "-test.run=^TestAppOwnerProcess$")
	_ = lease.Close()
	if err != nil {
		os.Exit(92)
	}
	os.Exit(0)
}

func TestAppParentDeathKeepsExclusionUntilChildSettles(t *testing.T) {
	dir, err := os.MkdirTemp("/private/tmp", "batz-owner-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, "ready.sock"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, ".batz")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAppOwnerProcess$")
	cmd.Env = append(os.Environ(), "BATZ_OWNER_HELPER=owner", "BATZ_OWNER_ROOT="+root, "BATZ_OWNER_SOCKET="+listener.Addr().String())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	connection, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var pid int
	if _, err := fmt.Fscanln(bufio.NewReader(connection), &pid); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	waited = true
	report, err := Execute(context.Background(), Request{Workspace: dir, Binding: "demo"})
	if err == nil || report.Exit != 5 {
		t.Fatalf("surviving child lost workspace exclusion: %+v %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(root, "state.sqlite")); !os.IsNotExist(err) {
		t.Fatal("busy contender changed coordinator state")
	}
	if _, err := connection.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	var eof [1]byte
	if n, err := connection.Read(eof[:]); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("child did not settle: %d %v", n, err)
	}
	pid = 0
	lease, err := os.OpenFile(filepath.Join(root, "owner.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("settled child retained exclusion: %v", err)
	}
}

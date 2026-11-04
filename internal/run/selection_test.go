package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"batz/internal/contracts"
)

func TestMutationAfterSelectionCannotCommitOrStartACollector(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.json")
	before := []byte(`{"Records":[{"value":"before"}]}`)
	after := []byte(`{"Records":[{"value":"after!"}]}`)
	if err := os.WriteFile(input, before, 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Workspace: t.TempDir(), Source: input, Binding: "demo", Contract: contracts.ExecutionContract{Framing: "records-json"}}
	report, err := execute(context.Background(), req, func(point string) {
		if point == "after-selection" {
			if err := os.WriteFile(input, after, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(input, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err == nil || !strings.Contains(err.Error(), "changed between selection") || report.ParsedRevision != nil || len(report.Resources) != 0 || report.Acquisition == nil || len(report.Acquisition.Acquired) != 0 {
		t.Fatalf("changed input was acquired: %+v %v", report, err)
	}
}

func TestSymlinkInputIsRefusedBeforeAnyCollector(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, []byte(`{"Records":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "source.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	report, err := Execute(context.Background(), Request{Workspace: t.TempDir(), Source: link, Binding: "demo"})
	if err == nil || !strings.Contains(err.Error(), "symlinks") || report.ParsedRevision != nil || len(report.Resources) != 0 {
		t.Fatalf("symlink started collection: %+v %v", report, err)
	}
}

func TestChangedBindingSourceVersionIsRefusedBeforeCollection(t *testing.T) {
	req := Request{Workspace: t.TempDir(), Source: "/nonexistent-batz-source", Binding: "demo", Contract: contracts.ExecutionContract{Pipeline: "test", Version: 1, Framing: "jsonl"}}
	_, _ = Execute(context.Background(), req)
	req.Contract.Version = 2
	report, err := Execute(context.Background(), req)
	if err == nil || report.Exit != 2 || !strings.Contains(err.Error(), "source/version") || len(report.Resources) != 0 {
		t.Fatalf("incompatible source version reused: %+v %v", report, err)
	}
}

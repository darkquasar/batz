package run

import (
	"os"
	"path/filepath"
	"testing"

	"batz/internal/contracts"
)

func TestBuildCountsMustAccountForCommittedInputs(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "evidence")
	if err := os.WriteFile(path, []byte("closed evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	ref, err := fileRef(root, path, 1)
	if err != nil {
		t.Fatal(err)
	}
	input := committedInventory{revision: "parsed", digest: "digest", receipts: []contracts.CollectionReceipt{{Counts: contracts.Counts{Accepted: 2}}}}
	built := builtRevision{Checks: true, InputRevision: input.revision, InputDigest: input.digest, AttemptID: "attempt", Nonce: "nonce", Catalog: ref, Rejects: []contracts.FileRef{ref}, Counts: contracts.Counts{Boundary: "normalize", Accepted: 1, Rejected: 1}}
	if err := verifyBuilt(root, built, input, "attempt", "nonce"); err != nil {
		t.Fatal(err)
	}
	for _, counts := range []contracts.Counts{
		{Boundary: "normalize", Accepted: 0, Rejected: 1},
		{Boundary: "normalize", Accepted: 2, Rejected: 1},
		{Boundary: "normalize", Accepted: -1, Rejected: 3},
		{Boundary: "parse", Accepted: 1, Rejected: 1},
		{Boundary: "normalize", Accepted: 2},
	} {
		built.Counts = counts
		if err := verifyBuilt(root, built, input, "attempt", "nonce"); err == nil {
			t.Fatalf("inconsistent accounting accepted: %+v", counts)
		}
	}
}

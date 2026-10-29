package run

import (
	"batz/collector/plugins"
	"batz/internal/contracts"
	"encoding/json"
	"github.com/parquet-go/parquet-go"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func nativeReceipt(t *testing.T) (string, plugins.Request) {
	t.Helper()
	stage, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	source := []byte(`{"Records":[{"eventID":"one"},{"eventID":"two"}]}`)
	input := contracts.InputRef{ID: hash([]byte("input")), Locator: "synthetic://receipt-test", SHA256: hash(source), Bytes: int64(len(source))}
	request := plugins.Request{Identity: contracts.Identity{Binding: "demo", Run: "run", Batch: "batch", Attempt: "attempt"}, Input: input, Stage: stage, Source: filepath.Join(stage, "input.json"), AcquiredAt: time.Now().UTC().Format(time.RFC3339Nano), Limits: Limits()}
	if err = os.WriteFile(request.Source, source, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(stage, "request.json"), encode(request), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(root, "runtime/.local/batz-collector"), filepath.Join(root, "collector/configs/cloudtrail/local-records.yaml"), filepath.Join(stage, "request.json"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("real collector: %v %s", err, out)
	}
	if _, err := verifyReceipt(stage, request); err != nil {
		t.Fatalf("native control: %v", err)
	}
	return stage, request
}
func TestReceiptMustDescribeEveryActualSourceRecord(t *testing.T) {
	for _, mutation := range []string{"duplicate-key", "trailing-document", "duplicate-ordinal", "wrong-observation", "omitted-record", "invalid-parquet", "wrong-attempt", "escaping-path", "missing-receipt"} {
		t.Run(mutation, func(t *testing.T) {
			stage, request := nativeReceipt(t)
			path := filepath.Join(stage, "receipt.json")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var receipt contracts.CollectionReceipt
			if err = json.Unmarshal(b, &receipt); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "duplicate-key":
				b = []byte(strings.Replace(string(b), `"complete":true`, `"complete":false,"complete":true`, 1))
			case "trailing-document":
				b = append(b, []byte(` {}`)...)
			case "wrong-attempt":
				receipt.Identity.Attempt = "old"
				b = encode(receipt)
			case "escaping-path":
				receipt.ParsedFiles[0].Path = "../escape.parquet"
				b = encode(receipt)
			case "missing-receipt":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
			default:
				dataPath := filepath.Join(stage, receipt.ParsedFiles[0].Path)
				rows, err := parquet.ReadFile[plugins.Record](dataPath)
				if err != nil {
					t.Fatal(err)
				}
				switch mutation {
				case "duplicate-ordinal":
					rows[1].Ordinal = rows[0].Ordinal
				case "wrong-observation":
					rows[1].ObservationID = hash([]byte("not-this-source"))
				case "omitted-record":
					rows = rows[:1]
					receipt.Counts.Accepted = 1
				}
				if err = parquet.WriteFile(dataPath, rows); err != nil {
					t.Fatal(err)
				}
				if mutation == "invalid-parquet" {
					if err = os.WriteFile(dataPath, []byte("not parquet"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				receipt.ParsedFiles[0], err = fileRef(stage, dataPath, int64(len(rows)))
				if err != nil {
					t.Fatal(err)
				}
				b = encode(receipt)
			}
			if mutation != "missing-receipt" {
				if err = os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = verifyReceipt(stage, request); err == nil {
				t.Fatal("untrusted receipt was accepted")
			}
		})
	}
}

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"batz/internal/contracts"
	"github.com/parquet-go/parquet-go"
	"github.com/warpstreamlabs/bento/public/service"
)

func TestRejectedElementsRetainActualPayloadAndReason(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "input.json")
	data := []byte(`{"Records":[null,42]}`)
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	job := Job{Request: Request{Stage: root, Source: source, Limits: contracts.Limits{Records: 2}}}
	input := fileInput{job: &job, data: data}
	ctx := context.Background()
	_, ack, err := input.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := ack(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := input.Read(ctx); !errors.Is(err, service.ErrEndOfInput) {
		t.Fatal(err)
	}
	output := sink{job: &job}
	for ordinal, payload := range []string{"null", "42"} {
		message, err := json.Marshal(map[string]any{"payload": payload, "ordinal": ordinal})
		if err != nil {
			t.Fatal(err)
		}
		if err := output.Write(ctx, service.NewMessage(message)); err != nil {
			t.Fatal(err)
		}
	}
	if err := job.Finish(); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[Record](filepath.Join(root, "rejects.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Payload != "null" || rows[1].Payload != "42" || rows[0].RejectReason == "" || rows[1].RejectReason == "" {
		t.Fatalf("rejected evidence was fabricated or lost: %+v", rows)
	}
}

func TestSourcePayloadComparisonIgnoresObjectOrderWithoutLosingNumbers(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "input.json")
	if err := os.WriteFile(source, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	job := Job{Request: Request{Stage: root, Source: source, Limits: contracts.Limits{Records: 2}}, eof: true, ack: true}
	output := sink{job: &job}
	for ordinal, payload := range []string{`{"z":{"b":9007199254740993,"a":1},"a":[2,1]}`, ` {"a":[2,1],"z":{"a":1,"b":9007199254740993}} `} {
		message, err := json.Marshal(map[string]any{"payload": payload, "ordinal": ordinal})
		if err != nil {
			t.Fatal(err)
		}
		if err := output.Write(context.Background(), service.NewMessage(message)); err != nil {
			t.Fatal(err)
		}
	}
	if err := job.Finish(); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[Record](filepath.Join(root, "parsed.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Payload != rows[1].Payload || rows[0].Payload != `{"a":[2,1],"z":{"a":1,"b":9007199254740993}}` {
		t.Fatalf("object ordering or exact number lost: %+v", rows)
	}
}

func TestIncompleteCollectionCannotProduceReceipt(t *testing.T) {
	for _, tc := range []struct {
		name string
		eof  bool
		ack  bool
	}{{name: "no-eof", ack: true}, {name: "no-ack", eof: true}, {name: "neither"}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			job := Job{Request: Request{Stage: root}, eof: tc.eof, ack: tc.ack}
			if err := job.Finish(); err == nil {
				t.Fatal("incomplete collection produced success")
			}
			if _, err := os.Stat(filepath.Join(root, "receipt.json")); !os.IsNotExist(err) {
				t.Fatal("incomplete collection wrote a receipt")
			}
		})
	}
}

func TestEmptyCompletedSinkKeepsItsNativeParquetSchema(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "input.json")
	if err := os.WriteFile(source, []byte(`{"Records":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	job := Job{Request: Request{Stage: root, Source: source}, eof: true, ack: true}
	if err := job.Finish(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(root, "parsed.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := parquet.OpenFile(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	if actual.NumRows() != 0 || actual.Schema().String() != parquet.SchemaOf(new(Record)).String() {
		t.Fatalf("empty completion lost typed schema: %s", actual.Schema())
	}
	raw, err := os.ReadFile(filepath.Join(root, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt contracts.CollectionReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete || receipt.Counts.Accepted != 0 || receipt.ParsedFiles[0].Rows != 0 {
		t.Fatalf("invalid explicit empty receipt: %+v", receipt)
	}
}

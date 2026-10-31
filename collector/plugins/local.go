// Package plugins supplies the finite file boundary and the sole parsed sink.
// Native Bento processors own JSON decoding and Records expansion.
package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"batz/internal/contracts"
	"github.com/parquet-go/parquet-go"
	"github.com/warpstreamlabs/bento/public/service"
)

var ErrUnsupportedShape = errors.New("unsupported-shape: native source decoding failed")

type Request struct {
	Framing    string             `json:"framing"`
	Identity   contracts.Identity `json:"identity"`
	Input      contracts.InputRef `json:"input"`
	Stage      string             `json:"stage"`
	Source     string             `json:"source"`
	AcquiredAt string             `json:"acquired_at"`
	Limits     contracts.Limits   `json:"limits"`
}
type Record struct {
	ObservationID  string `parquet:"observation_id"`
	BindingID      string `parquet:"binding_id"`
	RunID          string `parquet:"run_id"`
	BatchID        string `parquet:"batch_id"`
	AttemptID      string `parquet:"attempt_id"`
	InputID        string `parquet:"input_id"`
	InputLocator   string `parquet:"input_locator"`
	InputSHA256    string `parquet:"input_sha256"`
	Ordinal        int64  `parquet:"record_ordinal"`
	AcquiredAt     string `parquet:"acquired_at"`
	Representation string `parquet:"payload_representation"`
	Payload        string `parquet:"payload"`
	ParseStatus    string `parquet:"parse_status"`
	RejectReason   string `parquet:"reject_reason"`
}
type Job struct {
	Request       Request
	mu            sync.Mutex
	eof, ack      bool
	failure       error
	rows, rejects []Record
}

func (j *Job) fail(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failure == nil {
		j.failure = err
	}
}
func (j *Job) Err() error { j.mu.Lock(); defer j.mu.Unlock(); return j.failure }
func Register(j *Job) error {
	spec := service.NewConfigSpec()
	if err := service.RegisterInput("batz_file", spec, func(*service.ParsedConfig, *service.Resources) (service.Input, error) {
		b, err := os.ReadFile(j.Request.Source)
		if err != nil {
			return nil, err
		}
		if int64(len(b)) > j.Request.Limits.InputBytes {
			return nil, fmt.Errorf("input byte limit exceeded")
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != j.Request.Input.SHA256 {
			return nil, fmt.Errorf("input snapshot integrity")
		}
		if _, err := SourceRecords(b, j.Request.Framing); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnsupportedShape, err)
		}
		return &fileInput{job: j, data: b}, nil
	}); err != nil {
		return err
	}
	return service.RegisterOutput("batz_parquet_receipt", spec, func(*service.ParsedConfig, *service.Resources) (service.Output, int, error) {
		return &sink{job: j}, 1, nil
	})
}

type fileInput struct {
	job  *Job
	data []byte
	read bool
}

func (i *fileInput) Connect(context.Context) error { return nil }
func (i *fileInput) Close(context.Context) error   { return nil }
func (i *fileInput) Read(context.Context) (*service.Message, service.AckFunc, error) {
	if i.read {
		i.job.mu.Lock()
		i.job.eof = true
		i.job.mu.Unlock()
		return nil, nil, service.ErrEndOfInput
	}
	i.read = true
	return service.NewMessage(i.data), func(_ context.Context, err error) error {
		i.job.mu.Lock()
		defer i.job.mu.Unlock()
		i.job.ack = err == nil
		if err != nil {
			i.job.failure = err
		}
		return nil
	}, nil
}

type sink struct{ job *Job }

func (s *sink) Connect(context.Context) error { return nil }
func (s *sink) Write(_ context.Context, m *service.Message) error {
	j := s.job
	if err := m.GetError(); err != nil {
		j.fail(fmt.Errorf("%w: %v", ErrUnsupportedShape, err))
		return nil
	}
	b, err := m.AsBytes()
	if err != nil {
		j.fail(err)
		return nil
	}
	var value struct {
		Payload string `json:"payload"`
		Ordinal int64  `json:"ordinal"`
	}
	if err = json.Unmarshal(b, &value); err != nil {
		j.fail(err)
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if int64(len(j.rows)+len(j.rejects)) >= j.Request.Limits.Records {
		j.failure = fmt.Errorf("record limit exceeded")
		return nil
	}
	idbytes, _ := json.Marshal([]any{j.Request.Identity.Binding, j.Request.Identity.Batch, j.Request.Input.ID, value.Ordinal})
	id := sha256.Sum256(idbytes)
	r := Record{ObservationID: hex.EncodeToString(id[:]), BindingID: string(j.Request.Identity.Binding), RunID: string(j.Request.Identity.Run), BatchID: string(j.Request.Identity.Batch), AttemptID: string(j.Request.Identity.Attempt), InputID: j.Request.Input.ID, InputLocator: j.Request.Input.Locator, InputSHA256: j.Request.Input.SHA256, Ordinal: value.Ordinal, AcquiredAt: j.Request.AcquiredAt, Representation: "reconstructed-json", Payload: value.Payload, ParseStatus: "accepted"}
	var record map[string]json.RawMessage
	if err = json.Unmarshal([]byte(value.Payload), &record); err != nil || record == nil {
		r.ParseStatus = "rejected"
		r.RejectReason = "record must be a JSON object"
		j.rejects = append(j.rejects, r)
	} else {
		j.rows = append(j.rows, r)
	}
	return nil
}
func (s *sink) Close(context.Context) error { return nil }

// Finish runs after the real stream drained cleanly; it never turns cancellation into completion.
func (j *Job) Finish() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failure != nil {
		return j.failure
	}
	if !j.eof || !j.ack {
		return fmt.Errorf("finite input did not complete and acknowledge")
	}
	if err := parquet.WriteFile(filepath.Join(j.Request.Stage, "parsed.parquet"), j.rows); err != nil {
		return err
	}
	parsed, err := fileRef(j.Request.Stage, "parsed.parquet", int64(len(j.rows)))
	if err != nil {
		return err
	}
	rejectFiles := []contracts.FileRef{}
	if len(j.rejects) > 0 {
		if err := parquet.WriteFile(filepath.Join(j.Request.Stage, "rejects.parquet"), j.rejects); err != nil {
			return err
		}
		r, err := fileRef(j.Request.Stage, "rejects.parquet", int64(len(j.rejects)))
		if err != nil {
			return err
		}
		rejectFiles = append(rejectFiles, r)
	}
	source, err := fileRef(j.Request.Stage, "input.json", 0)
	if err != nil {
		return err
	}
	receipt := contracts.CollectionReceipt{APIVersion: contracts.APIVersion, Kind: "CollectionReceipt", Identity: j.Request.Identity, Inputs: []contracts.InputRef{j.Request.Input}, SourceFiles: []contracts.FileRef{source}, ParsedFiles: []contracts.FileRef{parsed}, RejectFiles: rejectFiles, Counts: contracts.Counts{Boundary: "parse", Accepted: int64(len(j.rows)), Rejected: int64(len(j.rejects))}, Complete: true}
	b, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(j.Request.Stage, "receipt.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	dir, err := os.Open(j.Request.Stage)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func fileRef(root, path string, rows int64) (contracts.FileRef, error) {
	p := filepath.Join(root, path)
	f, err := os.OpenFile(p, os.O_RDWR, 0600)
	if err != nil {
		return contracts.FileRef{}, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return contracts.FileRef{}, err
	}
	if err = f.Close(); err != nil {
		return contracts.FileRef{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return contracts.FileRef{}, err
	}
	sum := sha256.Sum256(b)
	return contracts.FileRef{Path: path, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(b)), Rows: rows}, nil
}

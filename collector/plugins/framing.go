package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// SourceRecords independently verifies the two qualified native framing shapes.
// This is adapter fidelity, separate from Bento's actual decoding/expansion.
func SourceRecords(data []byte, framing string) ([]json.RawMessage, error) {
	switch framing {
	case "records-json":
		var document struct {
			Records []json.RawMessage `json:"Records"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&document); err != nil || document.Records == nil {
			return nil, fmt.Errorf("unsupported-shape: expected Records JSON")
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return nil, fmt.Errorf("unsupported-shape: trailing source document")
		}
		return document.Records, nil
	case "jsonl":
		records := []json.RawMessage{}
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			if !json.Valid(line) {
				return nil, fmt.Errorf("unsupported-shape: invalid JSONL record")
			}
			records = append(records, append(json.RawMessage(nil), line...))
		}
		return records, nil
	default:
		return nil, fmt.Errorf("unsupported source framing %q", framing)
	}
}

package plugins

import "testing"

func TestQualifiedFramingRetainsOnlyExplicitSourceRecords(t *testing.T) {
	for _, tc := range []struct {
		name, framing, source string
		count                 int
	}{
		{name: "wrapped", framing: "records-json", source: `{"Records":[{"unknown":"kept"},null]}`, count: 2},
		{name: "empty-wrapped", framing: "records-json", source: `{"Records":[]}`},
		{name: "jsonl", framing: "jsonl", source: "{\"unknown\":\"kept\"}\n\n{\"n\":2}\n", count: 2},
		{name: "empty-jsonl", framing: "jsonl", source: "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records, err := SourceRecords([]byte(tc.source), tc.framing)
			if err != nil || len(records) != tc.count {
				t.Fatalf("qualified framing: records=%s err=%v", records, err)
			}
		})
	}
}

func TestUnqualifiedOrTruncatedFramingFails(t *testing.T) {
	for _, tc := range []struct{ name, framing, source string }{
		{name: "unknown", framing: "unknown", source: `{}`},
		{name: "truncated-wrapper", framing: "records-json", source: `{"Records":[`},
		{name: "no-wrapper", framing: "records-json", source: `[{}]`},
		{name: "trailing-document", framing: "records-json", source: `{"Records":[]} {}`},
		{name: "truncated-jsonl", framing: "jsonl", source: "{}\n{"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := SourceRecords([]byte(tc.source), tc.framing); err == nil {
				t.Fatal("invalid framing accepted")
			}
		})
	}
}

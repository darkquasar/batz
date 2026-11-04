// Package contracts defines versioned wire data. Decoding does not confer trust.
// Publication owns verified receipts and committed inventory capabilities.
package contracts

import "encoding/json"

const APIVersion = "batz/v0alpha1"
const Profile = "batz-investigation/v0alpha1"
const ECSVersion = "9.5.0"

type BindingID string
type RunID string
type BatchID string
type AttemptID string
type RevisionID string
type MembershipDigest string

type Identity struct {
	Binding BindingID `json:"binding_id"`
	Run     RunID     `json:"run_id"`
	Batch   BatchID   `json:"batch_id"`
	Attempt AttemptID `json:"attempt_id"`
	Case    *string   `json:"case_id"`
}
type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Rows   int64  `json:"rows"`
}
type InputRef struct {
	ID      string `json:"id"`
	Locator string `json:"locator"`
	SHA256  string `json:"sha256"`
	Bytes   int64  `json:"bytes"`
}
type Counts struct {
	Boundary    string `json:"boundary"`
	Accepted    int64  `json:"accepted"`
	Rejected    int64  `json:"rejected"`
	Unsupported int64  `json:"unsupported"`
	Excluded    int64  `json:"excluded"`
}

// CollectionReceipt is untrusted subprocess output, not a verified capability.
type CollectionReceipt struct {
	APIVersion   string     `json:"apiVersion"`
	Kind         string     `json:"kind"`
	Identity     Identity   `json:"identity"`
	Inputs       []InputRef `json:"inputs"`
	SourceFiles  []FileRef  `json:"source_files"`
	ParsedFiles  []FileRef  `json:"parsed_files"`
	RejectFiles  []FileRef  `json:"reject_files"`
	Counts       Counts     `json:"counts"`
	Complete     bool       `json:"complete"`
	Continuation *string    `json:"continuation"`
}
type Origin struct {
	ObservationID  string     `json:"observation_id"`
	ParsedRevision RevisionID `json:"parsed_revision"`
	File           string     `json:"file"`
	Row            int64      `json:"row"`
}
type Dataset struct {
	Name    string    `json:"name"`
	ModelID *string   `json:"model_id"`
	Schema  string    `json:"schema"`
	Files   []FileRef `json:"files"`
	Rows    int64     `json:"rows"`
}
type PublishedInventory struct {
	Reconciliation *Reconciliation   `json:"reconciliation,omitempty"`
	APIVersion     string            `json:"apiVersion"`
	Kind           string            `json:"kind"`
	Revision       RevisionID        `json:"revision"`
	Identity       Identity          `json:"identity"`
	InputRevision  RevisionID        `json:"input_revision"`
	InputDigest    MembershipDigest  `json:"input_digest"`
	Inputs         []InputRef        `json:"inputs"`
	Profile        string            `json:"profile"`
	Content        map[string]string `json:"content"`
	Datasets       []Dataset         `json:"datasets"`
	SourceFiles    []FileRef         `json:"source_files"`
	RejectFiles    []FileRef         `json:"reject_files"`
	Counts         []Counts          `json:"counts"`
	Quality        string            `json:"quality"`
	Catalog        FileRef           `json:"catalog"`
}
type Limits struct {
	InputFiles           int64  `json:"input_files"`
	InputBytes           int64  `json:"input_bytes"`
	Records              int64  `json:"records"`
	CollectionSeconds    int64  `json:"collection_seconds"`
	ModelSeconds         int64  `json:"model_seconds"`
	CollectorMemoryBytes *int64 `json:"collector_memory_bytes"`
	ModelMemoryBytes     *int64 `json:"model_memory_bytes"`
	DuckDBThreads        int64  `json:"duckdb_threads"`
	ScratchBytes         int64  `json:"scratch_bytes"`
	CollectionSlots      int64  `json:"collection_slots"`
	ModelSlots           int64  `json:"model_slots"`
}
type ResourceUsage struct {
	ScratchPeakBytes   int64   `json:"scratch_peak_bytes"`
	AverageRSSBytes    *int64  `json:"average_rss_bytes"`
	PeakRSSBytes       *int64  `json:"peak_rss_bytes"`
	Samples            int64   `json:"samples"`
	UnavailableSamples int64   `json:"unavailable_samples"`
	SampleIntervalMS   int64   `json:"sample_interval_ms"`
	WallMS             int64   `json:"wall_ms"`
	MeasurementError   *string `json:"measurement_error"`
}
type Diagnostic struct {
	Category string `json:"category"`
	Code     string `json:"code"`
	Field    string `json:"field"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}
type ValueOrigin struct {
	Value  json.RawMessage `json:"value"`
	Source string          `json:"source"`
	Field  string          `json:"field"`
}
type Coverage struct {
	Mapped      int64 `json:"mapped"`
	Unsupported int64 `json:"unsupported"`
}

// InputAccounting describes this invocation's frozen selection, separately
// from Inputs, which describes the complete committed evidence history.
type InputAccounting struct {
	New      []InputRef `json:"new"`
	Changed  []InputRef `json:"changed"`
	Skipped  []InputRef `json:"skipped"`
	Replayed []InputRef `json:"replayed"`
	Acquired []InputRef `json:"acquired"`
}

type Reconciliation struct {
	Observations             int64 `json:"observations"`
	Variants                 int64 `json:"variants"`
	LogicalEvents            int64 `json:"logical_events"`
	ConflictingEvents        int64 `json:"conflicting_events"`
	UnidentifiedObservations int64 `json:"unidentified_observations"`
}
type RunReport struct {
	Acquisition         *InputAccounting         `json:"acquisition,omitempty"`
	Reconciliation      *Reconciliation          `json:"reconciliation,omitempty"`
	Resources           map[string]ResourceUsage `json:"resources"`
	APIVersion          string                   `json:"apiVersion"`
	Kind                string                   `json:"kind"`
	Identity            Identity                 `json:"identity"`
	Outcome             string                   `json:"outcome"`
	AcquisitionComplete bool                     `json:"acquisition_complete"`
	Quality             *string                  `json:"quality"`
	Coverage            Coverage                 `json:"coverage"`
	Strict              bool                     `json:"strict"`
	Exit                int                      `json:"exit"`
	ParsedRevision      *RevisionID              `json:"parsed_revision"`
	NormalizedRevision  *RevisionID              `json:"normalized_revision"`
	Catalog             *string                  `json:"catalog"`
	Inputs              []InputRef               `json:"inputs"`
	Counts              []Counts                 `json:"counts"`
	Limits              Limits                   `json:"limits"`
	Content             map[string]string        `json:"content"`
	Resolution          map[string]ValueOrigin   `json:"resolution"`
	Diagnostics         []Diagnostic             `json:"diagnostics"`
}
type Parameter struct {
	Default  any    `json:"default,omitempty" yaml:"default,omitempty"`
	Type     string `json:"type" yaml:"type"`
	Required bool   `json:"required" yaml:"required"`
	Env      string `json:"env" yaml:"env"`
}
type RouteInput struct {
	Framing string               `json:"framing" yaml:"framing"`
	Engine  string               `json:"engine" yaml:"engine"`
	Config  string               `json:"config" yaml:"config"`
	Params  map[string]Parameter `json:"params" yaml:"params"`
}
type RouteOutput struct {
	Parsed struct {
		Format string `json:"format" yaml:"format"`
	} `json:"parsed" yaml:"parsed"`
	Normalized struct {
		Models map[string]string `json:"models" yaml:"models"`
	} `json:"normalized" yaml:"normalized"`
}
type Route struct {
	Input  RouteInput  `json:"input" yaml:"input"`
	Output RouteOutput `json:"output" yaml:"output"`
}
type Example struct {
	ID           string `json:"id" yaml:"id"`
	Route        string `json:"route" yaml:"route"`
	Input        string `json:"input" yaml:"input"`
	SHA256       string `json:"sha256" yaml:"sha256"`
	Expectations string `json:"expectations" yaml:"expectations"`
}
type SourcePipeline struct {
	Explain struct {
		Examples []Example `json:"examples" yaml:"examples"`
	} `json:"explain,omitempty" yaml:"explain,omitempty"`
	APIVersion    string `json:"apiVersion" yaml:"apiVersion"`
	Kind          string `json:"kind" yaml:"kind"`
	Name          string `json:"name" yaml:"name"`
	Version       int    `json:"version" yaml:"version"`
	Normalization struct {
		Project  string `json:"project" yaml:"project"`
		Staging  string `json:"staging" yaml:"staging"`
		Evidence string `json:"evidence" yaml:"evidence"`
		Checks   string `json:"checks" yaml:"checks"`
	} `json:"normalization" yaml:"normalization"`
	Routes map[string]Route `json:"routes" yaml:"routes"`
}
type PipelineBinding struct {
	APIVersion string         `json:"apiVersion" yaml:"apiVersion"`
	Kind       string         `json:"kind" yaml:"kind"`
	ID         BindingID      `json:"id" yaml:"id"`
	Definition string         `json:"definition" yaml:"definition"`
	Version    int            `json:"version" yaml:"version"`
	Route      string         `json:"route" yaml:"route"`
	Input      map[string]any `json:"input" yaml:"input"`
}
type ExplainRoute struct {
	Name         string            `json:"name"`
	Engine       string            `json:"engine"`
	Config       string            `json:"config"`
	ParsedFormat string            `json:"parsed_format"`
	Models       map[string]string `json:"models"`
	Basis        string            `json:"basis"`
}
type ExplainReport struct {
	Stages      []ExplainStage         `json:"stages"`
	Structures  []ExplainStructure     `json:"structures"`
	Samples     []ExplainSample        `json:"samples"`
	Resolution  map[string]ValueOrigin `json:"resolution"`
	Runtime     map[string]any         `json:"runtime"`
	APIVersion  string                 `json:"apiVersion"`
	Kind        string                 `json:"kind"`
	Pipeline    string                 `json:"pipeline"`
	Version     int                    `json:"version"`
	Level       int                    `json:"level"`
	Project     string                 `json:"project"`
	Routes      []ExplainRoute         `json:"routes"`
	Assets      map[string]string      `json:"assets"`
	Diagnostics []Diagnostic           `json:"diagnostics"`
}

// ExecutionContract is the selected route's production-only identity. Example and
// test metadata are deliberately absent; native SQLMesh owns model discovery.
type ExecutionContract struct {
	APIVersion string            `json:"apiVersion"`
	Pipeline   string            `json:"pipeline"`
	Version    int               `json:"version"`
	Route      string            `json:"route"`
	Framing    string            `json:"framing"`
	Staging    string            `json:"staging"`
	Evidence   string            `json:"evidence"`
	Checks     string            `json:"checks"`
	Outputs    map[string]string `json:"outputs"`
}

func (p SourcePipeline) Execution(route string) ExecutionContract {
	r := p.Routes[route]
	outputs := make(map[string]string, len(r.Output.Normalized.Models))
	for k, v := range r.Output.Normalized.Models {
		outputs[k] = v
	}
	return ExecutionContract{APIVersion: "batz-execution/v0alpha1", Pipeline: p.Name, Version: p.Version, Route: route, Framing: r.Input.Framing, Staging: p.Normalization.Staging, Evidence: p.Normalization.Evidence, Checks: p.Normalization.Checks, Outputs: outputs}
}

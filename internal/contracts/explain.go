package contracts

// ExplainStage describes one boundary without implying that it has executed.
type ExplainStage struct {
	ID      string   `json:"id"`
	Route   string   `json:"route"`
	Kind    string   `json:"kind"`
	Inputs  []string `json:"inputs"`
	Native  []string `json:"native"`
	Outputs []string `json:"outputs"`
	Basis   string   `json:"basis"`
	Reason  string   `json:"reason"`
}

type ExplainField struct {
	Name       string         `json:"name"`
	NativeType string         `json:"native_type"`
	Nullable   *bool          `json:"nullable"`
	Encoding   string         `json:"encoding"`
	Fields     []ExplainField `json:"fields"`
}

type ExplainStructure struct {
	Stage  string         `json:"stage"`
	Basis  string         `json:"basis"`
	Fields []ExplainField `json:"fields"`
}

type ExplainSample struct {
	Stage   string `json:"stage"`
	Basis   string `json:"basis"`
	Records []any  `json:"records"`
	Rows    int64  `json:"rows"`
	Reason  string `json:"reason"`
}

package discover

// Kind classifies one mechanically observable pattern.  Kinds make no claim
// that a move, deletion, or new package boundary is correct.
type Kind string

const (
	ReceiverCluster Kind = "receiver-field-method-cluster"
	Forwarder       Kind = "trivial-forwarder"
	APIFamily       Kind = "api-family"
	FileCluster     Kind = "declaration-file-test-cluster"
	TestCluster     Kind = "test-reference-cluster"
	DuplicateBody   Kind = "alpha-identical-body"
	SwitchCaseShape Kind = "repeated-switch-case-shape"
	ImportCluster   Kind = "import-caller-cluster"
	DuplicateIndex  Kind = "duplicate-index-construction"
	CallerPackage   Kind = "caller-package-cluster"
)

// Position is an exact source coordinate. Paths are supplied by the caller
// and are sorted lexically with slash separators in reports.
type Position struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Symbol is a type-checked declaration identity where one exists.  Synthetic
// symbols (for an import or source relation) are explicitly prefixed.
type Symbol struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Position Position `json:"position"`
}

// Evidence is a narrow, machine-derived fact supporting a candidate.
type Evidence struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Count  int    `json:"count"`
}

// Candidate is stable sorted output for a parent to review and turn into a
// cutplan Intent. Confidence measures detection certainty, not architectural
// desirability: high means the syntactic/type fact is exact.
type Candidate struct {
	Kind       Kind       `json:"kind"`
	Key        string     `json:"key"`
	Package    string     `json:"package"`
	Symbols    []Symbol   `json:"symbols"`
	Positions  []Position `json:"positions"`
	Reasons    []string   `json:"reasons"`
	Confidence string     `json:"confidence"`
	Evidence   []Evidence `json:"evidence"`
}

// Report contains no mutable AST or type information, so callers cannot turn
// discovery into an accidental editing API.
type Report struct {
	Package    string      `json:"package"`
	Candidates []Candidate `json:"candidates"`
}

// SurveyInput is a non-authoritative request for semantic evidence. It names
// selected resolved source declarations and a desired destination, but cannot
// express operations, verification rules, commands, or a Lock.
type SurveyInput struct {
	Symbols     []string `json:"symbols"`
	Containment []string `json:"containment,omitempty"`
	Destination string   `json:"destination"`
}

// Proposal is read-only review material. Rows marked ambiguous or
// unrepresentable must be resolved by the orchestrator before an Intent may
// be authored; this type has no conversion to Lock or apply input.
type Proposal struct {
	Kind              string   `json:"kind"`
	Destination       string   `json:"destination"`
	ReferenceClosure  []string `json:"reference_closure"`
	BindingCandidates []string `json:"binding_candidates"`
	ImportCandidates  []string `json:"import_candidates"`
	Read              []string `json:"read"`
	Write             []string `json:"write"`
	Residue           []string `json:"residue_denominator"`
	Ambiguous         []string `json:"ambiguous,omitempty"`
}

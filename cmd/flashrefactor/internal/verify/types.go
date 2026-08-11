package verify

import "github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"

// SourceFile is an executor-supplied source unit. Package is the canonical Go
// import path that owns this file, not its package-clause spelling.
type SourceFile struct {
	Path    string
	Package string
	Source  []byte
}

// SourceMap binds each exact repository-relative path to its supplied source
// bytes and canonical owning import path. Map identity makes a duplicate path
// unrepresentable; validation also requires the embedded Path to agree.
type SourceMap map[string]SourceFile

// Snapshot is a complete structural view for one side of a cut. Verify never
// loads omitted files.  Its import specifications are derived from the exact
// supplied source bytes; callers cannot smuggle a second, aggregate import
// authority alongside those bytes.
type Snapshot struct {
	Sources SourceMap
}

// ImportSpec is the exact syntactic import identity in one consumer file.
// Alias is the literal import name, with "" denoting the ordinary unaliased
// form. Name is deliberately absent: a package's declared name is semantic
// evidence owned by the typed resolver, not something this parser guesses.
type ImportSpec struct {
	Consumer string
	Path     string
	Alias    string
}

// ImportDelta records the exact imports removed and added in one consumer.
// Both lists are canonically sorted in a successful Report.
type ImportDelta struct {
	Consumer string
	Removed  []ImportSpec
	Added    []ImportSpec
}

// ImportEdge is the package graph projected from parsed post-cut import
// specifications. It is report evidence only; it is never caller input.
type ImportEdge struct {
	From string
	To   string
}

// ExternalEvidence is a typed semantic result supplied by the semantic
// verifier. Structural verification only checks its identity and successful
// disposition; it never re-resolves diagnostics or object references.
type ExternalEvidence struct {
	Kind   cutplan.Gate
	Passed bool
	Digest string
}

// GateEvidence commits the successful evidence that discharged one requested
// gate. Semantic gates retain the exact resolver-produced digest; structural
// gates retain the deterministic post-source digest they examined.
type GateEvidence struct {
	Gate   cutplan.Gate
	Digest string
}

// GateDisposition gives every requested lock gate exactly one owner. Gates
// backed by structural checks must use Structural; semantic gates must carry
// their separately-produced External evidence.
type GateDisposition struct {
	Gate     cutplan.Gate
	External *ExternalEvidence
}

// Request contains all facts the pure verifier may observe.
type Request struct {
	Before Snapshot
	After  Snapshot

	// Imports is the complete, flattened set of reviewed cutplan.Import
	// declarations. The verifier compares each route to the import-spec delta
	// of its named consumer file. It does not derive routes from the graph.
	Imports []cutplan.Import

	RequestedGates []cutplan.Gate
	Dispositions   []GateDisposition
}

// Report says which explicitly requested gates executed. It is deliberately
// useful to callers/tests proving an unrequested semantic gate was untouched.
type Report struct {
	Executed     []cutplan.Gate
	ImportDeltas []ImportDelta
	ImportGraph  []ImportEdge
	PostDigest   string
	Evidence     []GateEvidence
	Digest       string
}

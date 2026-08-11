package grammarproof

import "github.com/wippyai/go-lua/program/internal/grammarproof/astcodec"

type liveProduction struct {
	key       string
	reduction int
}

// FieldState is an observed parser-output field state. It is a cold trace
// encoding only: semantic requirements convert it to closed field-specific
// states before comparing it to a Program law. The alias keeps parser traces,
// ObserveSource, and the declaration-derived codec on one exact wire type.
type FieldState = astcodec.FieldState

const (
	FieldStateInvalid  = astcodec.FieldStateInvalid
	FieldStateAbsent   = astcodec.FieldStateAbsent
	FieldStatePresent  = astcodec.FieldStatePresent
	FieldStateEmpty    = astcodec.FieldStateEmpty
	FieldStateNonEmpty = astcodec.FieldStateNonEmpty
	FieldStateFalse    = astcodec.FieldStateFalse
	FieldStateTrue     = astcodec.FieldStateTrue
	FieldStateZero     = astcodec.FieldStateZero
	FieldStateNonZero  = astcodec.FieldStateNonZero
)

// ASTField is one exact exported parser-output field. Name is an AST source
// identifier; Value is retained only for closed scalar/enum discriminants.
type ASTField = astcodec.Field

// ASTOccurrence identifies one AST value emitted by a concrete parser
// reduction in one accepted source. It is an observed witness, never a
// requirement denominator.
type ASTOccurrence = astcodec.Occurrence

// SemanticTrace is one temporary-parser observation, resolved to the exact
// parser grammar alternative after tracing. It never participates in normal
// parsing, binding, lowering, sealing, artifacts, or runtime state.
type SemanticTrace struct {
	Production  string          `json:"production"`
	Source      string          `json:"source"`
	Occurrences []ASTOccurrence `json:"occurrences"`
}

// Ingress is one accepted corpus source that was parsed, bound, lowered, and
// sealed through the public Program ingress. ProgramID is the resulting
// immutable Program identity. It is cold generated evidence only: neither
// parser nor Program reads it while constructing semantics.
//
// Source identifies a member of the complete grammar witness corpus. The
// generated ledger must contain every corpus source, so a grammar production
// cannot cite a parser-only witness that bypasses the public ingress.
type Ingress struct {
	Source    string
	ProgramID string
}

// CorpusSource is one immutable grammar-witness input exposed to other cold
// proof components. It is not a parser input API: production callers keep
// using compiler/parse and program/lower directly.
type CorpusSource struct {
	ID   string
	Text string
}

// Snapshot is one freshly-derived cold proof input. Evidence, parser traces,
// and corpus entries come from the same parser grammar and exact source
// corpus, so a downstream denominator cannot join a stale trace to current
// ingress evidence.
type Snapshot struct {
	Evidence Evidence
	Traces   []SemanticTrace
	Corpus   []CorpusSource
}

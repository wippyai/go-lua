package grammarproof

import "github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"

type liveProduction struct {
	key       string
	reduction int
}

// SemanticTrace is one temporary-parser observation, resolved to the exact
// parser grammar alternative after tracing. It never participates in normal
// parsing, binding, lowering, sealing, artifacts, or runtime state.
type SemanticTrace struct {
	Production  string                `json:"production"`
	Source      string                `json:"source"`
	Occurrences []astcodec.Occurrence `json:"occurrences"`
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

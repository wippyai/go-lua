package static

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// poolRange selects an owner-local interval in an immutable pool.
type poolRange struct{ Start, End uint32 }

// len is the interval's width. An End that precedes its Start is an empty
// interval rather than a wrapped one.
func (span poolRange) len() int {
	if span.End < span.Start {
		return 0
	}
	return int(span.End - span.Start)
}

// poolSlice returns the interval's window of pool. An interval that does not
// lie inside pool yields the empty window, so a malformed range fails closed
// at the query boundary instead of panicking at the index.
func poolSlice[Element any](pool []Element, span poolRange) []Element {
	if span.End < span.Start || uint64(span.End) > uint64(len(pool)) {
		return nil
	}
	return pool[span.Start:span.End]
}

// poolAt returns one element of the interval's window of pool. It is the only
// indexed read of a pool: callers never reconstruct Start plus an offset.
func poolAt[Element any](pool []Element, span poolRange, index int) (value Element, ok bool) {
	window := poolSlice(pool, span)
	if index < 0 || index >= len(window) {
		return value, false
	}
	return window[index], true
}

// Input is the standalone authored Static boundary. Later verticals extend
// this package's private component rather than create a parallel static IR.
type Input struct {
	Counts       [keyspace.FamilyCount]uint32
	Types        TypesInput
	References   ReferencesInput
	Declarations DeclarationsInput
	Signatures   SignaturesInput
	Contracts    ContractsInput
	Operators    OperatorsInput
	Operands     OperandsInput
	Publications PublicationsInput
}

// Component is immutable authored Static syntax with no inferred/domain
// resolution, query index, or construction state.
// census is the sealed per-family cardinality column assigned once by Build;
// it is the sole cardinality authority and retains no duplicate Term stream
// or second graph.
type Component struct {
	contentID    identity.ContentID
	census       [keyspace.FamilyCount]uint32
	types        typeStore
	references   referenceStore
	declarations declarationStore
	signatures   signatureStore
	contracts    contractsStore
	operators    operatorsStore
	operands     operandsStore
	publications []publicationRow
}

type draftState struct {
	component        *Component
	localContainment *localContainmentProof
	phase            draftPhase
	mu               sync.Mutex
}

type draftPhase uint8

const (
	draftOpen draftPhase = iota + 1
	draftClaimed
	draftCommitted
	draftAborted
)

// Draft is a shared construction capability. Copies share state; publication
// is possible only by first claiming the owner-defined Finalizer.
type Draft struct{ state *draftState }

// Finalizer is an owner-defined one-shot publication capability. Copies share
// the Draft state, so exactly one terminal action (Commit or Abort) can win.
// The View is a read-only validation surface for the owner that coordinates
// finalization; it carries no construction state or sibling projection.
type Finalizer struct {
	state *draftState
}

// View partitions this vertical by exact typed relation.
type View struct {
	component *Component
	state     *draftState
}

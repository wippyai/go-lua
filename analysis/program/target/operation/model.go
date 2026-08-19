package operation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Input is neutral operation geometry accepted after Target has validated its
// root drafts. Source is the zero-based authoring coordinate; Core computes
// canonical root/produced order, issues operation handles, and retains the
// source-to-handle column.
//
// The nested slices are construction input only. CompileGeometry copies them
// into the sealed rows/pools below, so callers retain no write path into a
// Geometry or Core value.
type Input struct {
	Operations []OperationInput
}

// OperationInput is one root-validated operation descriptor. Produced edges
// name TargetSource rather than an operation handle because canonical order
// and handles are owned and issued here.
type OperationInput struct {
	Source            int
	Bindings          []vocabulary.BindingSpec
	InputFormalCount  int
	TypeFormalCount   int
	RowFormalCount    int
	ValuesVars        uint32
	OutcomeValueSlots []OutcomeInput
	Callbacks         []CallbackInput
	Produced          []ProducedInput
}

// OutcomeInput carries the fixed result-slot count and the neutral local
// discriminator used when a produced child receives its operation anchor.
// Anchor is copied as bytes and is not interpreted by this package.
type OutcomeInput struct {
	ValueSlots uint32
	Anchor     []byte
}

// CallbackInput is the callback coordinate and lifecycle needed by protocol
// and by Target's callback relation. Source is the zero-based callback
// authoring coordinate; the canonical callback ID is issued by Core.
type CallbackInput struct {
	Source    int
	Lifecycle vocabulary.CallbackLifecycle
}

// ProducedInput is one parent/outcome/result to child relation. TargetSource
// is a zero-based source operation coordinate, not a pre-issued Operation.
type ProducedInput struct {
	TargetSource int
	Outcome      uint32
	Result       uint32
}

type operationRow struct {
	bindings  rows.Span
	outcomes  rows.Span
	callbacks callbackRange
	produced  rows.Span
	input     uint32
	typeForms uint32
	rowForms  uint32
	valuesVar uint32
}

type callbackRange struct{ start, end uint32 }

func (span callbackRange) Len() int {
	if span.end < span.start {
		return 0
	}
	return int(span.end - span.start)
}

type bindingRow struct {
	namespace vocabulary.BindingNamespace
	owner     rows.Span
	member    rows.Span
}

type outcomeRow struct {
	slots  uint32
	anchor rows.Span
}

type callbackRow struct {
	id        vocabulary.CallbackID
	owner     vocabulary.Operation
	source    int
	ordinal   uint32
	lifecycle vocabulary.CallbackLifecycle
}

type producedRow struct {
	parent  vocabulary.Operation
	child   vocabulary.Operation
	outcome uint32
	result  uint32
}

type sourceRow struct{ operation vocabulary.Operation }

type bindingIndexRow struct {
	binding   uint32
	operation vocabulary.Operation
}

type bindingKeyRow struct {
	owner  rows.Span
	member rows.Span
}

type bindingKeyRange struct{ bindings rows.Span }

// Geometry is the first immutable operation value. It owns canonical
// operation/callback coordinates and all anchor-neutral geometry, but not the
// exact-key-dependent binding/produced semantic anchors.
type Geometry struct {
	operations rows.Rows[operationRow]
	bindings   rows.Pool[bindingRow]
	segments   rows.Pool[string]
	outcomes   rows.Pool[outcomeRow]
	anchors    rows.Pool[byte]
	callbacks  rows.Rows[callbackRow]
	produced   rows.Pool[producedRow]
	sources    rows.Rows[sourceRow]
	sourceN    int
	boundN     int
}

// anchorRow is kept separate from Geometry so exact-key finalization returns
// a distinct immutable value rather than mutating Geometry in place.
type anchorRow struct{ id identity.ContentID }

// Core is the complete immutable operation owner. Every operation handle,
// callback ID/lifecycle, and operation semantic anchor is read from this
// value. No caller can append, backpatch, or toggle a publication flag.
type Core struct {
	geometry                Geometry
	anchors                 rows.Rows[anchorRow]
	keys                    exactkey.Table
	bindingKeys             rows.Pool[vocabulary.ExactKey]
	bindingKeyRows          rows.Pool[bindingKeyRow]
	bindingRanges           rows.Rows[bindingKeyRange]
	lookup                  rows.Rows[bindingIndexRow]
	query                   queryState
	effectOperationIDs      []identity.ContentID
	effectDescriptorIDs     []identity.ContentID
	effectOccurrenceIDs     []identity.ContentID
	operationEffectFamilies []identity.ContentID
	callbackEffectFamilies  []identity.ContentID
}

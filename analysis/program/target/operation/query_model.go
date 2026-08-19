package operation

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// QueryInput is the immutable value handoff for Target's operation query
// plane. It contains only sealed handles and values; it has no Target
// callback, builder, or draft reference. CompileQuery copies it into Core and
// the caller may then discard its construction columns.
//
// The operation owner already issued Operation, Values, and Type handles
// before this handoff. QueryInput preserves those handles while taking
// ownership of the rows and their pools.
type QueryInput struct {
	Operations []QueryOperationInput
	Types      []TypeInput
	Values     []ValuesInput
	Effects    []EffectInput
}

// QueryOperationInput is one canonical operation's query projection. The
// nested slices are construction-only and are copied by CompileQuery.
type QueryOperationInput struct {
	Input              vocabulary.Values
	Outcomes           []QueryOutcomeInput
	Behavior           []BehaviorResultInput
	BehaviorPredicates []BehaviorPredicateInput
	ValuesTypes        []vocabulary.Type
	Transfers          []TransferInput
	EffectIndices      []int
	TypeFormals        []vocabulary.Type
	RowFormals         uint32
	EffectTail         vocabulary.RowTail
	EffectVar          vocabulary.RowVar
}

type TypeInput struct {
	Handle      vocabulary.Type
	Declaration schematype.Type
}

type ValuesInput struct {
	Handle vocabulary.Values
	Owner  vocabulary.Operation
	Types  []vocabulary.Type
	Tail   vocabulary.ValuesTail
	VarID  vocabulary.ValuesVar
	Suffix []vocabulary.Type
}

type QueryOutcomeInput struct {
	Kind   flowkind.OutcomeKind
	Values vocabulary.Values
}

type BehaviorResultInput struct {
	Outcome  uint32
	Result   uint32
	Source   vocabulary.InputSource
	Relation schema.EntryID
}

type BehaviorPredicateInput struct {
	Outcome  uint32
	Result   uint32
	Subject  vocabulary.InputSource
	Relation schema.EntryID
}

type TransferInput struct {
	Endpoint     vocabulary.TransferEndpoint
	Payload      vocabulary.InputSource
	Alias        vocabulary.InputSource
	Identity     vocabulary.TransferIdentity
	Capabilities vocabulary.TransferCapabilities
	Outcomes     []vocabulary.TransferPossibility
}

type EffectInput struct {
	Target         vocabulary.Operation
	Values         []vocabulary.ValueFormal
	Types          []vocabulary.TypeFormal
	ValuesVar      []vocabulary.ValuesVar
	Rows           []vocabulary.RowVar
	Publication    vocabulary.PublicationEffectSpec
	HasPublication bool
}

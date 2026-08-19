package operation

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
)

// QueryOperationInput is one canonical operation's query projection. The
// nested slices are construction-only and are consumed by QueryBuilder.
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

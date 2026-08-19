package operation

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// QueryOperationInput is one canonical operation's query projection. The
// nested slices are construction-only and are consumed by QueryBuilder.
type QueryOperationInput struct {
	Input              vocabulary.Values
	Outcomes           []QueryOutcomeInput
	Produced           []ProducedQueryInput
	FreshResults       []FreshResultInput
	CallbackResults    []CallbackResultInput
	ResultAliases      []ResultAliasInput
	Suspensions        []SuspensionInput
	Spawns             []SpawnInput
	Resumes            []ResumeInput
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

// ProducedQueryInput is the neutral, already-resolved relation handed to the
// operation owner during the one-shot query seal. Target may resolve its
// authoring SpecRef before this boundary, but no Target draft or Contract is
// retained by Core.
type ProducedQueryInput struct {
	Outcome  uint32
	Result   uint32
	Target   vocabulary.Operation
	Captures []CaptureInput
}

// CaptureInput is a neutral producer-side source. Callback ordinals are
// owner-issued CallbackIDs by the time this input crosses the query boundary.
type CaptureInput struct {
	Kind    vocabulary.CaptureKind
	Ordinal uint32
}

type FreshResultInput struct {
	Outcome uint32
	Result  uint32
	Ordinal uint32
	Kind    schematype.FreshClass
}

type CallbackResultInput struct {
	Outcome  uint32
	Result   uint32
	Callback vocabulary.CallbackID
}

type ResultAliasInput struct {
	Outcome uint32
	Result  uint32
	Source  vocabulary.InputSource
}

type SuspensionInput struct {
	Yield        uint32
	Reentry      uint32
	Source       vocabulary.ReentrySource
	Multiplicity vocabulary.ReentryMultiplicity
}

type SpawnInput struct {
	Function     vocabulary.InputSource
	Child        vocabulary.CallbackID
	Yield        uint32
	ParentResume uint32
	ChildEntry   vocabulary.Values
	ResumeValues vocabulary.Values
	Alternatives [2]vocabulary.SpawnSiblingAlternative
}

type ResumeInput struct {
	Source    vocabulary.ResumeSource
	Carrier   vocabulary.ValueFormal
	Arguments vocabulary.Values
	Outcomes  [5]uint32
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

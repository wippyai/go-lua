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
	CallbackValues     []CallbackQueryInput
	Subedges           []SubedgeInput
	SubedgeRelation    *SubedgeRelationInput
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
	// Semantics is the domain adapter for the Values transport laws. It is
	// construction input only; Core retains no adapter after FinishQuery.
	Semantics schematype.Semantics
}

// SubedgeInput is the neutral, already-resolved sealed declaration for one
// typed internal application. Target resolves authoring references to owner
// coordinates before this boundary; Core issues the global SubedgeID and
// retains the immutable rows.
type SubedgeInput struct {
	// Source is the zero-based authoring coordinate retained only long enough
	// for Core to preserve source-referenced sibling identity while issuing
	// canonical dense SubedgeIDs. It is never published in a query row.
	Source uint32
	Role   uint32
	Family vocabulary.SubedgeFamily
	Callee vocabulary.SubedgeCalleeKind
	// CallbackSource is the zero-based authored callback coordinate. Core
	// resolves it through the owner geometry and publishes the CallbackID.
	CallbackSource uint32
	ReadRoot       vocabulary.InitialRoot
	ReadKey        vocabulary.ExactKey
	MetaKey        vocabulary.ExactKey
	Admission      schematype.CallableAdmission
	// Arguments and Terminals are the authored endpoints. Callback-backed
	// edges must leave both empty; Core obtains their effective endpoints from
	// CallbackValues and validates the union here.
	Arguments        vocabulary.Values
	RuleEntry        bool
	ArgumentOrigins  []SubedgeArgumentOriginInput
	Terminals        []SubedgeTerminalInput
	AdmissionFailure vocabulary.Values
	AdmissionRoute   SubedgeRouteInput
	Routes           [5]SubedgeRouteInput
}

type SubedgeTerminalInput struct {
	Kind   flowkind.OutcomeKind
	Values vocabulary.Values
}

type CallbackQueryInput struct {
	Source    uint32
	Admission schematype.CallableAdmission
	Arguments vocabulary.Values
	Outcomes  [5]vocabulary.Values
}

type SubedgeArgumentOriginInput struct {
	Segment vocabulary.ArgumentSegment
	Index   uint32
	Kind    vocabulary.ArgumentSource
	Source  vocabulary.InputSource
}

// SubedgeRouteInput uses a zero-based source sibling rank. Core converts that
// rank through the source-to-canonical map to a global SubedgeID only after
// the owner range is sealed.
type SubedgeRouteInput struct {
	Route       vocabulary.SubedgeRoute
	Adjustment  vocabulary.Adjustment
	Result      vocabulary.Values
	Placement   vocabulary.Placement
	Offset      uint32
	Outcome     uint32
	HasSibling  bool
	SiblingRank uint32
}

type SubedgeRelationInput struct {
	Operand  vocabulary.ValueFormal
	Selector uint32
	// SubedgeRank is the zero-based authored source coordinate. Core resolves
	// it through the canonical role order before publishing the relation.
	SubedgeRank   uint32
	ResultOutcome uint32
	Result        uint32
	EffectAliases []uint32
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
	// Source is the zero-based authored outcome coordinate. It is consumed by
	// Core while resolving subedge/relation destinations and never published.
	Source    uint32
	HasSource bool
	Kind      flowkind.OutcomeKind
	Values    vocabulary.Values
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

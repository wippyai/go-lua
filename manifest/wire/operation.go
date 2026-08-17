package wire

import "github.com/wippyai/go-lua/domain/type/typ"

type CapturedInitialReadSpec = CapturedInitialRead

// Operation is the portable, provider-owned behavioral declaration for one
// callable. It complements FunctionSignatures: signatures describe the public
// type/effect contract, while Operation describes correlated runtime outcomes
// and callback/control relations. Consumers may index or seal this data, but
// must never re-author provider behavior by callable name.
type Operation struct {
	// Replace selects a complete provider-authored control envelope. Without it,
	// the law amends the exact signature-derived envelope below.
	Replace bool
	// SelfEffect requests the operation-occurrence row even when the callable
	// has no direct mounted binding (for produced iterators).
	SelfEffect        bool
	ValuesVars        uint32
	Input             Values
	Outcomes          []Outcome
	Callbacks         []Callback
	Subedges          []Subedge
	Suspensions       []Suspension
	Spawns            []Spawn
	Resumes           []Resume
	SubedgeRelation   *SubedgeRelation
	Effects           RowSpec
	AppendNormal      []Values
	ReplaceNormal     []Values
	ReplaceNormalSet  bool
	InputTailType     typ.Type `json:"-"`
	OutcomeTailTypes  []OutcomeTailType
	OutcomeAmendments []OutcomeAmendment
}

type OutcomeTailType struct {
	Outcome uint32
	Type    typ.Type `json:"-"`
}

type OutcomeAmendment struct {
	Outcome         uint32
	Produced        []Produced
	FreshResults    []FreshResult
	CallbackResults []CallbackResult
	ResultAliases   []ResultAlias
}

type OutcomeKind uint8

const (
	OutcomeNormal OutcomeKind = iota + 1
	OutcomeReturn
	OutcomeThrow
	OutcomeBreak
	OutcomeGoto
	OutcomeYield
	OutcomeCancel
)

type ValuesTail uint8

const (
	ValuesClosed ValuesTail = iota + 1
	ValuesVariable
	ValuesUnknown
)

type (
	ValueFormal uint32
	ValuesVar   uint32
	CallbackRef uint32
	SubedgeRef  uint32
)

type Values struct {
	Fixed    []typ.Type `json:"-"`
	Tail     ValuesTail
	Var      ValuesVar
	TailType typ.Type   `json:"-"`
	Suffix   []typ.Type `json:"-"`
}

type InputSourceKind uint8

const (
	InputSourceInvalid InputSourceKind = iota
	InputSourceValue
	InputSourceValues
)

type InputSource struct {
	Kind    InputSourceKind
	Ordinal uint32
}

type Terminal struct {
	Kind   OutcomeKind
	Values Values
}

type CallableAdmission uint8

const (
	CallableAdmissionInvalid CallableAdmission = iota
	CallableAdmissionDirectFunction
	CallableAdmissionOrdinary
)

type CallbackLifecycle uint8

const (
	CallbackLifecycleInvalid CallbackLifecycle = iota
	CallbackSyncOptionalOnce
	CallbackSyncRequiredOnce
	CallbackSyncOptionalMany
	CallbackSyncRequiredMany
	CallbackRetainedOptionalOnce
	CallbackRetainedRequiredOnce
	CallbackRetainedOptionalMany
	CallbackRetainedRequiredMany
)

type Callback struct {
	Function  InputSource
	Admission CallableAdmission
	Arguments Values
	Outcomes  []Terminal
	Lifecycle CallbackLifecycle
	Effects   RowSpec
}

type CallbackResult struct {
	Result   uint32
	Callback CallbackRef // one-based operation-local callback coordinate
}

type ResultAlias struct {
	Result uint32
	Source InputSource
}

type FreshClass uint8

const (
	FreshInvalid FreshClass = iota
	FreshTable
	FreshFunction
	FreshThread
	FreshUserdata
	FreshError
	FreshReflection
)

type FreshResult struct {
	Result uint32
	Class  FreshClass
}

type CaptureKind uint8

const (
	CaptureInvalid CaptureKind = iota
	CaptureValue
	CaptureTypeValue
	CaptureValues
	CaptureCallback
)

type Capture struct {
	Kind    CaptureKind
	Ordinal uint32
}

type Produced struct {
	Result    uint32
	Operation string
	Captures  []Capture
}

type Outcome struct {
	Kind            OutcomeKind
	Values          Values
	Produced        []Produced
	FreshResults    []FreshResult
	CallbackResults []CallbackResult
	ResultAliases   []ResultAlias
}

type ReentrySource uint8

const (
	ReentryInvalid ReentrySource = iota
	ReentryByCall
	ReentryByProvider
)

type ReentryMultiplicity uint8

const (
	ReentryMultiplicityInvalid ReentryMultiplicity = iota
	ReentryOnce
	ReentryMany
)

type Suspension struct {
	Yield        uint32
	Reentry      uint32
	Source       ReentrySource
	Multiplicity ReentryMultiplicity
}

type ResumeSource uint8

const (
	ResumeSourceInvalid ResumeSource = iota
	ResumeSourceValue
	ResumeSourceProduced
)

type ResumeOutcome struct {
	Kind    OutcomeKind
	Outcome uint32
}

type Resume struct {
	Source    ResumeSource
	Carrier   ValueFormal
	Arguments Values
	Outcomes  []ResumeOutcome
}

type SpawnSiblingAlternative uint8

const (
	SpawnSiblingInvalid SpawnSiblingAlternative = iota
	SpawnChildEntryThenParentResume
	SpawnParentResumeThenChildEntry
)

type Spawn struct {
	Function     InputSource
	Child        CallbackRef // one-based callback coordinate
	Yield        uint32
	ParentResume uint32
	ChildEntry   uint32
	Alternatives []SpawnSiblingAlternative
}

type SubedgeFamily uint8

const (
	SubedgeFamilyInvalid SubedgeFamily = iota
	SubedgeFamilyCall
	SubedgeFamilyLength
	SubedgeFamilyIndexGet
	SubedgeFamilyIndexSet
	SubedgeFamilyEqual
	SubedgeFamilyLess
)

type SubedgeCalleeKind uint8

const (
	SubedgeCalleeInvalid SubedgeCalleeKind = iota
	SubedgeCalleeCallback
	SubedgeCalleeCapturedInitialRead
	SubedgeCalleeMetaKey
)

type LiteralKind uint8

const (
	LiteralInvalid LiteralKind = iota
	LiteralString
)

type Literal struct {
	Kind   LiteralKind
	String string
}

type CapturedInitialRead struct {
	Root string
	Key  Literal
}

type SubedgeCallee struct {
	Kind     SubedgeCalleeKind
	Callback CallbackRef
	Read     CapturedInitialRead
	MetaKey  Literal
}

type SubedgeRouteKind uint8

const (
	SubedgeRouteInvalid SubedgeRouteKind = iota
	SubedgeRouteOutcome
	SubedgeRouteSubedge
	SubedgeRouteContinue
	SubedgeRoutePropagateYield
	SubedgeRouteRejectYield
)

type Adjustment uint8

const (
	AdjustmentInvalid Adjustment = iota
	AdjustmentPreserve
	AdjustmentExact
)

type Placement uint8

const (
	PlacementInvalid Placement = iota
	PlacementTail
	PlacementFixed
)

type SubedgeRoute struct {
	Kind       OutcomeKind
	Route      SubedgeRouteKind
	Adjustment Adjustment
	Result     Values
	Placement  Placement
	Offset     uint32
	Outcome    uint32
	Subedge    SubedgeRef
}

type ArgumentSegment uint8

const (
	ArgumentSegmentInvalid ArgumentSegment = iota
	ArgumentFixed
	ArgumentSuffix
	ArgumentTail
)

type ArgumentSource uint8

const (
	ArgumentSourceInvalid ArgumentSource = iota
	ArgumentSourceInput
	ArgumentSourceRule
)

type ArgumentOrigin struct {
	Segment ArgumentSegment
	Index   uint32
	Kind    ArgumentSource
	Source  InputSource
}

type AdmissionRoute struct {
	Route      SubedgeRouteKind
	Adjustment Adjustment
	Result     Values
	Placement  Placement
	Offset     uint32
	Outcome    uint32
	Subedge    SubedgeRef
}

type AdmissionFailure struct {
	Values Values
	Route  AdmissionRoute
}

type Subedge struct {
	Role             uint32
	Family           SubedgeFamily
	Callee           SubedgeCallee
	Admission        CallableAdmission
	Arguments        Values
	RuleEntry        bool
	ArgumentOrigins  []ArgumentOrigin
	Outcomes         []Terminal
	AdmissionFailure AdmissionFailure
	Routes           []SubedgeRoute
}

// SubedgeRelation records one provider-owned correlation between an input
// operand, an existing subedge, and an existing result. Selector is an opaque
// provider token: consumers preserve it without inventing provider semantics.
type SubedgeRelation struct {
	Operand       ValueFormal
	Selector      uint32
	Subedge       SubedgeRef
	ResultOutcome uint32
	Result        uint32
	EffectAliases []uint32
}

type RowTail uint8

const (
	RowClosed RowTail = iota + 1
	RowVariable
	RowUnknownOpen
)

// RowSpec is the portable operation-occurrence row. Native function effect
// labels remain in the exact signature Effect row; this row represents
// invocation correspondence and is therefore a distinct relation.
type RowSpec struct {
	Tail RowTail
}

// CloneOperation makes provider data ownership explicit. Type values are
// immutable declarations; every collection and nested relation is copied.
func CloneOperation(in Operation) Operation {
	out := in
	out.AppendNormal = cloneValuesList(in.AppendNormal)
	out.ReplaceNormal = cloneValuesList(in.ReplaceNormal)
	out.OutcomeTailTypes = append([]OutcomeTailType(nil), in.OutcomeTailTypes...)
	out.OutcomeAmendments = append([]OutcomeAmendment(nil), in.OutcomeAmendments...)
	for i := range out.OutcomeAmendments {
		out.OutcomeAmendments[i].Produced = append([]Produced(nil), in.OutcomeAmendments[i].Produced...)
		for j := range out.OutcomeAmendments[i].Produced {
			out.OutcomeAmendments[i].Produced[j].Captures = append([]Capture(nil), in.OutcomeAmendments[i].Produced[j].Captures...)
		}
		out.OutcomeAmendments[i].FreshResults = append([]FreshResult(nil), in.OutcomeAmendments[i].FreshResults...)
		out.OutcomeAmendments[i].CallbackResults = append([]CallbackResult(nil), in.OutcomeAmendments[i].CallbackResults...)
		out.OutcomeAmendments[i].ResultAliases = append([]ResultAlias(nil), in.OutcomeAmendments[i].ResultAliases...)
	}
	out.Input = cloneValues(in.Input)
	out.Outcomes = append([]Outcome(nil), in.Outcomes...)
	for i := range out.Outcomes {
		out.Outcomes[i].Values = cloneValues(in.Outcomes[i].Values)
		out.Outcomes[i].Produced = append([]Produced(nil), in.Outcomes[i].Produced...)
		for j := range out.Outcomes[i].Produced {
			out.Outcomes[i].Produced[j].Captures = append([]Capture(nil), in.Outcomes[i].Produced[j].Captures...)
		}
		out.Outcomes[i].FreshResults = append([]FreshResult(nil), in.Outcomes[i].FreshResults...)
		out.Outcomes[i].CallbackResults = append([]CallbackResult(nil), in.Outcomes[i].CallbackResults...)
		out.Outcomes[i].ResultAliases = append([]ResultAlias(nil), in.Outcomes[i].ResultAliases...)
	}
	out.Callbacks = append([]Callback(nil), in.Callbacks...)
	for i := range out.Callbacks {
		out.Callbacks[i].Arguments = cloneValues(in.Callbacks[i].Arguments)
		out.Callbacks[i].Outcomes = cloneTerminals(in.Callbacks[i].Outcomes)
	}
	out.Subedges = append([]Subedge(nil), in.Subedges...)
	for i := range out.Subedges {
		out.Subedges[i].Arguments = cloneValues(in.Subedges[i].Arguments)
		out.Subedges[i].ArgumentOrigins = append([]ArgumentOrigin(nil), in.Subedges[i].ArgumentOrigins...)
		out.Subedges[i].Outcomes = cloneTerminals(in.Subedges[i].Outcomes)
		out.Subedges[i].AdmissionFailure.Values = cloneValues(in.Subedges[i].AdmissionFailure.Values)
		out.Subedges[i].AdmissionFailure.Route.Result = cloneValues(in.Subedges[i].AdmissionFailure.Route.Result)
		out.Subedges[i].Routes = append([]SubedgeRoute(nil), in.Subedges[i].Routes...)
		for j := range out.Subedges[i].Routes {
			out.Subedges[i].Routes[j].Result = cloneValues(in.Subedges[i].Routes[j].Result)
		}
	}
	out.Suspensions = append([]Suspension(nil), in.Suspensions...)
	out.Spawns = append([]Spawn(nil), in.Spawns...)
	for i := range out.Spawns {
		out.Spawns[i].Alternatives = append([]SpawnSiblingAlternative(nil), in.Spawns[i].Alternatives...)
	}
	out.Resumes = append([]Resume(nil), in.Resumes...)
	for i := range out.Resumes {
		out.Resumes[i].Arguments = cloneValues(in.Resumes[i].Arguments)
		out.Resumes[i].Outcomes = append([]ResumeOutcome(nil), in.Resumes[i].Outcomes...)
	}
	if in.SubedgeRelation != nil {
		value := *in.SubedgeRelation
		value.EffectAliases = append([]uint32(nil), value.EffectAliases...)
		out.SubedgeRelation = &value
	}
	return out
}

func cloneValuesList(in []Values) []Values {
	out := append([]Values(nil), in...)
	for i := range out {
		out[i] = cloneValues(in[i])
	}
	return out
}

func cloneValues(in Values) Values {
	out := in
	out.Fixed = append([]typ.Type(nil), in.Fixed...)
	out.Suffix = append([]typ.Type(nil), in.Suffix...)
	return out
}

func cloneTerminals(in []Terminal) []Terminal {
	out := append([]Terminal(nil), in...)
	for i := range out {
		out[i].Values = cloneValues(in[i].Values)
	}
	return out
}

package wire

import (
	"sort"

	"github.com/wippyai/go-lua/domain/type/typ"
)

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
	Transfers         []TransferSpec
	// Acquisitions declare the typestate resources this callable creates. A
	// lifecycle effect label names its subject with a parameter reference, so
	// the acquiring member of a resource protocol - one that produces the
	// resource it creates - cannot state its subject there. Acquisition is
	// therefore an operation law, addressed the same way every other
	// result-slot declaration on this boundary is.
	Acquisitions []Acquisition
	// Requirements declare the typestate states this callable reads without
	// moving. Acquisition states where a resource comes from and a lifecycle
	// transition states where it goes; a member that only reads one - a query
	// on an open connection, a send on an open channel - constrains the state
	// without changing it, and has no other row on this boundary to say so.
	// Without this row such a member is indistinguishable from one that does
	// not care which state it runs in, and a consumer cannot refuse the call.
	Requirements []Requirement
	// Behavior carries provider-owned result and predicate correspondences.
	// The relation is deliberately a plain wire key: this package is a
	// portable module boundary and must not import the analyzer's schema
	// identity package. The Lua-domain target adapter resolves the key against
	// its structural vocabulary when it projects the operation.
	Behavior *OperationBehavior `json:"Behavior,omitempty"`
}

// Acquisition declares that one fixed result slot of one authored outcome
// creates a resource governed by a declared typestate protocol, in the named
// initial state. Outcome is the zero-based authored outcome ordinal and Result
// the fixed result ordinal within it, exactly as OperationResult addresses a
// result slot. Protocol must name a manifest-declared typestate FSM and State
// one of its declared states; the FSM alone decides which state may be
// acquired.
type Acquisition struct {
	Outcome  uint32
	Result   uint32
	Protocol string
	State    string
}

// Requirement declares that one input of this callable must be in the named
// state of a declared typestate protocol when the call runs, and that the call
// leaves it in that state. Input addresses the constrained argument exactly as
// every other input-source declaration on this boundary does. Protocol must
// name a manifest-declared typestate FSM and State one of its declared states;
// the FSM alone decides which state may be required.
//
// A requirement is deliberately not a transition with equal endpoints. A
// transition declares a move and is completed on an operation's normal arms
// only; a requirement declares no move at all, so it constrains every arm and
// discharges no obligation.
type Requirement struct {
	Input    InputSource
	Protocol string
	State    string
}

// OperationBehavior is the generic, provider-owned behavior descriptor of an
// operation. It contains no runtime-kind enum or evaluator; a relation is
// only a neutral key that the consuming domain resolves at its own boundary.
// A nil or empty descriptor carries no behavior rows.
type OperationBehavior struct {
	Results    []OperationResult
	Predicates []OperationPredicate
}

// OperationResult declares that one fixed result slot of an authored outcome
// classifies an existing operation input. Outcome is the zero-based authored
// outcome ordinal, Result is the fixed result ordinal, and Source identifies
// the input being classified. Relation is a provider-owned schema key carried
// opaquely through the portable manifest.
type OperationResult struct {
	Outcome  uint32
	Result   uint32
	Source   InputSource
	Relation string
}

// OperationPredicate declares the corresponding predicate relation for one
// fixed result slot. Predicate polarity is intentionally outside this
// declaration; consumers decide whether the row is used positively or
// negatively while preserving the same provider-owned correspondence.
type OperationPredicate struct {
	Outcome  uint32
	Result   uint32
	Subject  InputSource
	Relation string
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
	TypeFormal  uint32
	ValuesVar   uint32
	RowVar      uint32
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

// EffectSpec is one explicit operation-local invocation row. Target is the
// canonical manifest operation path of the invoked operation; it is resolved
// by manifesttarget against the same catalogue that owns all operations. A
// publication is never inferred from a signature label or from the target
// operation's name: it is present only when the provider supplies the typed
// consequence below.
type EffectSpec struct {
	Target      string
	ValueArgs   []ValueFormal
	TypeArgs    []TypeFormal
	ValuesArgs  []ValuesVar
	RowArgs     []RowVar
	Publication *PublicationEffectSpec
}

// PublicationEffectKind is the portable manifest spelling of Target's
// explicit publication operation. The adapter converts it to the canonical
// Target vocabulary; the wire package carries no placement or runtime proof.
type PublicationEffectKind uint8

const (
	PublicationEffectInvalid PublicationEffectKind = iota
	PublicationEffectSendTransfer
	PublicationEffectReturnEscape
	PublicationEffectCallbackEscape
	PublicationEffectFreezeSeal
	PublicationEffectWriteMutation
	PublicationEffectCloseRelease
)

type PublicationDestinationRole uint8

const (
	PublicationDestinationInvalid PublicationDestinationRole = iota
	PublicationDestinationNone
	PublicationDestinationValueFormal
)

type PublicationEscapeDisposition uint8

const (
	PublicationEscapeInvalid PublicationEscapeDisposition = iota
	PublicationEscapeNone
	PublicationEscapeSendTransfer
	PublicationEscapeReturn
	PublicationEscapeCallback
)

type PublicationMutabilityDisposition uint8

const (
	PublicationMutabilityInvalid PublicationMutabilityDisposition = iota
	PublicationMutabilityPreserve
	PublicationMutabilitySeal
	PublicationMutabilityWrite
	PublicationMutabilityCopyOnWrite
)

type PublicationLifetimeDisposition uint8

const (
	PublicationLifetimeInvalid PublicationLifetimeDisposition = iota
	PublicationLifetimePreserve
	PublicationLifetimeRelease
)

// PublicationEffectSpec carries all seven fields needed to construct the
// Target-owned PublicationEffectSpec. Subject selects either one target
// ValueFormal or the target input ValuesVar; Context remains a target
// ValueFormal when Destination is meaningful. These declarations are static
// semantic inputs only, never runtime delivery or placement conclusions.
type PublicationEffectSpec struct {
	Kind        PublicationEffectKind
	Subject     InputSource
	Destination PublicationDestinationRole
	Context     ValueFormal
	Escape      PublicationEscapeDisposition
	Mutability  PublicationMutabilityDisposition
	Lifetime    PublicationLifetimeDisposition
}

// TransferPossibility is the provider-owned outcome relation of one
// observable transfer. It intentionally mirrors the neutral Target
// vocabulary without importing analyzer packages into this portable wire
// package.
type TransferPossibility uint8

const (
	TransferMayDeliver TransferPossibility = 1 << iota
	TransferMayReject
)

type TransferOutcomeSpec struct {
	Outcome     uint32
	Possibility TransferPossibility
}

type TransferEndpointKind uint8

const (
	TransferEndpointInvalid TransferEndpointKind = iota
	TransferEndpointInput
	TransferEndpointExternal
)

type TransferEndpoint struct {
	Kind  TransferEndpointKind
	Input uint32
}

type TransferIdentity uint8

const (
	TransferIdentityInvalid TransferIdentity = iota
	TransferIdentityUnspecified
	TransferIdentitySame
	TransferIdentityDistinct
)

type TransferCapabilities uint8

const (
	TransferCapabilitiesInvalid TransferCapabilities = iota
	TransferCapabilitiesUnspecified
	TransferCapabilitiesPreserveAll
	TransferCapabilitiesLoseAll
)

// TransferSpec declares one provider-authored transfer relation. Endpoint,
// payload, alias, and outcome coordinates are all neutral operation-local
// references; the Lua Target adapter resolves them without inferring a
// transfer from a callable name or an ownership label.
type TransferSpec struct {
	Endpoint     TransferEndpoint
	Payload      InputSource
	Alias        InputSource
	Identity     TransferIdentity
	Capabilities TransferCapabilities
	Outcomes     []TransferOutcomeSpec
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
	Occurrences []EffectSpec
	Tail        RowTail
	Var         RowVar
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
	out.Effects = cloneRow(in.Effects)
	out.Transfers = append([]TransferSpec(nil), in.Transfers...)
	for index := range out.Transfers {
		out.Transfers[index].Outcomes = append([]TransferOutcomeSpec(nil), in.Transfers[index].Outcomes...)
	}
	out.Acquisitions = append([]Acquisition(nil), in.Acquisitions...)
	sort.SliceStable(out.Acquisitions, func(left, right int) bool {
		return compareAcquisition(out.Acquisitions[left], out.Acquisitions[right]) < 0
	})
	out.Requirements = append([]Requirement(nil), in.Requirements...)
	sort.SliceStable(out.Requirements, func(left, right int) bool {
		return compareRequirement(out.Requirements[left], out.Requirements[right]) < 0
	})
	if in.Behavior != nil {
		behavior := &OperationBehavior{
			Results:    append([]OperationResult(nil), in.Behavior.Results...),
			Predicates: append([]OperationPredicate(nil), in.Behavior.Predicates...),
		}
		sort.SliceStable(behavior.Results, func(left, right int) bool {
			return compareOperationResult(behavior.Results[left], behavior.Results[right]) < 0
		})
		sort.SliceStable(behavior.Predicates, func(left, right int) bool {
			return compareOperationPredicate(behavior.Predicates[left], behavior.Predicates[right]) < 0
		})
		out.Behavior = behavior
	}
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
		out.Callbacks[i].Effects = cloneRow(in.Callbacks[i].Effects)
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

func cloneRow(in RowSpec) RowSpec {
	out := in
	if len(in.Occurrences) == 0 {
		out.Occurrences = nil
		return out
	}
	out.Occurrences = make([]EffectSpec, len(in.Occurrences))
	for index, effect := range in.Occurrences {
		out.Occurrences[index] = effect
		out.Occurrences[index].ValueArgs = append([]ValueFormal(nil), effect.ValueArgs...)
		out.Occurrences[index].TypeArgs = append([]TypeFormal(nil), effect.TypeArgs...)
		out.Occurrences[index].ValuesArgs = append([]ValuesVar(nil), effect.ValuesArgs...)
		out.Occurrences[index].RowArgs = append([]RowVar(nil), effect.RowArgs...)
		if effect.Publication != nil {
			publication := *effect.Publication
			out.Occurrences[index].Publication = &publication
		}
	}
	return out
}

func compareAcquisition(left, right Acquisition) int {
	if left.Outcome != right.Outcome {
		if left.Outcome < right.Outcome {
			return -1
		}
		return 1
	}
	if left.Result != right.Result {
		if left.Result < right.Result {
			return -1
		}
		return 1
	}
	if left.Protocol != right.Protocol {
		if left.Protocol < right.Protocol {
			return -1
		}
		return 1
	}
	if left.State != right.State {
		if left.State < right.State {
			return -1
		}
		return 1
	}
	return 0
}

func compareRequirement(left, right Requirement) int {
	if left.Input.Kind != right.Input.Kind {
		if left.Input.Kind < right.Input.Kind {
			return -1
		}
		return 1
	}
	if left.Input.Ordinal != right.Input.Ordinal {
		if left.Input.Ordinal < right.Input.Ordinal {
			return -1
		}
		return 1
	}
	if left.Protocol != right.Protocol {
		if left.Protocol < right.Protocol {
			return -1
		}
		return 1
	}
	if left.State != right.State {
		if left.State < right.State {
			return -1
		}
		return 1
	}
	return 0
}

func compareOperationResult(left, right OperationResult) int {
	if left.Outcome != right.Outcome {
		if left.Outcome < right.Outcome {
			return -1
		}
		return 1
	}
	if left.Result != right.Result {
		if left.Result < right.Result {
			return -1
		}
		return 1
	}
	if order := compareInputSource(left.Source, right.Source); order != 0 {
		return order
	}
	if left.Relation < right.Relation {
		return -1
	}
	if left.Relation > right.Relation {
		return 1
	}
	return 0
}

func compareOperationPredicate(left, right OperationPredicate) int {
	if left.Outcome != right.Outcome {
		if left.Outcome < right.Outcome {
			return -1
		}
		return 1
	}
	if left.Result != right.Result {
		if left.Result < right.Result {
			return -1
		}
		return 1
	}
	if order := compareInputSource(left.Subject, right.Subject); order != 0 {
		return order
	}
	if left.Relation < right.Relation {
		return -1
	}
	if left.Relation > right.Relation {
		return 1
	}
	return 0
}

func compareInputSource(left, right InputSource) int {
	if left.Kind != right.Kind {
		if left.Kind < right.Kind {
			return -1
		}
		return 1
	}
	if left.Ordinal < right.Ordinal {
		return -1
	}
	if left.Ordinal > right.Ordinal {
		return 1
	}
	return 0
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

// Package target owns the sealed, portable target-operation contract.
//
// It deliberately owns only static operation ABI, Values, authored Koka rows,
// outcomes, bindings, callback correspondence and expected effect rows,
// explicit retained-release relations, produced operations, operation-owned
// suspension, and observable endpoint/payload/alias transfer authority. Protocol,
// project linking, and analysis remain later layers keyed by Operation.
package target

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Operation is the sole sealed target-operation identity. It is also the
// effect-row label. Zero is invalid.
type Operation uint32

// Type is a contract-local frozen static type handle. Its canonical bytes are
// owned by the Contract; it never exposes a typ.Type.
type Type uint32

// Values is a contract-local Lua Values relation: a fixed Type prefix, one
// explicit tail, and a fixed end-relative suffix. Zero is invalid.
type Values uint32

// Protocol is one sealed nominal lifecycle contract. Its identity is the
// complete canonical set of acquisition result coordinates; zero is invalid.
type Protocol uint32

// State is a one-based dense coordinate within a Protocol. It is never a
// global state identity.
type State uint32

// StateRef is a one-based state coordinate used only while authoring a
// ProtocolSpec. Seal resolves and discards it.
type StateRef uint32

// TypeFormal, ValueFormal, ValuesVar, and RowVar are zero-based, operation-
// scoped formal coordinates. They are never global identities.
type (
	TypeFormal  uint32
	ValueFormal uint32
	ValuesVar   uint32
	RowVar      uint32
)

// InitialRoot, BootShape, InitialValue, and ExactKey are sealed Target-owned
// structural ABI coordinates. ExactKey names one canonical typed Lua table-key
// atom; it is neither a string convention nor a Program/Link key handle. Zero
// is invalid for each handle.
type (
	InitialRoot  uint32
	BootShape    uint32
	InitialValue uint32
	ExactKey     uint32
	// TransferID is one sealed Target transfer declaration. It is contract
	// local and replaces an authored transfer ordinal once Seal completes.
	TransferID uint32
)

// BootAggregate identifies the static aggregate schema of one boot root.
// Heap alone owns every dynamic shape conclusion after actor boot.
type BootAggregate uint8

const (
	BootAggregateInvalid BootAggregate = iota
	BootAggregateTable
	BootAggregateMetatable
)

// InitialMutability is the initial policy of one exact root/key row. Later
// writes remain ordinary Heap behavior and do not change this Target record.
type InitialMutability uint8

const (
	InitialMutabilityInvalid InitialMutability = iota
	InitialMutable
	InitialFrozen
)

// InitialBindingClass is derived only from the InitialValueKind of one exact
// unshadowed global slot. It is the frozen three-way disposition, not a second
// classification authority: the exact InitialValue remains queryable.
type InitialBindingClass uint8

const (
	InitialBindingInvalid InitialBindingClass = iota
	InitialBindingAdmitted
	InitialBindingDenied
	InitialBindingOrdinary
)

// InitialValueKind gives an exact structural identity to one initial value.
// Root is an InitialRoot alias, Operation is a sealed ordinary target
// operation, and DeniedOperation is an exact unsupported binding identity
// with its one later typed rejection law. Numeric and string literals are a
// closed typed union; no textual literal codec participates in this ABI.
type InitialValueKind uint8

const (
	InitialValueInvalid InitialValueKind = iota
	InitialValueNil
	InitialValueBoolean
	InitialValueInteger
	InitialValueFloat
	InitialValueString
	InitialValueRoot
	InitialValueOperation
	InitialValueDeniedOperation
	InitialValueAbsent
)

// InitialValueSpec is authoring input for one exact boot value. Exactly the
// field selected by Kind is meaningful. FloatBits uses math.Float64bits's
// exact IEEE-754 representation; Root names an InitialRootSpec and Operation
// names an exact BindingSpec. No runtime value can enter this ABI.
type InitialValueSpec struct {
	Kind      InitialValueKind
	Boolean   bool
	Integer   int64
	FloatBits uint64
	String    string
	Root      string
	Operation BindingSpec
}

// BootShapeSpec is Target's immutable ABI row for a boot aggregate.  Immutable
// is the initial whole-object table-header state; it is distinct from the
// InitialMutability policy of any exact root/key entry.  Heap projects this
// declaration into its initial Frozen header, then owns every later header
// transition.  Its identity is (initial-root identity, aggregate schema,
// immutable header, initial value).
type BootShapeSpec struct {
	Aggregate BootAggregate
	Immutable bool
	Value     InitialValueSpec
}

// InitialRootSpec declares one exact boot aggregate and its mandatory shape.
type InitialRootSpec struct {
	Identity string
	Shape    BootShapeSpec
}

// InitialEntrySpec is one exact initial root/key/value/mutability row. Key is
// normalized under Lua table-key equality at Seal, never a wildcard or path.
type InitialEntrySpec struct {
	Root       string
	Key        keyspace.LiteralValue
	Value      InitialValueSpec
	Mutability InitialMutability
}

// InitialMetatableAttachmentSpec declares one immutable bootstrap attachment
// from an existing primitive initial-value kind to one metatable root.  It is
// deliberately only boot structure: setmetatable and every later attachment
// are mutable Heap state, not Target truth.
//
// Base reuses InitialValueKind so this relation cannot mint a second runtime
// base-kind vocabulary.  The current profile admits only InitialValueString.
type InitialMetatableAttachmentSpec struct {
	Base      InitialValueKind
	Metatable string
}

// InitialBindingSpec names one exact unshadowed global slot and binds it to
// the corresponding initial-root ledger row. Its class is derived from the
// sealed value. Program later owns the CellGlobal/GlobalBinding relation;
// Target owns only this boot disposition.
type InitialBindingSpec struct {
	Name string
	Root string
	Key  keyspace.LiteralValue
}

// InputSourceKind identifies one operation-scoped input coordinate. AllInputs
// is derived solely for the opaque Operation; ordinary authoring cannot claim
// that authority.
type InputSourceKind uint8

const (
	InputSourceInvalid InputSourceKind = iota
	InputSourceValueFormal
	InputSourceValuesVar
	InputSourceAllInputs
)

// InputSource selects an existing operation input coordinate. Ordinal is zero-
// based for ValueFormal and ValuesVar and must be zero for AllInputs.
type InputSource struct {
	Kind    InputSourceKind
	Ordinal uint32
}

// TransferPossibility is the exact observable relation between an existing
// operation Outcome and one selected source graph. MayDeliver preserves that
// graph's contents and internal aliases as observed at the operation
// occurrence; later sender mutation cannot alter that observation. MayReject
// permits no delivery from this row. Both bits may be present when the same
// outcome Values cannot distinguish delivery from rejection. The relation
// prescribes no runtime copy, move, COW, allocation, or placement strategy.
type TransferPossibility uint8

const (
	TransferMayDeliver TransferPossibility = 1 << iota
	TransferMayReject
)

// TransferOutcomeSpec classifies one zero-based authoring Outcome ordinal.
// Seal remaps it to the canonical Outcome ordinal.
type TransferOutcomeSpec struct {
	Outcome     uint32
	Possibility TransferPossibility
}

// TransferEndpointKind identifies where a transfer is observed. A fixed input
// endpoint is an exact ValueFormal of the owning operation; External is an
// intentionally unmodelled external endpoint. It is not an actor, PID, or
// runtime handle.
type TransferEndpointKind uint8

const (
	TransferEndpointInvalid TransferEndpointKind = iota
	TransferEndpointInput
	TransferEndpointExternal
)

// TransferEndpoint is one exact transfer destination. Input is meaningful
// only for TransferEndpointInput and must be zero for TransferEndpointExternal.
type TransferEndpoint struct {
	Kind  TransferEndpointKind
	Input ValueFormal
}

// TransferIdentity records the source-level identity relation between the
// payload before the operation and the delivered payload. It deliberately
// prescribes neither allocation nor a runtime copy strategy.
type TransferIdentity uint8

const (
	TransferIdentityInvalid TransferIdentity = iota
	TransferIdentityUnspecified
	TransferIdentitySame
	TransferIdentityDistinct
)

// TransferCapabilities records the complete capability preservation relation.
// Partial labels are intentionally not part of this static boundary.
type TransferCapabilities uint8

const (
	TransferCapabilitiesInvalid TransferCapabilities = iota
	TransferCapabilitiesUnspecified
	TransferCapabilitiesPreserveAll
	TransferCapabilitiesLoseAll
)

// TransferSpec declares one endpoint/content-and-alias relation and a total
// classification of its owning Operation's Outcomes. It carries no call-form
// correspondence, runtime strategy, domain conclusion, or second identity.
type TransferSpec struct {
	Endpoint     TransferEndpoint
	Payload      InputSource
	Alias        InputSource
	Identity     TransferIdentity
	Capabilities TransferCapabilities
	Outcomes     []TransferOutcomeSpec
}

// StateSpec declares a nominal state symbol. Name is an artifact symbol, not
// a rule: all target semantics use the sealed State coordinate.
type StateSpec struct {
	Name  string
	Final bool
}

// AcquisitionSpec creates State for the fixed Result slot of one exact
// outcome from an operation. It introduces no resource or operation identity.
type AcquisitionSpec struct {
	Operation SpecRef
	Outcome   uint32
	Result    uint32
	State     StateRef
}

// TransitionOutcomeSpec applies To only when its owning operation produces
// the exact named outcome.
type TransitionOutcomeSpec struct {
	Outcome uint32
	To      StateRef
}

// TransitionSpec observes From at invocation entry for one existing input
// coordinate, then applies one of Outcomes after the named outcome.
type TransitionSpec struct {
	Operation SpecRef
	Input     InputSource
	From      StateRef
	Outcomes  []TransitionOutcomeSpec
}

type EscapeSpec struct {
	Operation SpecRef
	Input     InputSource
}

// ProtocolSpec owns one nominal protocol and its complete acquisition set.
// There is deliberately no free-form protocol name or second operation ID.
type ProtocolSpec struct {
	Acquisitions    []AcquisitionSpec
	States          []StateSpec
	Transitions     []TransitionSpec
	Escapes         []EscapeSpec
	CallbackHolders []ProtocolCallbackHolderSpec
}

// ProtocolCallbackHolderSpec states that one retained callback port holds the
// resource selected by Input for the duration prescribed by its lifecycle.
// Operation and Callback are authoring-only references; Seal retains the
// exact Protocol × Operation × InputSource × CallbackID relation. It does not
// create an event, holder, or reference-count identity.
type ProtocolCallbackHolderSpec struct {
	Operation SpecRef
	Input     InputSource
	Callback  CallbackRef
}

// SpecRef is a one-based reference into Spec.Operations. It exists only while
// authoring the one-shot input; Seal resolves and discards it before Operation
// identities are assigned.
type SpecRef uint32

// CallbackRef addresses an operation-local CallbackSpec while authoring. Like
// SpecRef it is one-based; zero is invalid. Seal replaces it with CallbackID.
type CallbackRef uint32

// CallbackID is the sealed, contract-local identity of one callback
// correspondence. It is intentionally not a callable identity.
type CallbackID uint32

// SpawnID is the sealed identity of one detached coroutine.spawn
// correspondence.  It is not a continuation, carrier, or child activation
// identity; those remain in the existing suspension, callback, Values, and
// outcome relations it names.
type SpawnID uint32

// SpawnSiblingAlternative is one concrete legal order of the two events
// enabled when a spawn carrier consumes its parent suspension generation.
// Both alternatives are required: the relation deliberately has no implicit
// scheduling preference or ordering factor.
type SpawnSiblingAlternative uint8

const (
	SpawnSiblingInvalid SpawnSiblingAlternative = iota
	SpawnChildEntryThenParentResume
	SpawnParentResumeThenChildEntry
)

// TerminalSpec assigns one complete operation-scoped Values relation to one
// activation terminal. A callback or inline Subedge callee must author exactly
// one Normal, Return, Throw, Yield, and Cancel row. It says nothing about how
// the owner reacts to that terminal.
type TerminalSpec struct {
	Kind   flowkind.OutcomeKind
	Values ValuesSpec
}

// SubedgeRef addresses an operation-local authored SubedgeSpec. It is a
// one-based authoring coordinate only; Seal replaces it with SubedgeID.
type SubedgeRef uint32

// SubedgeID is the sealed contract-local identity of one typed internal
// application edge. It is an existing Target relation, never a Program call,
// Link Application, Candidate, scheduler occurrence, or a second fact plane.
type SubedgeID uint32

// SubedgeFamily is the closed structural application family. There is no
// generic "meta" or "access" bucket: each family carries the exact operand
// convention that later Link/Factor consumers project.
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

// Admission is the callable convention of one callback or inline
// callee. DirectFunction excludes __call delegation; OrdinaryCallable permits
// the ordinary callable relation. Callback-backed Subedges reuse their
// CallbackSpec admission instead of carrying a second authority.
type Admission uint8

const (
	AdmissionInvalid Admission = iota
	DirectFunction
	OrdinaryCallable
)

// SubedgeRoute is the complete handling of a callee terminal. Outcome and
// Subedge routes place a contextual projected Result into an existing target
// port; Continue hands that Result to the named owning operation-law consumer.
// PropagateYield preserves the existing child Yield without inventing an owner
// suspension. RejectYield discards that child payload, synthesizes the one
// canonical C-boundary error Values, and routes it to an owner Throw or a
// sibling Subedge (including self-recursive xpcall handler re-entry).
type SubedgeRoute uint8

const (
	RouteInvalid SubedgeRoute = iota
	RouteOutcome
	RouteSubedge
	RouteContinue
	RoutePropagateYield
	RouteRejectYield
)

// Adjustment is the only result-transport choice. Preserve requires exact
// source/destination Values equality. Exact derives scalar, fixed-N, or
// discard semantics solely from the destination's closed Values shape; it has
// no separate arity authority.
type Adjustment uint8

const (
	AdjustmentInvalid Adjustment = iota
	AdjustmentPreserve
	AdjustmentExact
)

// CapturedInitialReadSpec names one boot-root exact-key read captured once
// when its owning operation relation is entered. It is a structural callee
// source, not a reread-per-execution global lookup or a string convention.
// The sealed SubedgeID is the operation-local capture identity.
type CapturedInitialReadSpec struct {
	Root string
	Key  keyspace.LiteralValue
}

// SubedgeCalleeKind selects the complete source of a Call-family Subedge.
// Other families derive their callable selection from their family and typed
// Values operands, so they carry no Callee.
type SubedgeCalleeKind uint8

const (
	SubedgeCalleeInvalid SubedgeCalleeKind = iota
	SubedgeCalleeCallback
	SubedgeCalleeCapturedInitialRead
	SubedgeCalleeMetaKey
)

// SubedgeCalleeSpec is the closed authoring union for Call-family callees.
// Exactly the field selected by Kind is meaningful; Seal rejects every mixed
// or incomplete form. MetaKey is an exact typed selector, never a rebuilt
// metatable-name string.
type SubedgeCalleeSpec struct {
	Kind     SubedgeCalleeKind
	Callback CallbackRef
	Read     CapturedInitialReadSpec
	MetaKey  keyspace.LiteralValue
}

// Placement says where a terminal's transported Values enters an existing
// destination Values relation. Tail binds one source tail variable after the
// destination's fixed prefix. Fixed starts at Offset in a closed destination;
// the selected arity is the projected Result's fixed width, never a second
// field or the destination's remaining width.
type Placement uint8

const (
	PlacementInvalid Placement = iota
	PlacementTail
	PlacementFixed
)

// SubedgeRouteSpec handles one exact callee terminal. Result is the contextual
// full Values endpoint after Adjustment; equal Values handles never imply flow
// outside this (SubedgeID, terminal, result-role) relation. Outcome is a
// zero-based authored owner outcome for Outcome or RejectYield; Subedge is a
// one-based authored sibling relation for Subedge or RejectYield. RejectYield
// therefore describes C-boundary error delivery, not a fake child outcome.
// Placement maps Result to an existing destination; Continue and
// PropagateYield have no destination.
type SubedgeRouteSpec struct {
	Kind       flowkind.OutcomeKind
	Route      SubedgeRoute
	Adjustment Adjustment
	Result     ValuesSpec
	Placement  Placement
	Offset     uint32
	Outcome    uint32
	Subedge    SubedgeRef
}

// ArgumentSegment is one coordinate of a contextual Subedge argument
// Values relation. It deliberately names Values structure rather than a
// reusable endpoint: equal Values schemas at two Subedges never identify a
// flow edge.
type ArgumentSegment uint8

const (
	ArgumentSegmentInvalid ArgumentSegment = iota
	ArgumentFixed
	ArgumentSuffix
	ArgumentTail
)

// ArgumentSource states the sole authority for one argument
// segment. Input reads the owner operation's existing input coordinate;
// Rule means the owner operation Rule constructs that segment. It is not an
// expression language, port graph, or second Values carrier.
type ArgumentSource uint8

const (
	ArgumentSourceInvalid ArgumentSource = iota
	ArgumentSourceInput
	ArgumentSourceRule
)

// ArgumentOrigin gives one complete source for a fixed, suffix, or
// variable-tail argument segment. Index is zero-based for Fixed and Suffix and
// must be zero for Tail. Source is meaningful only for Input.
type ArgumentOrigin struct {
	Segment ArgumentSegment
	Index   uint32
	Kind    ArgumentSource
	Source  InputSource
}

// AdmissionRouteSpec is the terminal transport vocabulary applied to
// the distinct admission-failure source. It intentionally has no Kind: its
// enclosing AdmissionFailureSpec is already the exact source, so it
// cannot be mistaken for the callee's Throw terminal.
//
// Only owner Outcome and sibling Subedge destinations are valid. Result,
// Adjustment, Placement, and Offset have the same laws as SubedgeRouteSpec.
type AdmissionRouteSpec struct {
	Route      SubedgeRoute
	Adjustment Adjustment
	Result     ValuesSpec
	Placement  Placement
	Offset     uint32
	Outcome    uint32
	Subedge    SubedgeRef
}

// AdmissionFailureSpec is the complete distinct failure relation for
// an attempted callable admission. Values is authored at the exact operation
// law boundary: it may be a protected false/error result, an error payload
// passed to xpcall's handler, or a direct owner Throw. It is never candidate
// absence and never aliases the callee Throw terminal.
type AdmissionFailureSpec struct {
	Values ValuesSpec
	Route  AdmissionRouteSpec
}

// SubedgeSpec is one typed internal application relation. Role is a nonzero
// operation-local semantic coordinate, stable across authoring order; it
// distinguishes repeated structurally identical internal applications.
// Callback-backed edges derive their Arguments, terminals, and admission from CallbackSpec.
// Other closed families own their full Values ports and admission directly.
// Their exact metamethod selector is derived by Family and dynamic access keys
// remain Values operands; there is no speculative site-key vocabulary.
type SubedgeSpec struct {
	Role      uint32
	Family    SubedgeFamily
	Callee    SubedgeCalleeSpec
	Admission Admission
	Arguments ValuesSpec
	// RuleEntry is the nullary form of ArgumentSourceRule. An empty argument
	// product has no segment on which to place an ArgumentOrigin, so this bool
	// explicitly states that the owner Rule may invoke the Subedge. Nonempty
	// arguments use the complete ArgumentOrigins relation instead.
	RuleEntry        bool
	ArgumentOrigins  []ArgumentOrigin
	Outcomes         []TerminalSpec
	AdmissionFailure AdmissionFailureSpec
	Routes           []SubedgeRouteSpec
}

// CallbackLifecycle is the complete static causal contract for one callback
// correspondence. Sync callbacks are confined to their owning operation and
// complete before its terminal completion; an intermediate Yield does not end
// that lifetime. Retained callbacks may outlive the owning operation's
// terminal outcome. A retained callback may author one explicit release only
// when a source-visible target operation causes it.
// Optional/Required describe whether the callback may be absent, and
// Once/Many describe semantic invocation multiplicity rather than an
// implementation budget.
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

// CallbackReleaseMode describes how one source-visible release operation
// discharges a retained callback relation. It is a semantic multiplicity, not
// a scheduler policy or reference-count instruction.
type CallbackReleaseMode uint8

const (
	CallbackReleaseInvalid CallbackReleaseMode = iota
	CallbackReleaseOne
	CallbackReleaseAll
)

// CallbackReleaseZeroBehavior is the declared behavior when the retained
// callback holder is already absent. It is Target operation semantics, not a
// scheduler policy or a typestate conclusion.
type CallbackReleaseZeroBehavior uint8

const (
	CallbackReleaseZeroInvalid CallbackReleaseZeroBehavior = iota
	CallbackReleaseZeroSuppress
	CallbackReleaseZeroThrow
	CallbackReleaseZeroIdempotent
)

// CallbackReleaseZeroSpec is the zero-holder arm of a retained callback
// release. Outcome is an authored outcome ordinal only for Throw and
// Idempotent. Suppress carries no outcome coordinate and therefore requires
// Outcome to be zero.
type CallbackReleaseZeroSpec struct {
	Behavior CallbackReleaseZeroBehavior
	Outcome  uint32
}

// CallbackReleaseSpec ties a retained callback to a source-visible release
// operation. Input is a fixed ValueFormal of that release operation and
// Outcome is its zero-based authored outcome ordinal. Seal resolves both
// coordinates and retains no authoring reference.
type CallbackReleaseSpec struct {
	Operation SpecRef
	Input     ValueFormal
	Outcome   uint32
	Mode      CallbackReleaseMode
	Zero      CallbackReleaseZeroSpec
}

// ResumeID is the sealed, contract-local identity of one resumption
// correspondence. It is intentionally neither a continuation nor a runtime
// activation identity.
type ResumeID uint32

// ReentrySource identifies the closed authority that can restore a suspended
// operation. It is not a scheduler, continuation, or event identity.
type ReentrySource uint8

const (
	ReentrySourceInvalid ReentrySource = iota
	// ReentryByCall is supplied by a dynamically matched ordinary Call.
	ReentryByCall
	// ReentryByProvider is supplied by the operation's sealed provider ABI.
	ReentryByProvider
)

// ReentryMultiplicity says whether one live suspension is discharged by its
// first restoration or remains available for later restorations. Source-level
// recurrence remains Program Mu; it is never represented by this enum.
type ReentryMultiplicity uint8

const (
	ReentryMultiplicityInvalid ReentryMultiplicity = iota
	ReentryOnce
	ReentryMany
)

// ResumeSource identifies the activation operand of a resumption operation.
// Produced means the ordinary operation itself was selected through a
// Produced result; it does not mint another callable or continuation handle.
type ResumeSource uint8

const (
	ResumeSourceInvalid ResumeSource = iota
	ResumeSourceValueFormal
	ResumeSourceProduced
)

// BindingNamespace distinguishes the closed source/provider binding spaces.
type BindingNamespace uint8

const (
	BindingBuiltin BindingNamespace = iota + 1
	BindingModule
	BindingProvider
)

// ValuesTail distinguishes a closed Lua Values list, an operation-scoped
// Values formal tail, and an explicitly unknown tail.
type ValuesTail uint8

const (
	ValuesClosed ValuesTail = iota + 1
	ValuesVariable
	ValuesUnknown
)

// FreshKind is the closed reference-runtime kind of one nominal result root.
// It is deliberately a small runtime vocabulary: the fixed Values result
// remains the carrier identity and this row only proves freshness.
type FreshKind uint8

const (
	FreshInvalid FreshKind = iota
	FreshTable
	FreshFunction
	FreshThread
	FreshUserdata
	FreshError
	FreshReflection
)

// RowTail distinguishes a closed authored Koka row, an operation-scoped row
// variable, and the explicit opaque-boundary open row.
type RowTail uint8

const (
	RowClosed RowTail = iota + 1
	RowVariable
	RowUnknownOpen
)

// BindingSpec is one exact unjoined binding identity. Owner identifies a
// module/provider when Namespace requires it; Member is its exported path.
// Segments are retained as individual strings, never a reconstructed path.
type BindingSpec struct {
	Namespace BindingNamespace
	Owner     []string
	Member    []string
}

// CallbackSpec is static callback correspondence owned by one operation.
// Function selects one existing fixed ValueFormal input and Admission gives
// its callable convention even when this retained callback has no immediate
// Subedge. Arguments and every terminal are full Values relations; a
// ValuesVariable tail inside those relations is only an operation-scoped tail
// binder. Effects is the callback's expected Koka row. Release is optional and
// valid only for retained lifecycles; it never invents a scheduler occurrence.
// Runtime scheduling remains in the operation Rule.
type CallbackSpec struct {
	Function  InputSource
	Admission Admission
	Arguments ValuesSpec
	Outcomes  []TerminalSpec
	Lifecycle CallbackLifecycle
	Effects   RowSpec
	Release   *CallbackReleaseSpec
}

// CallbackResultSpec ties one fixed result slot of an outcome to an
// operation-local callback. The result Value itself remains the sole carrier
// identity; this is only static correspondence.
type CallbackResultSpec struct {
	Result   uint32
	Callback CallbackRef
}

// ResultAliasSpec ties one fixed prefix result slot of an outcome to an
// existing ValueFormal input. The result and input retain their own identities;
// this is only static correspondence.
type ResultAliasSpec struct {
	Result uint32
	Source InputSource
}

// GsubTableReplacementSpec is the one closed string.gsub replacement-table
// branch.  It is deliberately not a callback or a generic subedge recipe:
// Lua chooses the first capture when present and otherwise the complete match
// as the dynamic table key, then performs this exact IndexGet access.
// ResultOutcome and Result correlate the completed substitution to the
// existing outer result. EffectAliases name existing owning-operation effects.
type GsubTableReplacementSpec struct {
	Replacement   ValueFormal
	Access        SubedgeRef
	ResultOutcome uint32
	Result        uint32
	EffectAliases []uint32
}

// SuspensionSpec relates two exact outcome cases of its owning operation.
// Yield and Reentry are zero-based authoring outcome ordinals and are remapped
// to canonical ordinals by Seal. Their Values are the existing core Values
// relations; this row never introduces a second pack vocabulary.
type SuspensionSpec struct {
	Yield        uint32
	Reentry      uint32
	Source       ReentrySource
	Multiplicity ReentryMultiplicity
}

// SpawnSpec binds a system-yielding parent operation to one detached child
// callback. Function and Child deliberately name the same existing input
// authority; Child carries the existing complete five-outcome callback
// relation. ChildEntry and ParentResume are zero-based authored parent outcome
// ordinals whose existing Values relations must both be the closed empty Pack.
// Yield/ParentResume also name the existing one-shot provider suspension.
// Alternatives is the complete two-order sibling causal relation.
type SpawnSpec struct {
	Function     InputSource
	Child        CallbackRef
	Yield        uint32
	ParentResume uint32
	ChildEntry   uint32
	Alternatives []SpawnSiblingAlternative
}

// ResumeOutcomeSpec maps one terminal outcome of the restored activation to
// one authored outcome of the owning resumption operation. The activation
// outcome and operation outcome have deliberately separate kinds: for
// example, coroutine.resume turns a restored Throw into its false Normal
// result. Seal requires exactly one mapping for Normal, Return, Throw, Yield,
// and Cancel; Break and Goto cannot cross an activation boundary.
type ResumeOutcomeSpec struct {
	Kind    flowkind.OutcomeKind
	Outcome uint32
}

// ResumeSpec declares that an ordinary operation can restore an activation.
// A ValueFormal source consumes the exact existing formal at Carrier. A
// Produced source is selected through the owning ordinary Produced Operation;
// the CallbackID remains on the producer capture/result relation. Arguments is
// the complete operation-local Values relation supplied at restoration.
// Outcomes is the complete cross-activation terminal correspondence. At a
// dynamic match, Arguments instantiates the matched Suspension reentry Values
// relation. The restored terminal payload then instantiates the mapped owning
// outcome's existing ValuesVariable tail; a mapped closed outcome explicitly
// discards that payload. Unknown Values are forbidden for mapped outcomes by
// Seal.
type ResumeSpec struct {
	Source    ResumeSource
	Carrier   ValueFormal
	Arguments ValuesSpec
	Outcomes  []ResumeOutcomeSpec
}

// CaptureKind selects one producer-side source retained by a
// callable-valued outcome. The source is zero-based for ValueFormal,
// TypeValueFormal, and ValuesVar, and one-based CallbackRef for Callback.
type CaptureKind uint8

const (
	CaptureInvalid CaptureKind = iota
	CaptureValueFormal
	// CaptureTypeValueFormal retains the exact runtime TypeValue carried by
	// one fixed input formal. It is a distinct semantic claim from retaining
	// the input value itself. Seal requires the owning Produced result to have
	// the exact same-result FreshFunction relation.
	CaptureTypeValueFormal
	CaptureValuesVar
	CaptureCallback
)

// CaptureSpec is one ordered retained source. Presence in the
// list is the retention law; there is no separate lifetime mode or closure ID.
type CaptureSpec struct {
	Kind    CaptureKind
	Ordinal uint32
}

// ProducedSpec makes one fixed result slot of an outcome invoke an
// ordinary target Operation. Result cannot designate an open Values tail.
type ProducedSpec struct {
	Result    uint32
	Operation SpecRef
	Captures  []CaptureSpec
}

// FreshResultSpec proves that one fixed outcome result is a newly allocated
// nominal runtime root. Seal derives its dense outcome-local ordinal after
// sorting by Result, so authoring order cannot affect nominal identity.
type FreshResultSpec struct {
	Result uint32
	Kind   FreshKind
}

// ValuesSpec is the authoring form of a Lua Values relation. Fixed elements
// are typ authoring inputs only; a sealed Contract retains frozen Type handles.
type ValuesSpec struct {
	Fixed    []typ.Type
	Tail     ValuesTail
	Var      ValuesVar
	TailType typ.Type
	Suffix   []typ.Type
}

// OutcomeSpec is one finite correlated operation outcome case. Several cases
// may share a kind and Values relation only when their FreshResults differ;
// Produced, callback-result, and alias rows remain conjunctive annotations.
type OutcomeSpec struct {
	Kind            flowkind.OutcomeKind
	Values          ValuesSpec
	Produced        []ProducedSpec
	FreshResults    []FreshResultSpec
	CallbackResults []CallbackResultSpec
	ResultAliases   []ResultAliasSpec
}

// EffectSpec is one authored Koka effect occurrence. Each argument vector is
// checked against Target's ABI after SpecRef resolution. RowArgs carries the
// target operation's row formal substitution. Publication is absent unless an
// author explicitly declares one; ordinary Koka effects never imply memory
// publication by name or by their argument shape.
type EffectSpec struct {
	Target      SpecRef
	ValueArgs   []ValueFormal
	TypeArgs    []TypeFormal
	ValuesArgs  []ValuesVar
	RowArgs     []RowVar
	Publication *PublicationEffectSpec
}

// PublicationEffectKind is the closed semantic operation performed by one
// explicitly authored effect occurrence. It is Target semantic authority, not
// a Program spelling convention and not a runtime placement conclusion.
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

// PublicationDestinationRole selects the optional destination-context formal
// in the effect target operation. No role means that the event has no
// statically-authenticated destination context.
type PublicationDestinationRole uint8

const (
	PublicationDestinationInvalid PublicationDestinationRole = iota
	PublicationDestinationNone
	PublicationDestinationValueFormal
)

// PublicationEscapeDisposition is the exact escape effect, if any, declared
// by a publication operation.
type PublicationEscapeDisposition uint8

const (
	PublicationEscapeInvalid PublicationEscapeDisposition = iota
	PublicationEscapeNone
	PublicationEscapeSendTransfer
	PublicationEscapeReturn
	PublicationEscapeCallback
)

// PublicationMutabilityDisposition is the static mutability transition
// declared by the target semantic operation. Runtime ownership/COW decisions
// remain later authenticated conclusions.
type PublicationMutabilityDisposition uint8

const (
	PublicationMutabilityInvalid PublicationMutabilityDisposition = iota
	PublicationMutabilityPreserve
	PublicationMutabilitySeal
	PublicationMutabilityWrite
	PublicationMutabilityCopyOnWrite
)

// PublicationLifetimeDisposition is the static lifetime transition declared
// by the target semantic operation.
type PublicationLifetimeDisposition uint8

const (
	PublicationLifetimeInvalid PublicationLifetimeDisposition = iota
	PublicationLifetimePreserve
	PublicationLifetimeRelease
)

// PublicationEffectSpec explicitly attaches memory-relevant semantics to one
// effect occurrence. Subject and Destination are zero-based ValueFormal
// selectors in the resolved effect target ABI; Destination is meaningful only
// for PublicationDestinationValueFormal.
//
// The exact valid combinations are checked while sealing. A nil Publication
// remains absent rather than being inferred from generic effect metadata.
type PublicationEffectSpec struct {
	Kind        PublicationEffectKind
	Subject     ValueFormal
	Destination PublicationDestinationRole
	Context     ValueFormal
	Escape      PublicationEscapeDisposition
	Mutability  PublicationMutabilityDisposition
	Lifetime    PublicationLifetimeDisposition
}

// PublicationEffectDescriptor is the immutable Target-owned projection of an
// explicitly authored PublicationEffectSpec. It can only be obtained from a
// sealed Contract query; its fields intentionally remain private so callers
// cannot forge a descriptor to splice into another owner.
type PublicationEffectDescriptor struct {
	kind        PublicationEffectKind
	subject     ValueFormal
	destination PublicationDestinationRole
	context     ValueFormal
	escape      PublicationEscapeDisposition
	mutability  PublicationMutabilityDisposition
	lifetime    PublicationLifetimeDisposition
}

func (d PublicationEffectDescriptor) Kind() PublicationEffectKind { return d.kind }
func (d PublicationEffectDescriptor) Subject() ValueFormal        { return d.subject }
func (d PublicationEffectDescriptor) DestinationRole() PublicationDestinationRole {
	return d.destination
}
func (d PublicationEffectDescriptor) Context() ValueFormal { return d.context }
func (d PublicationEffectDescriptor) Escape() PublicationEscapeDisposition {
	return d.escape
}
func (d PublicationEffectDescriptor) Mutability() PublicationMutabilityDisposition {
	return d.mutability
}
func (d PublicationEffectDescriptor) Lifetime() PublicationLifetimeDisposition {
	return d.lifetime
}

// RowSpec is an authored Koka effect row. Multiplicity in Occurrences is
// semantic and survives sealing.
type RowSpec struct {
	Occurrences []EffectSpec
	Tail        RowTail
	Var         RowVar
}

// OperationSpec is one target operation authoring input. Bindings are an
// equivalent canonical source-spelling set; only produced-only operations
// have none. ValuesVars counts open Values parameters. ValueFormal
// coordinates are exactly Input.Fixed ordinals. Program owns Lua call syntax:
// a colon call contributes its receiver as Input.Fixed[0], so target has no
// separate call-form or receiver plane.
type OperationSpec struct {
	Bindings             []BindingSpec
	TypeFormals          []*typ.TypeParam
	ValuesVars           uint32
	RowFormals           uint32
	Input                ValuesSpec
	Outcomes             []OutcomeSpec
	Callbacks            []CallbackSpec
	Subedges             []SubedgeSpec
	Suspensions          []SuspensionSpec
	Spawns               []SpawnSpec
	Resumes              []ResumeSpec
	Transfers            []TransferSpec
	GsubTableReplacement *GsubTableReplacementSpec
	Effects              RowSpec
}

// Spec is a one-shot authoring container. Seal consumes it on its first
// attempt, including an attempt that fails validation.
type Spec struct {
	Operations        []OperationSpec
	Protocols         []ProtocolSpec
	InitialRoots      []InitialRootSpec
	InitialEntries    []InitialEntrySpec
	InitialBindings   []InitialBindingSpec
	InitialMetatables []InitialMetatableAttachmentSpec
	consumed          bool
}

type operationRow struct {
	bindings    indexRange
	input       Values
	outcomes    indexRange
	valuesTypes indexRange
	callbacks   indexRange
	subedges    indexRange
	suspensions indexRange
	spawns      indexRange
	resumes     indexRange
	transfers   indexRange
	gsubTable   uint32
	releases    indexRange
	effects     indexRange
	typeFormals indexRange
	valuesVars  uint32
	rowFormals  uint32
	effectTail  RowTail
	effectVar   RowVar
}

type bindingRange struct {
	namespace  BindingNamespace
	owner      indexRange
	member     indexRange
	ownerKeys  indexRange
	memberKeys indexRange
}

type initialRootRow struct {
	identity string
	shape    BootShape
}

type bootShapeRow struct {
	root      InitialRoot
	aggregate BootAggregate
	immutable bool
	value     InitialValue
}

type initialValueRow struct {
	kind      InitialValueKind
	boolean   bool
	integer   int64
	floatBits uint64
	string    string
	root      InitialRoot
	operation Operation
	binding   uint32
}

type initialEntryRow struct {
	root       InitialRoot
	key        ExactKey
	value      InitialValue
	mutability InitialMutability
}

type initialBindingRow struct {
	name string
	root InitialRoot
	key  ExactKey
}

type initialMetatableAttachmentRow struct {
	base      InitialValueKind
	metatable InitialRoot
}

type valuesRow struct {
	owner  Operation
	types  indexRange
	tail   ValuesTail
	varID  ValuesVar
	suffix indexRange
}

type typeRow struct {
	owner Operation
	bytes []byte
}

type outcomeRow struct {
	kind            flowkind.OutcomeKind
	values          Values
	produced        indexRange
	fresh           indexRange
	callbackResults indexRange
	resultAliases   indexRange
}

type freshResultRow struct {
	result  uint32
	ordinal uint32
	kind    FreshKind
}

type callbackResultRow struct {
	result   uint32
	callback CallbackID
}

type resultAliasRow struct {
	result uint32
	source InputSource
}

type suspensionRow struct {
	yield        uint32
	reentry      uint32
	source       ReentrySource
	multiplicity ReentryMultiplicity
}

type spawnRow struct {
	owner        Operation
	function     InputSource
	child        CallbackID
	yield        uint32
	parentResume uint32
	childEntry   Values
	resumeValues Values
	alternatives [2]SpawnSiblingAlternative
}

type resumeRow struct {
	owner     Operation
	source    ResumeSource
	carrier   ValueFormal
	arguments Values
	outcomes  [5]uint32
}

type transferRow struct {
	owner        Operation
	endpoint     TransferEndpoint
	payload      InputSource
	alias        InputSource
	identity     TransferIdentity
	capabilities TransferCapabilities
	outcomes     indexRange
}

type protocolRow struct {
	acquisitions    indexRange
	states          indexRange
	transitions     indexRange
	escapes         indexRange
	callbackHolders indexRange
}

type stateRow struct {
	name  string
	final bool
}

type acquisitionRow struct {
	operation Operation
	outcome   uint32
	result    uint32
	state     State
}

type transitionRow struct {
	operation Operation
	input     InputSource
	from      State
	outcomes  indexRange
}

type transitionOutcomeRow struct {
	outcome uint32
	to      State
}

type escapeRow struct {
	operation Operation
	input     InputSource
}

type protocolCallbackHolderRow struct {
	operation Operation
	input     InputSource
	callback  CallbackID
}

type callbackRow struct {
	owner      Operation
	function   InputSource
	admission  Admission
	arguments  Values
	outcomes   [5]Values
	lifecycle  CallbackLifecycle
	subedge    SubedgeID
	effects    indexRange
	effectTail RowTail
	effectVar  RowVar
	release    uint32
}

type subedgeRow struct {
	owner            Operation
	role             uint32
	family           SubedgeFamily
	callee           SubedgeCalleeKind
	callback         CallbackID
	readRoot         InitialRoot
	readKey          ExactKey
	metaKey          ExactKey
	admission        Admission
	arguments        Values
	ruleEntry        bool
	argumentOrigins  indexRange
	outcomes         [5]Values
	admissionFailure Values
	admissionRoute   subedgeRouteRow
	routes           [5]subedgeRouteRow
}

type subedgeArgumentOriginRow struct {
	segment ArgumentSegment
	index   uint32
	kind    ArgumentSource
	source  InputSource
}

type subedgeRouteRow struct {
	route       SubedgeRoute
	adjustment  Adjustment
	result      Values
	placement   Placement
	offset      uint32
	outcome     uint32
	subedge     SubedgeID
	destination Values
}

type callbackReleaseRow struct {
	callback     CallbackID
	operation    Operation
	input        ValueFormal
	outcome      uint32
	mode         CallbackReleaseMode
	zeroBehavior CallbackReleaseZeroBehavior
	zeroOutcome  uint32
}

type producedRow struct {
	result           uint32
	target           Operation
	captures         indexRange
	typeValueCapture uint32 // relative capture index; noTypeValueCapture when absent
}

type captureRow struct {
	kind    CaptureKind
	ordinal uint32
}

type effectRow struct {
	target         Operation
	values         indexRange
	types          indexRange
	valuesVar      indexRange
	rows           indexRange
	publication    PublicationEffectDescriptor
	hasPublication bool
}

type indexRange struct{ start, end uint32 }

type bindingIndexRow struct {
	binding   uint32
	operation Operation
}

// Contract is immutable after Seal. Every slice is private and every public
// hot query returns only scalar handles or values.
type Contract struct {
	operations         []operationRow
	types              []typeRow
	values             []valuesRow
	valueTypes         []Type
	outcomes           []outcomeRow
	effects            []effectRow
	effectVals         []ValueFormal
	effectType         []TypeFormal
	effectVars         []ValuesVar
	valuesVarTypes     []Type
	effectRows         []RowVar
	formals            []Type
	bindings           []bindingRange
	callbacks          []callbackRow
	subedges           []subedgeRow
	subedgeOrigins     []subedgeArgumentOriginRow
	callbackResults    []callbackResultRow
	resultAliases      []resultAliasRow
	suspensions        []suspensionRow
	spawns             []spawnRow
	resumes            []resumeRow
	transfers          []transferRow
	gsubTables         []gsubTableReplacementRow
	gsubEffects        []uint32
	transferOutcomes   []TransferPossibility
	callbackReleases   []callbackReleaseRow
	protocols          []protocolRow
	states             []stateRow
	acquisitions       []acquisitionRow
	transitions        []transitionRow
	transitionOutcomes []transitionOutcomeRow
	escapes            []escapeRow
	callbackHolders    []protocolCallbackHolderRow
	produced           []producedRow
	fresh              []freshResultRow
	captures           []captureRow
	segments           []string
	bindingKeys        []ExactKey
	lookup             []bindingIndexRow
	initialRoots       []initialRootRow
	exactKeys          []keyspace.LiteralValue
	bootShapes         []bootShapeRow
	initialValues      []initialValueRow
	initialValueBinds  []bindingRange
	initialEntries     []initialEntryRow
	initialBindings    []initialBindingRow
	initialMetatables  []initialMetatableAttachmentRow
	globalEnvRoot      InitialRoot
	initialAbsent      InitialValue
	semanticReceipt    SemanticSourceReceipt
	// semantic identity columns are sealed with the contract.  They are not a
	// second graph authority: each row is a cached canonical descriptor owned
	// by Target and indexed only by the existing dense Target tables.
	operationAnchors []keyspace.ContentID
	// Effect identity columns are likewise only projections of the existing
	// operation/callback/effect tables.  Effect descriptors intentionally have
	// no inverse index: duplicate authored occurrences are distinct evidence,
	// while their descriptor identity is the shared semantic quotient.
	effectOperationIDs      []keyspace.ContentID
	effectDescriptorIDs     []keyspace.ContentID
	effectOccurrenceIDs     []keyspace.ContentID
	operationEffectFamilies []keyspace.ContentID
	callbackEffectFamilies  []keyspace.ContentID
	operationContentIDs     []keyspace.ContentID
	callbackSelectors       []keyspace.ContentID
	callbackContentIDs      []keyspace.ContentID
	callbackContentIndex    []callbackContentIDRow
	outcomeSelectors        []keyspace.ContentID
	outcomeContentIDs       []keyspace.ContentID
	transferContentIDs      []keyspace.ContentID
	transferOutcomeIDs      []keyspace.ContentID
	resumeContentIDs        []keyspace.ContentID
	resumeContentIndex      []resumeContentIDRow
	inputFormalRanges       []indexRange
	inputFormalIDs          []keyspace.ContentID
	inputFormalIndex        []inputFormalIDRow
	outcomeResultRanges     []indexRange
	outcomeResultIDs        []keyspace.ContentID
	outcomeResultIndex      []outcomeResultIDRow
	initialValueContentIDs  []keyspace.ContentID
	bootRelationID          keyspace.ContentID
	boundCount              uint32
	opaque                  Operation
	sealed                  bool
}

type inputFormalIDRow struct {
	id     keyspace.ContentID
	op     Operation
	formal ValueFormal
}

type outcomeResultIDRow struct {
	id      keyspace.ContentID
	op      Operation
	outcome uint32
	result  uint32
}

// callbackContentIDRow and resumeContentIDRow are the immutable sorted
// reverse columns for the Target-owned portable relation identities. The
// forward columns remain dense by their existing sealed handles; these rows
// retain no authoring ordinal or secondary lookup map.
type callbackContentIDRow struct {
	id       keyspace.ContentID
	op       Operation
	callback CallbackID
}

type resumeContentIDRow struct {
	id     keyspace.ContentID
	op     Operation
	resume ResumeID
}

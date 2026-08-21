package vocabulary

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

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
	Admission schematype.CallableAdmission
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

// SubedgeRelationSpec records one neutral correlation between an operation's
// existing value formal, one existing Subedge, and one existing outcome
// result. Selector is an opaque domain-supplied discriminator; Target stores
// it for canonical identity but does not assign it domain meaning. Effect
// aliases name existing owning-operation effects.
type SubedgeRelationSpec struct {
	Operand       ValueFormal
	Selector      uint32
	Subedge       SubedgeRef
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
	_ CaptureKind = iota // ordinal zero is reserved
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
	Kind   schematype.FreshClass
}

// ValuesSpec is the authoring form of a Lua Values relation. Fixed elements
// are neutral schema declarations supplied by the owning domain; a sealed
// Contract retains frozen Type handles.
type ValuesSpec struct {
	Fixed    []schematype.Type
	Tail     ValuesTail
	Var      ValuesVar
	TailType schematype.Type
	Suffix   []schematype.Type
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

// OperationResultSpec declares one neutral relation between an operation's
// fixed result and an existing operation input. Relation is an opaque schema
// entry identity supplied by the owner of the behavior vocabulary. Target
// stores and authenticates it but never interprets its category, spelling, or
// ordinal. Outcome is the zero-based authored outcome ordinal and is resolved
// to the sealed canonical outcome while Target seals the operation.
//
// A provider can therefore declare that a result classifies an input without
// teaching Target what the class means. The runtime-kind provider uses this
// shape for its result rows; other domains may use the same shape for a
// different declared vocabulary.
type OperationResultSpec struct {
	Outcome  uint32
	Result   uint32
	Source   InputSource
	Relation schema.EntryID
}

// OperationPredicateSpec declares one neutral predicate relation between an
// operation's fixed result and an existing input subject. Relation is opaque
// provider-issued schema identity, exactly as in OperationResultSpec. The
// branch consumer decides whether the observed predicate is used positively
// or negatively; Target only preserves the declared correspondence.
type OperationPredicateSpec struct {
	Outcome  uint32
	Result   uint32
	Subject  InputSource
	Relation schema.EntryID
}

// OperationBehaviorSpec is the optional behavior descriptor attached to one
// OperationSpec. It contains declarations only: no builtin name, runtime
// value, domain enum, evaluator, or execution strategy crosses this boundary.
// Empty or nil behavior is equivalent to no behavior rows.
type OperationBehaviorSpec struct {
	Results    []OperationResultSpec
	Predicates []OperationPredicateSpec
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
	_ PublicationDestinationRole = iota // ordinal zero is reserved
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
// effect occurrence. Subject selects one existing ValueFormal or the target
// input ValuesVar in the resolved effect target ABI; Destination is meaningful
// only for PublicationDestinationValueFormal, whose Context remains a
// ValueFormal selector.
//
// The exact valid combinations are checked while sealing. A nil Publication
// remains absent rather than being inferred from generic effect metadata.
type PublicationEffectSpec struct {
	Kind        PublicationEffectKind
	Subject     InputSource
	Destination PublicationDestinationRole
	Context     ValueFormal
	Escape      PublicationEscapeDisposition
	Mutability  PublicationMutabilityDisposition
	Lifetime    PublicationLifetimeDisposition
}

// FormalEffectKind is the closed ownership metadata vocabulary attached to an
// operation declaration. Formal effects are deliberately separate from the
// invocation Effects row and from PublicationEffectSpec: they describe the
// operation's neutral ownership contract, not an effect occurrence or a
// publication event.
type FormalEffectKind uint8

const (
	FormalEffectInvalid FormalEffectKind = iota
	FormalEffectBorrow
	FormalEffectRetain
	FormalEffectStore
	FormalEffectBorrowAll
	FormalEffectSendSuffix
	FormalEffectSendParam
	FormalEffectExport
	FormalEffectOpaque
	FormalEffectFreeze
)

// FormalEffectSpec is one canonical ownership metadata row. Param is retained
// as a signed int32 because -1 is a meaningful neutral unknown parameter. Store
// has an explicit optional Into coordinate; absent Into values are canonicalized
// to -1 during Target seal. SendSuffix uses FromParam as a non-negative
// parameter-list boundary.
type FormalEffectSpec struct {
	Kind      FormalEffectKind
	Param     int32
	Into      int32
	HasInto   bool
	FromParam int32
}

// FormalEffectRow is the operation-owned formal ownership row. Unlike an
// invocation RowSpec it has no row-variable coordinate: only a finite closed
// row or the explicit opaque unknown-open row is admitted.
type FormalEffectRow struct {
	Occurrences []FormalEffectSpec
	Tail        RowTail
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
	Bindings        []BindingSpec
	TypeFormals     []TypeFormalSpec
	ValuesVars      uint32
	RowFormals      uint32
	Input           ValuesSpec
	Outcomes        []OutcomeSpec
	Behavior        *OperationBehaviorSpec
	Callbacks       []CallbackSpec
	Subedges        []SubedgeSpec
	Suspensions     []SuspensionSpec
	Spawns          []SpawnSpec
	Resumes         []ResumeSpec
	Transfers       []TransferSpec
	SubedgeRelation *SubedgeRelationSpec
	Effects         RowSpec
	FormalEffects   FormalEffectRow
}

// TypeFormalSpec declares one operation-local formal coordinate. The
// constraint is already a neutral schema declaration; its domain adapter
// validates and encodes it before target sealing. An unavailable constraint
// means the formal is unconstrained.
type TypeFormalSpec struct {
	Constraint schematype.Type
}

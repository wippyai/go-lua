// Package target owns the sealed, portable target-operation contract.
//
// It deliberately owns only static operation ABI, Values, authored Koka rows,
// outcomes, bindings, callback correspondence and expected effect rows,
// explicit retained-release relations, produced operations, operation-owned
// suspension, and observable endpoint/payload/alias transfer authority. Protocol,
// project linking, and analysis remain later layers keyed by Operation.
package target

import "github.com/wippyai/go-lua/analysis/program/keyspace"

type Operation uint32

// Type is a contract-local frozen static type handle. Its portable declaration
// is owned by the schema type-contract envelope; target never decodes it.
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

package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

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
	Admission schematype.CallableAdmission
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

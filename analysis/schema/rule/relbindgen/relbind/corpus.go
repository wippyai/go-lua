package relbind

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// The reason the two remaining raw-access reductions carry.
//
// Nothing about the operands blocks them any longer: the owner publishes both
// frames constructibly, the authored plan states every read the reductions
// make, and a delivered span now says which row each position carries. What is
// left is the decoder that maps each delivered row to the owner tag its
// lookup is keyed by, and that is a binding to write rather than a statement
// anyone owes. The rows stay declared and named until it is written, because a
// row that claims a binding it does not have is worse than a named debt.
const abiGapRawReduction = "w0-decoder-unwritten: every operand is reachable and the delivered spans carry their row identity, and the decoder that keys each selection by the owner tag its lookup takes is not written yet"

// span is the delivery a read observes every row of. A key selection is one
// such read: its length is the count the owner's enumeration is stated
// against, and a candidate whose key is static reads no row at all.
const span = signature.BoundedSpanDelivery

// The four reasons the remaining census families carry. Each is a statement
// the compiler makes about an operand, not one this layer asserts.
const (
	// A judgment that reads the operand type of the protocol this engine
	// replaces cannot be reached from a frame. A binding delivers owner values
	// and spans; it cannot construct an execution selection, and the compiler
	// says so: "cannot use cells (variable of struct type
	// relbindgen.Span[value.Value]) as []execution.SelectedCell[value.Value]".
	abiGapLegacyOperand = "w0-legacy-operand: the owner judgment reads execution.SelectedCell or execution.SummaryVector, the operand type of the protocol this engine replaces, and a binding delivers owner values and spans"
	// Two judgments answer one family and they are not the same function. The
	// declared reducer is reached by nothing but its own laws while the rule
	// the analyzer runs calls another with a different signature, so which one
	// a binding carries is the owner's statement to make and not this layer's.
	abiGapDivergentJudgment = "w0-divergent-judgment: the declared reducer is referenced only by its own laws and the executing rule calls a differently shaped function, so the family states two judgments and the owner has not said which one answers"
	// An operation that publishes a disposition and no fact has no result type
	// to instantiate the bounded emitter with, and the compiler says so:
	// "cannot use nil as struct{} value in argument to relbindgen.Reduce".
	abiGapDispositionOnly = "w0-disposition-only: the judgment answers with a disposition and publishes no fact, and the semantic ABI types its emitter and its output columns by the value a family produces"
	// A declared reducer slot with no bound implementation is a family whose
	// mathematics has not been written yet.
	abiGapAbsentJudgment = "w0-absent-judgment: the reducer slot carries no Go implementation and the package holds no fold-shaped judgment at all, so there is nothing for a binding to reach"
)

// scalar is the delivery a single-cell input carries.
const scalar = signature.ScalarDelivery

// rawAccessRouteBound is the declared output bound every raw-access route
// expansion carries. It is the bound the authored plan states, so the emitter
// refuses the row that would exceed it rather than truncating the expansion.
const rawAccessRouteBound = 64

// axes are the emission targets. Every produced artifact lives beside the
// owner whose mathematics it carries, so the generic substrate keeps no
// dependency on any domain and each owner keeps its own binding surface.
func axes() []Axis {
	return []Axis{
		{Key: "value", Dir: "domain/value/relation", Package: "relation"},
		{Key: "heap", Dir: "domain/heap/relation", Package: "relation"},
		{Key: "pack", Dir: "domain/pack/relation", Package: "relation"},
		{Key: "call", Dir: "domain/call/relation", Package: "relation"},
		{Key: "effect", Dir: "domain/effect/relation", Package: "relation"},
		{Key: "static", Dir: "domain/static/relation", Package: "relation"},
		{Key: "placement", Dir: "domain/placement/relation", Package: "relation"},
	}
}

// Declared is the whole binding corpus: every owner payload the families share
// and one row per census family that compiles.
//
// A row is stated whether or not it can be bound. A row that cannot names the
// gap that stops it, so the corpus is total over the census and a family is
// never absent because it was hard.
func Declared() Corpus {
	return Corpus{Axes: axes(), Payloads: payloads(), Families: families()}
}

// payloads is one row per owner payload type that crosses the boundary. A
// payload with a Lattice is a fact column whose TypeID carries ascent
// authority; a payload without one is a candidate carrier that is only ever
// decoded.
func payloads() []Payload {
	return []Payload{
		{Key: "value", Axis: "value", Field: "Value", Type: "valuedomain.Value", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value", Lattice: "ValueLattice"},
		{Key: "heap", Axis: "heap", Field: "Heap", Type: "heapdomain.Value", Alias: "heapdomain", Path: "github.com/wippyai/go-lua/domain/heap", Lattice: "HeapLattice"},
		{Key: "call", Axis: "call", Field: "Call", Type: "calldomain.Value", Alias: "calldomain", Path: "github.com/wippyai/go-lua/domain/call", Lattice: "CallLattice"},
		{Key: "effect", Axis: "effect", Field: "Effect", Type: "effectfactor.Value", Alias: "effectfactor", Path: "github.com/wippyai/go-lua/domain/effect/factor", Lattice: "EffectLattice"},
		{Key: "pack", Axis: "pack", Field: "Pack", Type: "packdomain.Value", Alias: "packdomain", Path: "github.com/wippyai/go-lua/domain/pack", Lattice: "PackLattice"},
		{Key: "placement", Axis: "placement", Field: "Placement", Type: "placementdomain.Fact", Alias: "placementdomain", Path: "github.com/wippyai/go-lua/domain/placement", Lattice: "PlacementLattice"},
		{Key: "static", Axis: "static", Field: "Static", Type: "staticdomain.TypeFact", Alias: "staticdomain", Path: "github.com/wippyai/go-lua/domain/static", Lattice: "StaticLattice"},

		{Key: "arithmetic-candidate", Axis: "value", Field: "ArithmeticCandidate", Type: "valuedomain.BinaryArithmetic", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "equality-candidate", Axis: "value", Field: "EqualityCandidate", Type: "valuedomain.BinaryEquality", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "order-candidate", Axis: "value", Field: "OrderCandidate", Type: "valuedomain.BinaryOrder", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "refinement-candidate", Axis: "value", Field: "RefinementCandidate", Type: "valuedomain.PresenceRefinement", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "runtimekind-candidate", Axis: "value", Field: "RuntimeKindCandidate", Type: "valuedomain.RuntimeKindCall", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "moduleload-candidate", Axis: "value", Field: "ModuleLoadCandidate", Type: "valuedomain.ModuleLoadCall", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "allocation-candidate", Axis: "value", Field: "AllocationCandidate", Type: "*valuedomain.AllocationResult", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "globalbootstrap-candidate", Axis: "value", Field: "GlobalBootstrapCandidate", Type: "*valuedomain.GlobalBootstrapResult", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "valuesource-candidate", Axis: "value", Field: "ValueSourceCandidate", Type: "valuedomain.SourceSeed", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "heap-candidate", Axis: "heap", Field: "HeapCandidate", Type: "heapdomain.Key", Alias: "heapdomain", Path: "github.com/wippyai/go-lua/domain/heap"},
		{Key: "packsource-candidate", Axis: "pack", Field: "PackSourceCandidate", Type: "packdomain.Source", Alias: "packdomain", Path: "github.com/wippyai/go-lua/domain/pack"},
		{Key: "call-candidate", Axis: "call", Field: "CallCandidate", Type: "calldomain.CallCoordinate", Alias: "calldomain", Path: "github.com/wippyai/go-lua/domain/call"},
		{Key: "value-summary", Axis: "value", Field: "ValueSummary", Type: "valuedomain.ValueSummaryObservation", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "heap-key-route", Axis: "heap", Field: "KeyRoute", Type: "KeyRouteFact"},
		{Key: "heap-call-route", Axis: "heap", Field: "CallRoute", Type: "CallRouteFact"},
		{Key: "heap-route", Axis: "heap", Field: "HeapRoute", Type: "HeapRouteFact"},
		{Key: "heap-pack-route", Axis: "heap", Field: "PackRoute", Type: "PackRouteFact"},
		{Key: "heap-source-route", Axis: "heap", Field: "SourceRoute", Type: "SourceRouteFact"},
		{Key: "heap-route-tag", Axis: "heap", Field: "HeapRouteTag", Type: "uint64"},
		{Key: "placement-requirement", Axis: "placement", Field: "Requirement", Type: "placementdomain.Placement", Alias: "placementdomain", Path: "github.com/wippyai/go-lua/domain/placement"},
		{Key: "placement-route-tag", Axis: "placement", Field: "PlacementRouteTag", Type: "uint64"},
		{Key: "value-result-slot", Axis: "value", Field: "ResultSlotCandidate", Type: "valuedomain.MountedCallResultSlot", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "value-fresh-result-call", Axis: "value", Field: "FreshResultCandidate", Type: "valuedomain.FreshResultCall", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "value-storage-transfer", Axis: "value", Field: "StorageTransferCandidate", Type: "valuedomain.StorageTransfer", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "value-coordinate", Axis: "value", Field: "ValueCoordinate", Type: "valuedomain.Coordinate", Alias: "valuedomain", Path: "github.com/wippyai/go-lua/domain/value"},
		{Key: "value-route-tag", Axis: "value", Field: "ValueRouteTag", Type: "uint64"},
		{Key: "heap-read-candidate", Axis: "heap", Field: "ReadCandidate", Type: "indexdomain.Index", Alias: "indexdomain", Path: "github.com/wippyai/go-lua/domain/heap/index"},
		{Key: "heap-write-candidate", Axis: "heap", Field: "WriteCandidate", Type: "indexdomain.Index", Alias: "indexdomain", Path: "github.com/wippyai/go-lua/domain/heap/index"},
		{Key: "effect-candidate", Axis: "effect", Field: "EffectCandidate", Type: "effectfactor.MountedCall", Alias: "effectfactor", Path: "github.com/wippyai/go-lua/domain/effect/factor"},
	}
}

// families is one row per census family that compiles. A census row whose
// declaration lowers to two Apply nodes states two rows here, because the
// semantic ABI counts operations and not declarations.
func families() []Family {
	return []Family{
		{
			Census: "value/arithmetic", Rule: "value-binary-arithmetic", Stem: "ValueArithmetic", Axis: "value",
			Judgment: "ValueArithmeticOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "arithmetic-candidate", Delivery: scalar},
				{Field: "Left", Payload: "value", Delivery: scalar},
				{Field: "Right", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/equality", Rule: "value-binary-equality", Stem: "ValueEquality", Axis: "value",
			Judgment: "ValueEqualityOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "equality-candidate", Delivery: scalar},
				{Field: "Left", Payload: "value", Delivery: scalar},
				{Field: "Right", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/order", Rule: "value-binary-order", Stem: "ValueOrder", Axis: "value",
			Judgment: "ValueOrderOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "order-candidate", Delivery: scalar},
				{Field: "Left", Payload: "value", Delivery: scalar},
				{Field: "Right", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/refinement", Rule: "value-presence-refinement", Stem: "ValueRefinement", Axis: "value",
			Judgment: "ValueRefinementOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "refinement-candidate", Delivery: scalar},
				{Field: "Fact", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/runtimekind", Rule: "value-runtime-kind-call", Stem: "ValueRuntimeKind", Axis: "value",
			Judgment: "ValueRuntimeKindOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "runtimekind-candidate", Delivery: scalar},
				{Field: "Dispatched", Payload: "call", Delivery: scalar},
				{Field: "Subject", Payload: "value", Delivery: scalar},
				{Field: "Comparison", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/moduleload", Rule: "value-callresult-moduleload", Stem: "ValueModuleLoad", Axis: "value",
			Judgment: "ValueModuleLoadOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "moduleload-candidate", Delivery: scalar},
				{Field: "Argument", Payload: "value", Delivery: scalar},
				{Field: "Dispatched", Payload: "call", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/transfer", Rule: "value-transfer", Stem: "ValueTransfer", Axis: "value",
			Judgment: "ValueTransferOperation",
			Inputs: []Slot{
				{Field: "Source", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/source", Rule: "value-source", Stem: "ValueSource", Axis: "value",
			Judgment: "ValueSourceOperation",
			Inputs: []Slot{
				{Field: "Seed", Payload: "valuesource-candidate", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/bootstrap", Rule: "value-bootstrap", Stem: "ValueBootstrap", Axis: "value",
			Judgment: "ValueBootstrapOperation",
			Inputs: []Slot{
				{Field: "Result", Payload: "globalbootstrap-candidate", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/allocation", Rule: "value-allocation", Stem: "ValueAllocation", Axis: "value",
			Judgment: "ValueAllocationOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "allocation-candidate", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/allocation", Rule: "value-allocation", Stem: "ValueAllocationAge", Axis: "value",
			Judgment: "ValueAllocationAgeOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "allocation-candidate", Delivery: scalar},
				{Field: "Prior", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "heap/allocation/ingress", Rule: "heap-ingress", Stem: "HeapIngress", Axis: "heap",
			Judgment: "HeapIngressOperation",
			Inputs: []Slot{
				{Field: "Key", Payload: "heap-candidate", Delivery: scalar},
			},
			Result: "heap", Outputs: []Column{{Payload: "heap"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "heap/bootstrap", Rule: "heap-bootstrap", Stem: "HeapBootstrap", Axis: "heap",
			Judgment: "HeapBootstrapOperation",
			Inputs: []Slot{
				{Field: "Key", Payload: "heap-candidate", Delivery: scalar},
			},
			Result: "heap", Outputs: []Column{{Payload: "heap"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "heap/allocation/empty", Rule: "heap-empty", Stem: "HeapEmptyAllocation", Axis: "heap",
			Judgment: "HeapEmptyAllocationOperation",
			Inputs: []Slot{
				{Field: "Key", Payload: "heap-candidate", Delivery: scalar},
				{Field: "Predecessor", Payload: "heap", Delivery: scalar},
			},
			Result: "heap", Outputs: []Column{{Payload: "heap"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "heap/allocation/empty", Rule: "heap-empty", Stem: "HeapEmptyAllocationAge", Axis: "heap",
			Judgment: "HeapEmptyAllocationAgeOperation",
			Inputs: []Slot{
				{Field: "Key", Payload: "heap-candidate", Delivery: scalar},
				{Field: "Prior", Payload: "heap", Delivery: scalar},
			},
			Result: "heap", Outputs: []Column{{Payload: "heap"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "pack/source", Rule: "pack-source", Stem: "PackSource", Axis: "pack",
			Judgment: "PackSourceOperation",
			Inputs: []Slot{
				{Field: "Source", Payload: "packsource-candidate", Delivery: scalar},
			},
			Result: "pack", Outputs: []Column{{Payload: "pack"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "call/dispatch", Rule: "call-dispatch", Stem: "CallDispatch", Axis: "call",
			Judgment: "CallDispatchOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "call-candidate", Delivery: scalar},
				{Field: "Callee", Payload: "value", Delivery: scalar},
			},
			Result: "call", Outputs: []Column{{Payload: "call"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "effect/callsite/opaque", Rule: "effect-opaque", Stem: "EffectOpaqueCallSite", Axis: "effect",
			Judgment: "EffectOpaqueCallSiteOperation",
			Inputs: []Slot{
				{Field: "Mounted", Payload: "effect-candidate", Delivery: scalar},
				{Field: "Dispatched", Payload: "call", Delivery: scalar},
			},
			Result: "effect", Outputs: []Column{{Payload: "effect"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "effect/callsite/selected", Rule: "effect-selected", Stem: "EffectSelectedCallSite", Axis: "effect",
			Judgment: "EffectSelectedCallSiteOperation",
			Inputs: []Slot{
				{Field: "Mounted", Payload: "effect-candidate", Delivery: scalar},
				{Field: "Dispatched", Payload: "call", Delivery: scalar},
			},
			Result: "effect", Outputs: []Column{{Payload: "effect"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "static/transfer", Rule: "static-transfer", Stem: "StaticTransfer", Axis: "static",
			Judgment: "StaticTransferOperation",
			Inputs: []Slot{
				{Field: "Source", Payload: "static", Delivery: scalar},
			},
			Result: "static", Outputs: []Column{{Payload: "static"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Arm: "grouped reduction over a complete span", Stem: "ValueSummary", Axis: "value",
			Judgment: "ValueSummaryOperation",
			Inputs: []Slot{
				{Field: "Cells", Payload: "value", Delivery: signature.CompleteSpanDelivery},
				{Field: "Group", Payload: "value", Delivery: scalar},
			},
			Result: "value-summary", Outputs: []Column{{Payload: "value-summary"}},
			Cardinality: model.ExactlyOne, Address: 1,
		},
		{
			Arm: "finite expansion at owner-named rows", Stem: "HeapReceiverRoutes", Axis: "heap",
			Judgment: "HeapReceiverRoutesOperation",
			Inputs: []Slot{
				{Field: "Receiver", Payload: "value", Delivery: scalar},
			},
			Result: "heap-route", Outputs: []Column{{Payload: "heap-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Census: "heap/index", Rule: "raw-get", Stem: "RawGetKeyRoutes", Axis: "heap",
			Judgment: "RawGetKeyRoutesOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "heap-read-candidate", Delivery: scalar},
				{Field: "Receiver", Payload: "value", Delivery: scalar},
			},
			Result: "heap-key-route", Outputs: []Column{{Payload: "heap-key-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Census: "heap/index", Rule: "raw-get", Stem: "RawGetCallRoutes", Axis: "heap",
			Judgment: "RawGetCallRoutesOperation",
			Inputs: []Slot{
				{Field: "Key", Payload: "heap-key-route", Delivery: scalar},
				{Field: "Receiver", Payload: "value", Delivery: scalar},
			},
			Result: "heap-call-route", Outputs: []Column{{Payload: "heap-call-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Census: "heap/index", Rule: "raw-get", Stem: "RawGetSourceRoutes", Axis: "heap",
			Judgment: "RawGetSourceRoutesOperation",
			Inputs: []Slot{
				{Field: "Pack", Payload: "heap-pack-route", Delivery: scalar},
				{Field: "Values", Payload: "pack", Delivery: scalar},
			},
			Result: "heap-source-route", Outputs: []Column{{Payload: "heap-source-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Census: "heap/index", Rule: "raw-set", Stem: "RawSetKeyRoutes", Axis: "heap",
			Judgment: "RawSetKeyRoutesOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "heap-write-candidate", Delivery: scalar},
				{Field: "Receiver", Payload: "value", Delivery: scalar},
			},
			Result: "heap-key-route", Outputs: []Column{{Payload: "heap-key-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Census: "heap/index", Rule: "raw-set", Stem: "RawSetSourceRoutes", Axis: "heap",
			Judgment: "RawSetSourceRoutesOperation",
			Inputs: []Slot{
				{Field: "Pack", Payload: "heap-pack-route", Delivery: scalar},
				{Field: "Values", Payload: "pack", Delivery: scalar},
			},
			Result: "heap-source-route", Outputs: []Column{{Payload: "heap-source-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Arm: "cell update at the row it read", Stem: "HeapAscent", Axis: "heap",
			Judgment: "HeapAscentOperation",
			Inputs: []Slot{
				{Field: "Current", Payload: "heap", Delivery: scalar},
				{Field: "Proposed", Payload: "heap", Delivery: scalar},
			},
			Result: "heap", Outputs: []Column{{Payload: "heap"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "heap/formalfreeze", Rule: "heap-formal-freeze", Stem: "HeapFormalFreeze", Axis: "heap",
			Judgment: "HeapFormalFreezeOperation",
			Inputs: []Slot{
				{Field: "RouteTag", Payload: "heap-route-tag", Delivery: scalar},
				{Field: "Predecessor", Payload: "heap", Delivery: scalar},
			},
			Result: "heap", Outputs: []Column{{Payload: "heap"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "heap/publicationfreeze", Rule: "heap-publication-freeze", Stem: "HeapPublicationFreeze", Axis: "heap",
			Judgment: "HeapPublicationFreezeOperation",
			Inputs: []Slot{
				{Field: "RouteTag", Payload: "heap-route-tag", Delivery: scalar},
				{Field: "Predecessor", Payload: "heap", Delivery: scalar},
			},
			Result: "heap", Outputs: []Column{{Payload: "heap"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "placement/publicationescape", Rule: "placement-publication-escape", Stem: "PlacementPublicationEscape", Axis: "placement",
			Judgment: "PlacementPublicationEscapeOperation",
			Inputs: []Slot{
				{Field: "Requirement", Payload: "placement-requirement", Delivery: scalar},
				{Field: "Current", Payload: "placement", Delivery: scalar},
			},
			Result: "placement", Outputs: []Column{{Payload: "placement"}},
			Cardinality: model.ExactlyOne, Address: 1,
		},
		{
			Census: "placement/returnescape", Rule: "placement-return-escape", Stem: "PlacementReturnEscape", Axis: "placement",
			Judgment: "PlacementReturnEscapeOperation",
			Inputs: []Slot{
				{Field: "RouteTag", Payload: "placement-route-tag", Delivery: scalar},
				{Field: "Current", Payload: "placement", Delivery: scalar},
			},
			Result: "placement", Outputs: []Column{{Payload: "placement"}},
			Cardinality: model.ExactlyOne, Address: 1,
		},
		{
			Census: "placement/transfer", Rule: "placement-transfer", Stem: "PlacementTransfer", Axis: "placement",
			Judgment: "PlacementTransferOperation",
			Inputs: []Slot{
				{Field: "RouteTag", Payload: "placement-route-tag", Delivery: scalar},
				{Field: "Selected", Payload: "placement", Delivery: scalar},
			},
			Result: "placement", Outputs: []Column{{Payload: "placement"}},
			Cardinality: model.ExactlyOne, Address: 1,
		},
		{
			Census: "value/freshresult", Rule: "value-callresult-freshresult", Stem: "ValueFreshResult", Axis: "value",
			Judgment: "ValueFreshResultOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "call-candidate", Delivery: scalar},
				{Field: "CallFact", Payload: "call", Delivery: scalar},
				{Field: "Destination", Payload: "value-coordinate", Delivery: scalar},
				{Field: "Tag", Payload: "value-route-tag", Delivery: scalar},
				{Field: "Prior", Payload: "value", Delivery: scalar},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "placement/allocationbirth", Rule: "placement-allocation-birth", Stem: "PlacementAllocationBirth", Axis: "placement",
			Judgment: "PlacementAllocationBirthOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "allocation-candidate", Delivery: scalar},
				{Field: "Allocated", Payload: "value", Delivery: scalar},
			},
			Result: "placement", Outputs: []Column{{Payload: "placement"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "placement/freshbirth", Rule: "placement-fresh-birth", Stem: "PlacementFreshBirth", Axis: "placement",
			Judgment: "PlacementFreshBirthOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "value-fresh-result-call", Delivery: scalar},
				{Field: "Result", Payload: "value", Delivery: scalar},
			},
			Result: "placement", Outputs: []Column{{Payload: "placement"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "placement/formal", Rule: "placement-formal", Stem: "PlacementFormal", Axis: "placement",
			Judgment: "PlacementFormalOperation",
			Inputs: []Slot{
				{Field: "RouteTag", Payload: "placement-route-tag", Delivery: scalar},
				{Field: "Selected", Payload: "placement", Delivery: scalar},
			},
			Result: "placement", Outputs: []Column{{Payload: "placement"}},
			Cardinality: model.ExactlyOne, Address: 1,
		},
		{
			Census: "placement/store", Rule: "placement-storage", Stem: "PlacementStorage", Axis: "placement",
			Judgment: "PlacementStorageOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "value-storage-transfer", Delivery: scalar},
				{Field: "Source", Payload: "value", Delivery: scalar},
				{Field: "RouteTag", Payload: "placement-route-tag", Delivery: scalar},
				{Field: "Selected", Payload: "placement", Delivery: scalar},
			},
			Result: "placement", Outputs: []Column{{Payload: "placement"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{Census: "effect/callsite/body", Rule: "effect-body", Stem: "EffectBodyCallSite", Axis: "effect", Pending: abiGapLegacyOperand},
		{
			Census: "value/bodyresult", Rule: "value-callresult-body", Stem: "ValueBodyResult", Axis: "value",
			Judgment: "ValueBodyResultOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "value-result-slot", Delivery: scalar},
				{Field: "Dispatched", Payload: "call", Delivery: scalar},
				{Field: "Cells", Payload: "value", Delivery: span},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{
			Census: "value/resultalias", Rule: "value-callresult-resultalias", Stem: "ValueResultAlias", Axis: "value",
			Judgment: "ValueResultAliasOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "value-result-slot", Delivery: scalar},
				{Field: "Dispatched", Payload: "call", Delivery: scalar},
				{Field: "Cells", Payload: "value", Delivery: span},
			},
			Result: "value", Outputs: []Column{{Payload: "value"}},
			Cardinality: model.ExactlyOne, Address: 0,
		},
		{Census: "placement/suspension", Rule: "placement-suspension", Stem: "PlacementSuspension", Axis: "placement", Pending: abiGapLegacyOperand},
		{Census: "placement/suspension-evidence", Rule: "placement-suspension-evidence", Stem: "PlacementSuspensionEvidence", Axis: "placement", Pending: abiGapLegacyOperand},
		{Census: "placement/capture", Rule: "placement-closure-capture", Stem: "PlacementClosureCapture", Axis: "placement", Pending: abiGapDivergentJudgment},
		{Census: "placement/containment", Rule: "placement-containment", Stem: "PlacementContainment", Axis: "placement", Pending: abiGapDivergentJudgment},
		{
			Census: "call/activation", Rule: "call-activation", Stem: "CallActivation", Axis: "call",
			Judgment: "CallActivationOperation",
			Inputs: []Slot{
				{Field: "Candidate", Payload: "call-candidate", Delivery: scalar},
				{Field: "Trigger", Payload: "call", Delivery: scalar},
			},
			Cardinality: model.Optional, Address: NoDestination,
		},
		{Census: "typestate", Rule: "typestate-obligation", Stem: "TypestateObligation", Axis: "placement", Pending: abiGapAbsentJudgment},
		{
			Census: "heap/index", Rule: "raw-get", Stem: "RawGetPackRoutes", Axis: "heap",
			Judgment: "RawGetPackRoutesOperation",
			Inputs: []Slot{
				{Field: "Route", Payload: "heap-route", Delivery: scalar},
				{Field: "Fact", Payload: "heap", Delivery: scalar},
				{Field: "Candidate", Payload: "heap-read-candidate", Delivery: scalar},
				{Field: "Key", Payload: "value", Delivery: span},
			},
			Result: "heap-pack-route", Outputs: []Column{{Payload: "heap-pack-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Census: "heap/index", Rule: "raw-set", Stem: "RawSetPackRoutes", Axis: "heap",
			Judgment: "RawSetPackRoutesOperation",
			Inputs: []Slot{
				{Field: "Route", Payload: "heap-route", Delivery: scalar},
				{Field: "Fact", Payload: "heap", Delivery: scalar},
				{Field: "Candidate", Payload: "heap-write-candidate", Delivery: scalar},
				{Field: "Key", Payload: "value", Delivery: span},
			},
			Result: "heap-pack-route", Outputs: []Column{{Payload: "heap-pack-route"}},
			Cardinality: model.BoundedMany, Bound: rawAccessRouteBound, Address: KeyedDestination,
		},
		{
			Census: "heap/index", Rule: "raw-get", Stem: "RawGetResult", Axis: "heap",
			Pending: abiGapRawReduction,
		},
		{
			Census: "heap/index", Rule: "raw-set", Stem: "RawSetCommit", Axis: "heap",
			Pending: abiGapRawReduction,
		},
	}
}

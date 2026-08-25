package relbind

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// abiGapRawAccess is the reason the two indexed raw-access rows carry. Their
// owner mathematics exists and is monotone, but the payload-tail expansion,
// the semantic-source expansion and both reductions are reachable only through
// symbols the owner does not export and through the operand type of the
// protocol this engine replaces. A binding cannot name them, so the rows are
// declared, named, and left unbound until the owner publishes them.
const abiGapRawAccess = "w0-abi-gap: domain/heap/index publishes its route and reduction mathematics only through unexported symbols and the legacy operand type"

// scalar is the delivery a single-cell input carries.
const scalar = signature.ScalarDelivery

// receiverRouteBound is the declared output bound of the receiver-route
// expansion. It is the sealed signature's own contract, and the emitter
// refuses the row that would exceed it rather than truncating the expansion.
const receiverRouteBound = 64

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
		{Key: "heap-route", Axis: "heap", Field: "HeapRoute", Type: "RouteFact"},
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
			Cardinality: model.BoundedMany, Bound: receiverRouteBound, Address: KeyedDestination,
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
			Census: "heap/index", Rule: "raw-get", Stem: "HeapIndexRawGet", Axis: "heap",
			Pending: abiGapRawAccess,
		},
		{
			Census: "heap/index", Rule: "raw-set", Stem: "HeapIndexRawSet", Axis: "heap",
			Pending: abiGapRawAccess,
		},
	}
}

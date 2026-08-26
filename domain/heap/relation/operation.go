package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// HeapIngressOperation is domain/heap's own ingress seed.
type HeapIngressOperation struct{}

// Available reports whether the operation carries its owner mathematics. The
// seed is a package function over a key that carries its own owner, so there
// is no derived state to hold and nothing to be unavailable.
func (HeapIngressOperation) Available() bool { return true }

// Evaluate answers one ingress coordinate.
func (HeapIngressOperation) Evaluate(argument HeapIngressArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	fact, reduction := heapdomain.IngressFact(argument.Key)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// HeapBootstrapOperation is domain/heap's own boot-root seed.
type HeapBootstrapOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (HeapBootstrapOperation) Available() bool { return true }

// Evaluate answers one sealed boot root.
func (HeapBootstrapOperation) Evaluate(argument HeapBootstrapArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	fact, reduction := heapdomain.BootFact(argument.Key)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// HeapEmptyAllocationOperation is domain/heap's own empty allocation fold.
type HeapEmptyAllocationOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (HeapEmptyAllocationOperation) Available() bool { return true }

// Evaluate answers one empty allocation against its predecessor cell.
func (HeapEmptyAllocationOperation) Evaluate(argument HeapEmptyAllocationArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	fact, reduction := heapdomain.EmptyAllocationFact(argument.Key, argument.Predecessor)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// HeapEmptyAllocationAgeOperation is domain/heap's own allocation carry: the
// transform every allocation-form rule ages its carried coordinates through.
type HeapEmptyAllocationAgeOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (HeapEmptyAllocationAgeOperation) Available() bool { return true }

// Evaluate ages one carried coordinate through its allocation key.
func (HeapEmptyAllocationAgeOperation) Evaluate(argument HeapEmptyAllocationAgeArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	fact, held := argument.Key.Age(argument.Prior)
	return relbindgen.Carried(emitter, fact, held)
}

// HeapAscentOperation is domain/heap's own ascent. It is monotone because it
// is the owner lattice's own join, so every proposal it returns is an ascent
// of the cell it read; nothing here proves that a second time.
type HeapAscentOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (HeapAscentOperation) Available() bool { return true }

// Evaluate ascends the read cell by the proposed heap fact.
func (HeapAscentOperation) Evaluate(argument HeapAscentArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	ascended, ok := heapdomain.Join(argument.Current, argument.Proposed)
	if !ok {
		return outcome.Refused
	}
	if !emitter.Put(ascended) {
		return outcome.Refused
	}
	return outcome.Produced
}

// HeapReceiverRoutesOperation is domain/heap/index's own receiver observation.
// It holds the sealed topology, whose root support is frozen at seal, and it
// names each emitted row by the owner-issued content of that route's root. It
// cannot mint a row: a name outside the mounted denominator is refused by the
// denominator witness rather than published.
type HeapReceiverRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewHeapReceiverRoutesOperation adopts one sealed heap topology.
func NewHeapReceiverRoutesOperation(topology *indexdomain.Topology) (HeapReceiverRoutesOperation, bool) {
	if topology == nil {
		return HeapReceiverRoutesOperation{}, false
	}
	return HeapReceiverRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation HeapReceiverRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate expands one receiver into its rooted routes. The expansion is
// finite under the declared denominator: the emitter's capacity is the sealed
// signature's own output bound, and the row that would exceed it refuses the
// whole invocation rather than truncating the observation.
func (operation HeapReceiverRoutesOperation) Evaluate(argument HeapReceiverRoutesArgument, emitter *relbindgen.Emitter[HeapRouteFact]) outcome.Code {
	admitted := true
	observed := operation.topology.VisitReceiver(argument.Receiver, nil, func(route indexdomain.Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			return true
		}
		tag, tagged := operation.topology.RawRouteTag(key, role)
		content, named := key.ContentID()
		if !tagged || !named || !emitter.PutAt(content, HeapRouteFact{Kind: route.Kind(), Key: key, Role: role, Tag: tag}) {
			admitted = false
			return false
		}
		return true
	})
	if !observed || !admitted {
		return outcome.Refused
	}
	if emitter.Len() == 0 {
		return outcome.NoCandidate
	}
	return outcome.Produced
}

// RawGetKeyRoutesOperation is domain/heap/index's own dynamic key selection.
// A read candidate whose key is static selects no coordinate, and that is a
// distinct answer rather than an empty expansion.
type RawGetKeyRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewRawGetKeyRoutesOperation adopts one sealed heap topology.
func NewRawGetKeyRoutesOperation(topology *indexdomain.Topology) (RawGetKeyRoutesOperation, bool) {
	if topology == nil {
		return RawGetKeyRoutesOperation{}, false
	}
	return RawGetKeyRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation RawGetKeyRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate answers the coordinate one read candidate's dynamic key selects.
func (operation RawGetKeyRoutesOperation) Evaluate(argument RawGetKeyRoutesArgument, emitter *relbindgen.Emitter[KeyRouteFact]) outcome.Code {
	return keyRoute(operation.topology, argument.Candidate, emitter)
}

// RawSetKeyRoutesOperation is the same selection for a write candidate.
type RawSetKeyRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewRawSetKeyRoutesOperation adopts one sealed heap topology.
func NewRawSetKeyRoutesOperation(topology *indexdomain.Topology) (RawSetKeyRoutesOperation, bool) {
	if topology == nil {
		return RawSetKeyRoutesOperation{}, false
	}
	return RawSetKeyRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation RawSetKeyRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate answers the coordinate one write candidate's dynamic key selects.
func (operation RawSetKeyRoutesOperation) Evaluate(argument RawSetKeyRoutesArgument, emitter *relbindgen.Emitter[KeyRouteFact]) outcome.Code {
	return keyRoute(operation.topology, argument.Candidate, emitter)
}

// keyRoute is the one statement of the dynamic key selection both directions
// perform. The coordinate is named by the identity its own schema issued, so
// the published row is addressed by the coordinate's owner and never by this
// layer.
func keyRoute(topology *indexdomain.Topology, candidate indexdomain.Index, emitter *relbindgen.Emitter[KeyRouteFact]) outcome.Code {
	coordinate, dynamic := candidate.DynamicKey()
	if !dynamic {
		return outcome.NoCandidate
	}
	name, named := topology.CoordinateName(coordinate)
	if !named || !emitter.PutAt(name, KeyRouteFact{Coordinate: coordinate}) {
		return outcome.Refused
	}
	return outcome.Produced
}

// RawGetCallRoutesOperation is domain/heap/index's own receiver call demand.
type RawGetCallRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewRawGetCallRoutesOperation adopts one sealed heap topology.
func NewRawGetCallRoutesOperation(topology *indexdomain.Topology) (RawGetCallRoutesOperation, bool) {
	if topology == nil {
		return RawGetCallRoutesOperation{}, false
	}
	return RawGetCallRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation RawGetCallRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate expands one receiver into the calls it demands, each row named by
// the call's own content identity.
func (operation RawGetCallRoutesOperation) Evaluate(argument RawGetCallRoutesArgument, emitter *relbindgen.Emitter[CallRouteFact]) outcome.Code {
	admitted := true
	observed := operation.topology.VisitReceiverCallDemand(argument.Receiver, func(key calldomain.Key, tag uint64) bool {
		name, named := key.ContentID()
		if !named || !emitter.PutAt(name, CallRouteFact{Key: key, Tag: tag}) {
			admitted = false
			return false
		}
		return true
	})
	if !observed || !admitted {
		return outcome.Refused
	}
	if emitter.Len() == 0 {
		return outcome.NoCandidate
	}
	return outcome.Produced
}

// RawGetSourceRoutesOperation is domain/heap/index's own semantic source
// enumeration for a read.
type RawGetSourceRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewRawGetSourceRoutesOperation adopts one sealed heap topology.
func NewRawGetSourceRoutesOperation(topology *indexdomain.Topology) (RawGetSourceRoutesOperation, bool) {
	if topology == nil {
		return RawGetSourceRoutesOperation{}, false
	}
	return RawGetSourceRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation RawGetSourceRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate expands one payload into the semantic sources it declares.
func (operation RawGetSourceRoutesOperation) Evaluate(argument RawGetSourceRoutesArgument, emitter *relbindgen.Emitter[SourceRouteFact]) outcome.Code {
	return sourceRoutes(operation.topology, argument.Pack.Payload, emitter)
}

// RawSetSourceRoutesOperation is the same enumeration for a write.
type RawSetSourceRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewRawSetSourceRoutesOperation adopts one sealed heap topology.
func NewRawSetSourceRoutesOperation(topology *indexdomain.Topology) (RawSetSourceRoutesOperation, bool) {
	if topology == nil {
		return RawSetSourceRoutesOperation{}, false
	}
	return RawSetSourceRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation RawSetSourceRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate expands one payload into the semantic sources it declares.
func (operation RawSetSourceRoutesOperation) Evaluate(argument RawSetSourceRoutesArgument, emitter *relbindgen.Emitter[SourceRouteFact]) outcome.Code {
	return sourceRoutes(operation.topology, argument.Pack.Payload, emitter)
}

// sourceRoutes is the one statement of the source enumeration both directions
// perform. Each source is published at the coordinate it names, under the
// identity that coordinate's own schema issued.
func sourceRoutes(topology *indexdomain.Topology, payload heapdomain.RawPayloadTag, emitter *relbindgen.Emitter[SourceRouteFact]) outcome.Code {
	admitted := true
	observed := topology.VisitPayloadSources(payload, func(tag indexdomain.RawSourceTag, coordinate valuedomain.Coordinate) bool {
		name, named := topology.CoordinateName(coordinate)
		if !named || !emitter.PutAt(name, SourceRouteFact{Tag: tag, Coordinate: coordinate}) {
			admitted = false
			return false
		}
		return true
	})
	if !observed || !admitted {
		return outcome.Refused
	}
	if emitter.Len() == 0 {
		return outcome.NoCandidate
	}
	return outcome.Produced
}

// RawGetPackRoutesOperation is domain/heap/index's own payload enumeration for
// a read: the open payload tails one selected route carries under every key
// selector its candidate resolves to.
type RawGetPackRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewRawGetPackRoutesOperation adopts one sealed heap topology.
func NewRawGetPackRoutesOperation(topology *indexdomain.Topology) (RawGetPackRoutesOperation, bool) {
	if topology == nil {
		return RawGetPackRoutesOperation{}, false
	}
	return RawGetPackRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation RawGetPackRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate expands one selected route into the payload tails it carries.
func (operation RawGetPackRoutesOperation) Evaluate(argument RawGetPackRoutesArgument, emitter *relbindgen.Emitter[PackRouteFact]) outcome.Code {
	return packRoutes(operation.topology, argument.Route, argument.Fact, argument.Candidate, argument.Key, emitter)
}

// RawSetPackRoutesOperation is the same enumeration for a write.
type RawSetPackRoutesOperation struct {
	topology *indexdomain.Topology
}

// NewRawSetPackRoutesOperation adopts one sealed heap topology.
func NewRawSetPackRoutesOperation(topology *indexdomain.Topology) (RawSetPackRoutesOperation, bool) {
	if topology == nil {
		return RawSetPackRoutesOperation{}, false
	}
	return RawSetPackRoutesOperation{topology: topology}, true
}

// Available reports whether the operation carries a sealed topology.
func (operation RawSetPackRoutesOperation) Available() bool { return operation.topology != nil }

// Evaluate expands one selected route into the payload tails it carries.
func (operation RawSetPackRoutesOperation) Evaluate(argument RawSetPackRoutesArgument, emitter *relbindgen.Emitter[PackRouteFact]) outcome.Code {
	return packRoutes(operation.topology, argument.Route, argument.Fact, argument.Candidate, argument.Key, emitter)
}

// packRoutes is the one statement of the payload enumeration both directions
// perform.
//
// The key selection is delivered as the span the plan's key read observed, so
// its length is the count the owner's enumeration is stated against and each
// delivered row carries its own disposition. A statically keyed candidate
// reads no key row and enumerates the selector its own slot names; a
// dynamically keyed one reads exactly one, and the selectors are the ones that
// row's value projects. Each published row is named by the identity the sealed
// pack schema issued for the root it projects.
func packRoutes(
	topology *indexdomain.Topology,
	route HeapRouteFact,
	fact heapdomain.Value,
	candidate indexdomain.Index,
	key relbindgen.Span[valuedomain.Value],
	emitter *relbindgen.Emitter[PackRouteFact],
) outcome.Code {
	selected := indexdomain.NewMissingSelected[valuedomain.Value]()
	if key.Len() != 0 {
		value, present, ok := key.At(0)
		if !ok {
			return outcome.Refused
		}
		selected = indexdomain.NewSelected(value, present)
	}
	admitted := true
	observed := topology.VisitKeySelectors(candidate, selected, key.Len(), func(selector heapdomain.KeySelector) bool {
		return topology.VisitRoutePayloads(route.Tag, fact, selector, func(payload indexdomain.RawPayload) bool {
			if !payload.IsTail() {
				return true
			}
			root, rooted := payload.Root()
			if !rooted {
				admitted = false
				return false
			}
			name, named := topology.PackRootName(root)
			if !named || !emitter.PutAt(name, PackRouteFact{Root: root, Payload: payload.Tag()}) {
				admitted = false
				return false
			}
			return true
		})
	})
	if !observed || !admitted {
		return outcome.Refused
	}
	if emitter.Len() == 0 {
		return outcome.NoCandidate
	}
	return outcome.Produced
}

// HeapFormalFreezeOperation is domain/heap's own formal freeze: the fact one
// selected route's predecessor becomes when a call freezes its formals.
type HeapFormalFreezeOperation struct{}

// Available reports whether the operation carries its owner mathematics. The
// freeze is a package function over values that carry their own owner, so
// there is no derived state to hold and nothing to be unavailable.
func (HeapFormalFreezeOperation) Available() bool { return true }

// Evaluate answers one formal freeze at the route its tag names.
func (HeapFormalFreezeOperation) Evaluate(argument HeapFormalFreezeArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	fact, reduction := heapdomain.FormalFreezeFact(argument.RouteTag, argument.Predecessor)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// HeapPublicationFreezeOperation is domain/heap's own publication freeze.
type HeapPublicationFreezeOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (HeapPublicationFreezeOperation) Available() bool { return true }

// Evaluate answers one publication freeze at the route its tag names.
func (HeapPublicationFreezeOperation) Evaluate(argument HeapPublicationFreezeArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	fact, reduction := heapdomain.PublicationFreezeFact(argument.RouteTag, argument.Predecessor)
	return relbindgen.Reduce(emitter, fact, reduction)
}

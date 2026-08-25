package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
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
func (operation HeapReceiverRoutesOperation) Evaluate(argument HeapReceiverRoutesArgument, emitter *relbindgen.Emitter[RouteFact]) outcome.Code {
	admitted := true
	observed := operation.topology.VisitReceiver(argument.Receiver, nil, func(route indexdomain.Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			return true
		}
		content, named := key.ContentID()
		if !named || !emitter.PutAt(content, RouteFact{Kind: route.Kind(), Role: role}) {
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

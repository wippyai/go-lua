package relation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The raw-set commit's decoder.
//
// A write is publication to an existing authenticated row: the owner ascends
// one selected route's cell at a time, so the binding answers one row per
// route the delivery carries and names each by the heap key that route is
// rooted at. The correspondence it needs is the same one the read needs, in
// the other direction: a delivered heap row is addressed by its key's own
// identity, and the route tag that key answers under is the one the owner
// issues for it.

// rawSetLookups is one worker's resolution of a raw-set frame.
type rawSetLookups struct {
	topology *indexdomain.Topology

	values  relbindgen.Span[valuedomain.Value]
	packs   relbindgen.Span[packdomain.Value]
	sources relbindgen.Span[SourceRouteFact]

	// routes is the correspondence the owner's route enumeration issues: each
	// rooted route's own row identity and the tag it answers under.
	routes map[identity.ContentID]heapdomain.RawRouteTag

	pack   func(heapdomain.RawPayloadTag) indexdomain.Selected[packdomain.Value]
	source func(indexdomain.RawSourceTag) indexdomain.Selected[valuedomain.Value]
}

func newRawSetLookups(topology *indexdomain.Topology) (*rawSetLookups, bool) {
	if topology == nil {
		return nil, false
	}
	lookups := &rawSetLookups{topology: topology, routes: map[identity.ContentID]heapdomain.RawRouteTag{}}
	lookups.pack = func(tag heapdomain.RawPayloadTag) indexdomain.Selected[packdomain.Value] {
		payload, held := lookups.topology.RawPayloadAt(tag)
		if !held {
			return indexdomain.NewRefusedSelected[packdomain.Value]()
		}
		root, rooted := payload.Root()
		if !rooted {
			return indexdomain.NewRefusedSelected[packdomain.Value]()
		}
		row, named := lookups.topology.PackRootName(root)
		if !named {
			return indexdomain.NewRefusedSelected[packdomain.Value]()
		}
		return selectAt(lookups.packs, row)
	}
	lookups.source = func(tag indexdomain.RawSourceTag) indexdomain.Selected[valuedomain.Value] {
		coordinate, held := lookups.topology.RawSourceCoordinate(tag)
		if !held {
			return indexdomain.NewRefusedSelected[valuedomain.Value]()
		}
		row, named := lookups.topology.CoordinateName(coordinate)
		if !named {
			return indexdomain.NewRefusedSelected[valuedomain.Value]()
		}
		return selectAt(lookups.values, row)
	}
	return lookups, true
}

// open assigns one invocation's delivery and rebuilds the correspondence the
// owner's route enumeration issues for this receiver.
func (lookups *rawSetLookups) open(argument RawSetCommitArgument, receiver valuedomain.Value) (indexdomain.RawSetFrame, bool) {
	lookups.values, lookups.packs, lookups.sources = argument.Values, argument.Packs, argument.Sources

	clear(lookups.routes)
	enumerated := lookups.topology.VisitReceiver(receiver, nil, func(route indexdomain.Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			return true
		}
		tag, tagged := lookups.topology.RawRouteTag(key, role)
		row, named := key.ContentID()
		if !tagged || !named {
			return false
		}
		lookups.routes[row] = tag
		return true
	})
	if !enumerated {
		return indexdomain.RawSetFrame{}, false
	}

	frame := indexdomain.RawSetFrame{
		KeyCount:    0,
		HeapCount:   argument.Heaps.Len(),
		PackCount:   argument.Packs.Len(),
		Pack:        lookups.pack,
		SourceCount: argument.Sources.Len(),
		Source:      lookups.source,
	}
	if coordinate, dynamic := argument.Candidate.DynamicKey(); dynamic {
		row, named := lookups.topology.CoordinateName(coordinate)
		if !named {
			return indexdomain.RawSetFrame{}, false
		}
		frame.Key = selectAt(lookups.values, row)
		frame.KeyCount = 1
	}
	return frame, true
}

// RawSetCommitOperation is domain/heap/index's own raw-set commit.
type RawSetCommitOperation struct {
	topology *indexdomain.Topology
	lookups  *rawSetLookups
}

// NewRawSetCommitOperation adopts one sealed heap topology.
func NewRawSetCommitOperation(topology *indexdomain.Topology) (RawSetCommitOperation, bool) {
	lookups, ok := newRawSetLookups(topology)
	if !ok {
		return RawSetCommitOperation{}, false
	}
	return RawSetCommitOperation{topology: topology, lookups: lookups}, true
}

// NewOperation gives one solve-local worker its own resolution storage.
func (operation RawSetCommitOperation) NewOperation() relbindgen.Operation[RawSetCommitArgument, heapdomain.Value] {
	lookups, ok := newRawSetLookups(operation.topology)
	if !ok {
		return nil
	}
	local := operation
	local.lookups = lookups
	return local
}

// Available reports whether the operation carries a sealed topology and its
// resolution storage.
func (operation RawSetCommitOperation) Available() bool {
	return operation.topology != nil && operation.lookups != nil
}

// Evaluate ascends every route the write selected, one published row each at
// the heap key that route is rooted at.
func (operation RawSetCommitOperation) Evaluate(argument RawSetCommitArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	coordinate, addressed := argument.Candidate.Receiver()
	if !addressed {
		return outcome.Refused
	}
	row, named := operation.topology.CoordinateName(coordinate)
	if !named {
		return outcome.Refused
	}
	selected := selectAt(argument.Values, row)
	if !selected.Valid() || !selected.Found() {
		return outcome.Refused
	}
	if !selected.Present() {
		return outcome.NoCandidate
	}
	frame, opened := operation.lookups.open(argument, selected.Value())
	if !opened {
		return outcome.Refused
	}

	for index := 0; index < argument.Heaps.Len(); index++ {
		destination, ok := argument.Heaps.RowKeyAt(index)
		if !ok {
			return outcome.Refused
		}
		tag, routed := operation.lookups.routes[destination]
		if !routed {
			return outcome.Refused
		}
		fact, present, available := argument.Heaps.At(index)
		if !available {
			return outcome.Refused
		}
		if !present {
			continue
		}
		ascended, committed := operation.topology.RawSetMutateRoute(argument.Candidate, tag, fact, frame)
		if !committed {
			return outcome.Refused
		}
		if !emitter.PutAt(destination, ascended) {
			return outcome.Refused
		}
	}
	if emitter.Len() == 0 {
		return outcome.NoCandidate
	}
	return outcome.Produced
}

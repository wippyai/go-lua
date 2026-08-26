package relation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The raw-get reduction's decoder.
//
// The owner's reduction re-enumerates the receiver's calls and rooted routes
// and looks each one's fact up by the owner tag that enumeration issued. The
// relational frame delivers those facts as spans, in the mounted order of
// their declared keys, addressed by the identity each row's own owner issued.
// So the decoder's whole job is one correspondence: from the owner tag a
// lookup is keyed by, to the identity the row carrying that fact is addressed
// by. Every step of it is the owner's own resolution - a source tag names a
// coordinate, a coordinate is named by the value schema, a route is named by
// its heap key, a payload names a pack root - and nothing here numbers
// anything.
//
// The lookups are built once and read the spans through a pointer, so an
// invocation assigns its delivery and allocates nothing.

// rawGetLookups is one worker's resolution of a raw-get frame.
type rawGetLookups struct {
	topology *indexdomain.Topology
	scratch  *indexdomain.RawGetScratch

	values  relbindgen.Span[valuedomain.Value]
	calls   relbindgen.Span[calldomain.Value]
	heaps   relbindgen.Span[heapdomain.Value]
	sources relbindgen.Span[SourceRouteFact]
	packs   relbindgen.Span[packdomain.Value]

	// demand is the correspondence the owner's own call enumeration issues:
	// each demand tag and the call whose identity addresses its fact.
	demand map[uint64]identity.ContentID

	call   func(uint64) indexdomain.Selected[calldomain.Value]
	heap   func(heapdomain.RawRouteTag, heapdomain.Key) indexdomain.Selected[heapdomain.Value]
	pack   func(heapdomain.RawPayloadTag) indexdomain.Selected[packdomain.Value]
	source func(indexdomain.RawSourceTag) indexdomain.Selected[valuedomain.Value]
}

func newRawGetLookups(topology *indexdomain.Topology) (*rawGetLookups, bool) {
	scratch, ok := indexdomain.NewRawGetScratch(topology)
	if !ok {
		return nil, false
	}
	lookups := &rawGetLookups{topology: topology, scratch: scratch, demand: map[uint64]identity.ContentID{}}
	lookups.call = func(tag uint64) indexdomain.Selected[calldomain.Value] {
		row, named := lookups.demand[tag]
		if !named {
			return indexdomain.NewRefusedSelected[calldomain.Value]()
		}
		return selectAt(lookups.calls, row)
	}
	lookups.heap = func(_ heapdomain.RawRouteTag, key heapdomain.Key) indexdomain.Selected[heapdomain.Value] {
		row, named := key.ContentID()
		if !named {
			return indexdomain.NewRefusedSelected[heapdomain.Value]()
		}
		return selectAt(lookups.heaps, row)
	}
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
		return lookups.coordinate(coordinate)
	}
	return lookups, true
}

// coordinate answers the value delivered at one coordinate's own row.
func (lookups *rawGetLookups) coordinate(coordinate valuedomain.Coordinate) indexdomain.Selected[valuedomain.Value] {
	row, named := lookups.topology.CoordinateName(coordinate)
	if !named {
		return indexdomain.NewRefusedSelected[valuedomain.Value]()
	}
	return selectAt(lookups.values, row)
}

// selectAt answers the row of a delivered span addressed by one owner-issued
// identity. A span the owner never selected holds no such row, which is a
// missing selection and not a refused one.
func selectAt[T any](span relbindgen.Span[T], row identity.ContentID) indexdomain.Selected[T] {
	for index := 0; index < span.Len(); index++ {
		delivered, ok := span.RowKeyAt(index)
		if !ok {
			return indexdomain.NewRefusedSelected[T]()
		}
		if delivered != row {
			continue
		}
		value, present, available := span.At(index)
		if !available {
			return indexdomain.NewRefusedSelected[T]()
		}
		return indexdomain.NewSelected(value, present)
	}
	return indexdomain.NewMissingSelected[T]()
}

// open assigns one invocation's delivery and rebuilds the correspondence the
// owner's call enumeration issues for this receiver.
func (lookups *rawGetLookups) open(argument RawGetResultArgument, receiver valuedomain.Value) (indexdomain.RawGetFrame, bool) {
	lookups.values, lookups.calls = argument.Values, argument.Calls
	lookups.heaps, lookups.sources, lookups.packs = argument.Heaps, argument.Sources, argument.Packs

	clear(lookups.demand)
	enumerated := lookups.topology.VisitReceiverCallDemand(receiver, func(key calldomain.Key, tag uint64) bool {
		row, named := key.ContentID()
		if !named {
			return false
		}
		lookups.demand[tag] = row
		return true
	})
	if !enumerated {
		return indexdomain.RawGetFrame{}, false
	}

	frame := indexdomain.RawGetFrame{
		Scratch:     lookups.scratch,
		KeyCount:    0,
		CallCount:   argument.Calls.Len(),
		Call:        lookups.call,
		HeapCount:   argument.Heaps.Len(),
		Heap:        lookups.heap,
		PackCount:   argument.Packs.Len(),
		Pack:        lookups.pack,
		SourceCount: argument.Sources.Len(),
		Source:      lookups.source,
	}
	if coordinate, dynamic := argument.Candidate.DynamicKey(); dynamic {
		frame.Key = lookups.coordinate(coordinate)
		frame.KeyCount = 1
	}
	return frame, true
}

// RawGetResultOperation is domain/heap/index's own raw-get reduction.
type RawGetResultOperation struct {
	topology *indexdomain.Topology
	lookups  *rawGetLookups
}

// NewRawGetResultOperation adopts one sealed heap topology.
func NewRawGetResultOperation(topology *indexdomain.Topology) (RawGetResultOperation, bool) {
	lookups, ok := newRawGetLookups(topology)
	if !ok {
		return RawGetResultOperation{}, false
	}
	return RawGetResultOperation{topology: topology, lookups: lookups}, true
}

// NewOperation gives one solve-local worker its own resolution storage.
func (operation RawGetResultOperation) NewOperation() relbindgen.Operation[RawGetResultArgument, valuedomain.Value] {
	lookups, ok := newRawGetLookups(operation.topology)
	if !ok {
		return nil
	}
	local := operation
	local.lookups = lookups
	return local
}

// Available reports whether the operation carries a sealed topology and its
// resolution storage.
func (operation RawGetResultOperation) Available() bool {
	return operation.topology != nil && operation.lookups != nil
}

// Evaluate answers one indexed read.
func (operation RawGetResultOperation) Evaluate(argument RawGetResultArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	coordinate, addressed := argument.Candidate.Receiver()
	if !addressed {
		return outcome.Refused
	}
	selected := operation.lookups.coordinate(coordinate)
	if !selected.Valid() || !selected.Found() {
		return outcome.Refused
	}
	if !selected.Present() {
		return outcome.NoCandidate
	}
	receiver := selected.Value()
	frame, opened := operation.lookups.open(argument, receiver)
	if !opened {
		return outcome.Refused
	}
	fact, contributed, valid := operation.topology.RawGetReduce(argument.Candidate, receiver, frame)
	if !valid {
		return outcome.Refused
	}
	if !contributed {
		return outcome.NoCandidate
	}
	if !emitter.Put(fact) {
		return outcome.Refused
	}
	return outcome.Produced
}

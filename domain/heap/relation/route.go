package relation

import (
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The typed payloads the indexed raw-access route relations carry.
//
// A dependent read of the raw-access chain reads a coordinate that does not
// exist until an earlier read's cell is known, so it is not an equijoin over
// authored columns. Each one is an expansion that publishes a route relation,
// and the read is then an ordinary equijoin onto it. These are the facts those
// relations hold: one per route kind, carrying what the read after it observes
// through and nothing else.

// KeyRouteFact is the value coordinate one dynamic key selects.
type KeyRouteFact struct {
	Coordinate valuedomain.Coordinate
}

// CallRouteFact is one call the receiver demands, and the demand tag its own
// topology issued for it.
type CallRouteFact struct {
	Key calldomain.Key
	Tag uint64
}

// HeapRouteFact is one rooted route the receiver observes: the heap root, the
// role it is materialized under, and the raw route tag the heap schema issued
// for the pair.
type HeapRouteFact struct {
	Kind indexdomain.RouteKind
	Key  heapdomain.Key
	Role materialization.Role
	Tag  heapdomain.RawRouteTag
}

// PackRouteFact is one open payload tail a selected route carries, and the
// pack root it projects.
type PackRouteFact struct {
	Root    packdomain.Root
	Payload heapdomain.RawPayloadTag
}

// SourceRouteFact is one semantic source a payload declares, and the value
// coordinate that source names.
type SourceRouteFact struct {
	Tag        indexdomain.RawSourceTag
	Coordinate valuedomain.Coordinate
}

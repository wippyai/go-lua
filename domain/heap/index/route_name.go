package index

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

// RawRouteTag answers the raw route tag the sealed heap schema issued for one
// rooted route. A route expansion carries it so the read after it addresses
// the same route its own owner named, without reaching the schema itself.
func (topology *Topology) RawRouteTag(key heapdomain.Key, role materialization.Role) (heapdomain.RawRouteTag, bool) {
	if topology == nil || !topology.valid() {
		return 0, false
	}
	return topology.heap.RouteTag(key, role)
}

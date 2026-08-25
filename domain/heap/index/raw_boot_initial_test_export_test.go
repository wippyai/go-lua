package index

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// VisitSealedBootInitials walks the sealed boot-initial receipt table exactly
// as the hot raw read indexes it. It exists so the external law can compare
// the complete baked table against Heap's and Value's own owner projections.
func VisitSealedBootInitials(topology *Topology, visit func(heapdomain.RawRouteTag, heapdomain.RawPayloadTag, valuedomain.Value) bool) bool {
	if topology == nil || !topology.valid() || visit == nil {
		return false
	}
	for index, value := range topology.catalog.bootInitials {
		if !visit(index.route, index.payload, value) {
			return false
		}
	}
	return true
}

// SealedBootInitialWithoutValueSchema resolves one receipt through the
// production lookup with the rule's live Value schema detached. A rule that
// manufactured the fact by reopening Value mid-solve cannot answer here.
func SealedBootInitialWithoutValueSchema(topology *Topology, route heapdomain.RawRouteTag, payload heapdomain.RawPayloadTag) (valuedomain.Value, bool) {
	if topology == nil {
		return valuedomain.Value{}, false
	}
	rule := &RawGetRule{runtime: &rawGetRuntime{topology: topology, heap: topology.heap, calls: topology.calls}}
	return rule.bootInitialAt(route, payload)
}

// ApplyBootInitialPresent runs the production RawGet boot branch over one
// real sealed RawAccess/Present pair and returns its joined transfer result.
func ApplyBootInitialPresent(topology *Topology, route heapdomain.RawRouteTag, raw heapdomain.RawAccess, present heapdomain.Present) (valuedomain.Value, bool, bool) {
	if topology == nil || !topology.valid() {
		return valuedomain.Value{}, false, false
	}
	result, any := topology.values.Bottom(), false
	ok := topology.applyPresent(route, raw, present, RawGetFrame{}, &rawGetCensus{}, &result, &any)
	return result, any, ok
}

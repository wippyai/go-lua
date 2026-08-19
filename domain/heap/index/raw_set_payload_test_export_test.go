package index

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func rawSetRuleForTest(topology *Topology) *RawSetRule {
	if topology == nil || !topology.valid() {
		return nil
	}
	return &RawSetRule{runtime: &rawSetRuntime{values: topology.values, heap: topology.heap}, topology: topology}
}

// ApplyRawSetTopPayload runs the production unconstrained right-hand-side
// branch over one real sealed RawAccess and returns its joined write result.
func ApplyRawSetTopPayload(topology *Topology, raw heapdomain.RawAccess, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment) (heapdomain.Value, bool) {
	rule := rawSetRuleForTest(topology)
	if rule == nil {
		return heapdomain.Value{}, false
	}
	result := topology.heap.Bottom()
	var frozen, changed bool
	ok := rule.applyTop(topology.heap, raw, Access{}, slot, payload, keyChild, &result, &frozen, &changed)
	return result, ok && changed && !frozen
}

// ApplyRawSetSourcePayload runs the production enumerated right-hand-side
// branch over the same RawAccess, so a law can compare the two on equal terms.
func ApplyRawSetSourcePayload(topology *Topology, raw heapdomain.RawAccess, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment, source valuedomain.Value) (heapdomain.Value, bool) {
	rule := rawSetRuleForTest(topology)
	if rule == nil {
		return heapdomain.Value{}, false
	}
	result := topology.heap.Bottom()
	var frozen, changed, preserved bool
	ok := rule.applySourceValue(topology.heap, raw, source, Access{}, slot, payload, keyChild, &result, &frozen, &changed, &preserved)
	return result, ok && changed && !frozen
}

// ReadStoredPayload joins the production RawGet containment reduction over
// every Present a stored fact carries at selector, against the same source
// Value the write consumed. It is the read half of the write/read round trip.
func ReadStoredPayload(topology *Topology, key heapdomain.Key, fact heapdomain.Value, role materialization.Role, selector heapdomain.KeySelector, source valuedomain.Value) (valuedomain.Value, bool) {
	if topology == nil || !topology.valid() {
		return valuedomain.Value{}, false
	}
	rule := &RawGetRule{runtime: &rawGetRuntime{topology: topology, values: topology.values, heap: topology.heap, calls: topology.calls}}
	result, any := topology.values.Bottom(), false
	ok := topology.heap.VisitRawAccess(key, fact, role, selector, func(raw heapdomain.RawAccess) bool {
		if raw.IsTop() {
			return false
		}
		cell, cellOK := raw.Cell()
		if !cellOK {
			return false
		}
		for index := 0; index < cell.PresentCount(); index++ {
			present, presentOK := cell.PresentAt(index)
			if !presentOK {
				return false
			}
			containment, _, containmentOK := present.Containment()
			if !containmentOK || !rule.reduceAndJoin(containment, source, &result, &any) {
				return false
			}
		}
		return true
	})
	return result, ok
}

package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

// FreezeFold is the formal-freeze judgment: given the exact Recent route set a
// call justifies, it answers what this axis publishes at one of those routes.
//
// It is a total function of owner-issued data and concludes one of the sealed
// five dispositions, so the whole judgment is reachable without an engine
// frame, a selection object, or a rule callback. The routed runtime calls it
// once per selected route, and once over an empty plan, which is the one place
// a fold can settle the explicitly empty selection.
//
//   - An empty plan is NoSelection. The call is a real occurrence whose freeze
//     has no exact Recent root to publish at - unresolved, open, opaque or
//     ambiguous evidence all land here - which is a different answer from
//     refusing to look and from the call not being a candidate at all.
//   - A selected route with no predecessor fact publishes Bottom. The routed
//     output must still settle one exact Heap target, and the empty normal
//     image is that target rather than a fabricated frozen object.
//   - A route whose predecessor freezes publishes the normal successor. A
//     transition the owner cannot issue is Refuse: this fold never widens a
//     freeze it could not prove.
func FreezeFold(
	schema heapdomain.Schema,
	plan routePlan,
	tag heapdomain.RawRouteTag,
	predecessor heapdomain.Value,
	present bool,
) (heapdomain.Value, structure.ReductionOutcome) {
	if !schema.Valid() {
		return heapdomain.Value{}, structure.Refuse
	}
	if plan.Count() == 0 {
		return schema.Bottom(), structure.NoSelection
	}
	route, routeOK := routeForTag(plan, tag)
	if !routeOK {
		return heapdomain.Value{}, structure.Refuse
	}
	if !present {
		return schema.Bottom(), structure.Concrete
	}
	reference, referenceOK := schema.Reference(route.Key, materialization.Recent)
	if !referenceOK {
		return heapdomain.Value{}, structure.Refuse
	}
	branches, freezeOK := schema.ShallowFreeze(predecessor, reference)
	if !freezeOK {
		return heapdomain.Value{}, structure.Refuse
	}
	next, normalOK := branches.Normal(route.Key)
	if !normalOK {
		return schema.Bottom(), structure.Concrete
	}
	return next, structure.Concrete
}

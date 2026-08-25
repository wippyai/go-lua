package heap

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/materialization"
)

// FormalFreezeFact is the formal-freeze judgment: at one route a mounted call
// justifies, it answers what Heap publishes for that route's allocation root.
//
// The judgment is Heap's, so it lives beside Heap's other allocation folds and
// reaches no Call, Pack, Target or Value authority. Which routes a call
// justifies is a separate question, answered by the route relation the freeze
// rule declares; this fold is only ever handed a member of that relation and
// therefore states no plan of its own.
//
// The route arrives as the owner-issued TAG its cells were paired by, because
// that is what a routed form hands a member reducer whose declaration names
// the tag carrier. The coordinate is recovered by admitting the tag back
// through the schema that issued it, which the predecessor carries: a tag this
// schema did not issue is refused rather than decoded into a neighbouring root.
func FormalFreezeFact(routeTag uint64, predecessor Value) (Value, structure.ReductionOutcome) {
	if routeTag == 0 {
		return Value{}, structure.NoSelection
	}
	if predecessor.owner == nil {
		return Value{}, structure.Refuse
	}
	schema := Schema{owner: predecessor.owner}
	key, role, routeOK := schema.RouteForTag(RawRouteTag(routeTag))
	if !routeOK || role != materialization.Recent {
		return Value{}, structure.Refuse
	}
	reference, referenceOK := schema.Reference(key, materialization.Recent)
	if !referenceOK {
		return Value{}, structure.Refuse
	}
	branches, freezeOK := schema.ShallowFreeze(predecessor, reference)
	if !freezeOK {
		return Value{}, structure.Refuse
	}
	next, normalOK := branches.Normal(key)
	if !normalOK {
		return schema.Bottom(), structure.Concrete
	}
	return next, structure.Concrete
}

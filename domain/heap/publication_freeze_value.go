package heap

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/materialization"
)

// PublicationFreezeFact is the publication-freeze judgment: at one route an
// authored FreezeSeal publication justifies, it answers what Heap publishes
// for that route's allocation root.
//
// It is the same freeze FormalFreezeFact states, reached from the other
// authority: a formal freeze is justified by a call's declared parameters, a
// publication freeze by an effect the program authored. Which routes either
// justifies is a separate question, answered by the route relation each rule
// declares; this fold is only ever handed a member of that relation.
//
// The route arrives as the owner-issued TAG its cells were paired by, because
// that is what a routed form hands a member reducer whose declaration names
// the tag carrier. The coordinate is recovered by admitting the tag back
// through the schema that issued it, which the predecessor carries: a tag this
// schema did not issue is refused rather than decoded into a neighbouring root.
//
//   - A zero tag is NoSelection. It is the one invocation a routed form makes
//     over an empty route set, and the answer says the publication is a real
//     occurrence whose freeze has no exact Recent root to publish at -
//     unresolved, open, opaque and ambiguous evidence all land here, which is a
//     different answer from refusing to look and from the call not being a
//     candidate at all.
//   - A route whose predecessor freezes publishes the normal successor. A
//     transition the owner cannot issue is Refuse: this fold never widens a
//     freeze it could not prove.
//   - A predecessor that issues no normal branch publishes the empty normal
//     image, which is Bottom. Bottom is also this Factor's declared default, so
//     an unwritten route coordinate reaches the fold as Bottom and takes the
//     same answer: absence is not a distinction this judgment draws.
func PublicationFreezeFact(routeTag uint64, predecessor Value) (Value, structure.ReductionOutcome) {
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

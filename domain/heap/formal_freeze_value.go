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
// The candidate is the route's allocation coordinate. It carries its own owner,
// so the fold recovers the schema it decides in from the candidate itself.
//
//   - The zero coordinate is NoSelection. It is the one invocation a route form
//     makes over an empty route set, and the answer says the call is a real
//     occurrence whose freeze has no exact Recent root to publish at -
//     unresolved, open, opaque and ambiguous evidence all land here, which is a
//     different answer from refusing to look and from the call not being a
//     candidate at all.
//   - A route whose predecessor freezes publishes the normal successor. A
//     transition the owner cannot issue is Refuse: this fold never widens a
//     freeze it could not prove.
//   - A predecessor that issues no normal branch publishes the empty normal
//     image. The routed output must still settle one exact Heap target, and
//     Bottom is that target rather than a fabricated frozen object. Bottom is
//     also this Factor's declared default, so an unwritten route coordinate
//     reaches the fold as Bottom and takes the same answer: absence is not a
//     distinction this judgment draws, and making the two disagree is a change
//     to the freeze judgment rather than to a read's sparse clause.
func FormalFreezeFact(key Key, predecessor Value) (Value, structure.ReductionOutcome) {
	if key == (Key{}) {
		return Value{}, structure.NoSelection
	}
	schema := Schema{owner: key.owner}
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

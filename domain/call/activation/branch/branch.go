// Package branch owns Call activation's structural judgment: which of a
// trigger's candidate branches one Call value actually names.
//
// The branch set is Call's global body table, because a Call value may flow to
// any admitted body. What decides a branch is the value's own known targets,
// so the walk is over those - a value naming two targets settles two branches
// and passes over the rest without reading anything about them.
package branch

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
)

// Selector is the sealed state this judgment is issued by: Call's own algebra,
// which owns the body table the branch ordinals address. A fold cannot take an
// axis schema as a parameter, so the state it rests on is named once and
// sealed here.
type Selector struct {
	calls *calldomain.Algebra
}

// Derive seals the selector against the one Call algebra whose body table the
// branch ordinals are positions in.
func Derive(calls *calldomain.Algebra) (Selector, bool) {
	if calls == nil || !calls.Valid() {
		return Selector{}, false
	}
	return Selector{calls: calls}, true
}

// Settle answers whether the trigger's value names the body at this branch
// ordinal.
//
// A value that is Top names every admitted body: an unconstrained callable may
// be any of them, and an activation that instantiated none of them would be
// claiming knowledge the value does not carry. A value that names a target
// with no body contributes no branch - it is a seed, not a call into an
// admitted body - and one whose target the algebra cannot place refuses,
// because a target this Call did not issue is not a target this rule may
// settle a branch on.
func (selector Selector) Settle(candidate calldomain.CallCoordinate, branch uint64, trigger calldomain.Value) structure.ReductionOutcome {
	if selector.calls == nil || !selector.calls.Valid() || !selector.calls.OwnsCallCoordinate(candidate) {
		return structure.Refuse
	}
	bodies := selector.calls.Bodies()
	if branch >= uint64(bodies.Count()) {
		return structure.Refuse
	}
	if trigger.IsTop() {
		return structure.Concrete
	}
	for index := 0; index < trigger.KnownTargetCount(); index++ {
		target, targetOK := trigger.KnownTargetAt(index)
		if !targetOK {
			return structure.Refuse
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			// A target with no body is an external seed. It instantiates no
			// admitted body and is not a defect.
			continue
		}
		ordinal, ordinalOK := bodies.Index(body)
		if !ordinalOK {
			return structure.Refuse
		}
		if uint64(ordinal) == branch {
			return structure.Concrete
		}
	}
	return structure.NoSelection
}

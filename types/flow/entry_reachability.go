package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
)

// LiftEntryReachability turns the solver's bottom seed into the reachable
// identity state for a function entry. The entry point has no predecessor, so
// axes that use Bottom as unreachable must be lifted before transfer seeds
// parameter facts.
func LiftEntryReachability(ps *PointState) bool {
	if ps == nil {
		return false
	}
	changed := false
	changed = EnsurePointNumericReachableState(ps) || changed
	if constraint.Domain.Equal(ps.Cond, constraint.Domain.Bottom()) {
		ps.Cond = constraint.Domain.Top()
		changed = true
	}
	if ps.Rel.IsBottom() {
		ps.Rel = PointRelationsDomain.Top()
		changed = true
	}
	if ps.ReturnRel.IsBottom() {
		ps.ReturnRel = ReturnRelationsDomain.Top()
		changed = true
	}
	changed = LiftCellEffectsEntry(ps) || changed
	if ps.ReceiverEffects.IsBottom() {
		ps.ReceiverEffects = ReceiverEffectsIdentity()
		changed = true
	}
	changed = LiftStaticMembersEntry(ps) || changed
	changed = LiftKeyPresenceEntry(ps) || changed
	changed = LiftValueOriginsEntry(ps) || changed
	changed = LiftPathAliasesEntry(ps) || changed
	changed = LiftIndexWritesEntry(ps) || changed
	return changed
}

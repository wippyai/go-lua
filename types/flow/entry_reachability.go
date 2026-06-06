package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
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
	if ps.Num == nil || ps.Num.IsUnsat() {
		ps.Num = numeric.NewState()
		changed = true
	}
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
	if ps.CellEffects.IsBottom() {
		ps.CellEffects = CaptureEffectsIdentity()
		changed = true
	}
	if ps.ReceiverEffects.IsBottom() {
		ps.ReceiverEffects = ReceiverEffectsIdentity()
		changed = true
	}
	if ps.StaticMembers.IsBottom() {
		ps.StaticMembers = StaticMemberFactsDomain.Top()
		changed = true
	}
	if ps.KeyPresence.IsBottom() {
		ps.KeyPresence = KeyPresenceFactsDomain.Top()
		changed = true
	}
	if ps.ValueOrigins.IsBottom() {
		ps.ValueOrigins = ValueOriginFactsDomain.Top()
		changed = true
	}
	if ps.PathAliases.IsBottom() {
		ps.PathAliases = PathAliasFactsDomain.Top()
		changed = true
	}
	if ps.IndexWrites.IsBottom() {
		ps.IndexWrites = IndexWriteAdmissionFactsDomain.Top()
		changed = true
	}
	return changed
}

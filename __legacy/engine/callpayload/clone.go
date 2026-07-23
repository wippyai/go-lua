package callpayload

import (
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// Clone returns a detached immutable-publication copy of o. Abstract product
// values and domain atoms are immutable; slices, maps, paths, and heap objects
// are copied so a caller cannot mutate retained publication storage.
func (o CallOutcome) Clone() CallOutcome {
	out := o
	out.Results = cloneCallOutcomeSlice(o.Results)
	out.NormalReturnFacts = o.NormalReturnFacts.Clone()
	out.ProtectedCallTypestate = o.ProtectedCallTypestate.Clone()
	if len(o.HeapTableObjects) != 0 {
		out.HeapTableObjects = make(map[identity.ID]heapidentity.TableObject, len(o.HeapTableObjects))
		for id, object := range o.HeapTableObjects {
			out.HeapTableObjects[id] = heapidentity.CloneObject(object)
		}
	}
	if len(o.Placements) != 0 {
		out.Placements = make(map[identity.ID]placement.Value, len(o.Placements))
		for id, value := range o.Placements {
			out.Placements[id] = value
		}
	}
	out.ParamObligations = cloneCallOutcomeSlice(o.ParamObligations)
	out.PathObligations = cloneCallOutcomeSlice(o.PathObligations)
	out.TypestateRequirements = cloneCallOutcomeSlice(o.TypestateRequirements)
	out.ParamPathRefinements = cloneCallOutcomeSlice(o.ParamPathRefinements)
	out.ParamPathWrites = cloneCallOutcomeSlice(o.ParamPathWrites)
	out.ParamLengthFloors = cloneCallOutcomeSlice(o.ParamLengthFloors)
	out.ParamPathInvalidations = cloneCallOutcomeSlice(o.ParamPathInvalidations)
	out.ParamConditions = cloneCallOutcomeSlice(o.ParamConditions)
	out.ParamPathRelations = cloneCallOutcomeSlice(o.ParamPathRelations)
	out.ReturnConditionRefinements = cloneCallOutcomeSlice(o.ReturnConditionRefinements)
	out.ReturnConditionSlots = cloneCallOutcomeSlice(o.ReturnConditionSlots)
	out.ReturnPresenceRelations = cloneCallOutcomeSlice(o.ReturnPresenceRelations)
	out.ParamExposures = cloneCallOutcomeSlice(o.ParamExposures)
	for i := range out.PathObligations {
		out.PathObligations[i].Path = out.PathObligations[i].Path.Clone()
	}
	for i := range out.TypestateRequirements {
		out.TypestateRequirements[i].Target = out.TypestateRequirements[i].Target.Clone()
	}
	for i := range out.ParamPathRefinements {
		out.ParamPathRefinements[i].Path = out.ParamPathRefinements[i].Path.Clone()
	}
	for i := range out.ParamPathWrites {
		out.ParamPathWrites[i].Path = out.ParamPathWrites[i].Path.Clone()
	}
	for i := range out.ParamLengthFloors {
		out.ParamLengthFloors[i].Path = out.ParamLengthFloors[i].Path.Clone()
	}
	for i := range out.ParamPathInvalidations {
		out.ParamPathInvalidations[i].Path = out.ParamPathInvalidations[i].Path.Clone()
	}
	for i := range out.ParamPathRelations {
		out.ParamPathRelations[i].Left = out.ParamPathRelations[i].Left.Clone()
		out.ParamPathRelations[i].Right = out.ParamPathRelations[i].Right.Clone()
	}
	for i := range out.ReturnConditionRefinements {
		out.ReturnConditionRefinements[i].Target = out.ReturnConditionRefinements[i].Target.Clone()
	}
	for i := range out.ParamExposures {
		out.ParamExposures[i].Source = out.ParamExposures[i].Source.Clone()
	}
	return out
}

func cloneCallOutcomeSlice[T any](in []T) []T { return append([]T(nil), in...) }

package body

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type TypestateRequirementStatus uint8

const (
	TypestateRequirementUnproven TypestateRequirementStatus = iota + 1
	TypestateRequirementRefuted
)

// TypestateRequirementProof records a declared call-entry lifecycle
// precondition that was either proven false or could not be established.
type TypestateRequirementProof struct {
	Point    cfg.Point
	Resource string
	Protocol string
	Expected string
	Found    string
	Target   string
	Status   TypestateRequirementStatus
	Span     SourceSpan
}

func (r *Result) TypestateRequirementProofs() []TypestateRequirementProof {
	if r == nil || r.Graph() == nil {
		return nil
	}
	var out []TypestateRequirementProof
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		outcome, ok := r.CallOutcomeAt(point)
		if !ok || len(outcome.TypestateRequirements) == 0 {
			continue
		}
		in, stateOK := r.StateAt(point)
		bindings := r.callGuardCallBindingsAt(point)
		for _, requirement := range outcome.TypestateRequirements {
			target, targetOK := requirement.Target.Substitute(bindings)
			proof := TypestateRequirementProof{
				Point:    point,
				Protocol: string(requirement.Protocol),
				Expected: string(requirement.State),
				Status:   TypestateRequirementUnproven,
				Span:     r.callSpanAt(point),
			}
			if targetOK && !target.IsEmpty() {
				proof.Target = r.DisplayPath(target)
			}
			if !targetOK || !stateOK {
				out = append(out, proof)
				continue
			}
			resource, resourceOK := r.TypestateResourceAtCallEntry(point, target, requirement.Protocol)
			if !resourceOK {
				out = append(out, proof)
				continue
			}
			proof.Resource = resource.ID.String()
			slot, slotOK := in.TypestateSlot(resource)
			if !slotOK || slot.Current == "" || slot.Locality == typestate.LocalityBottom || slot.Locality == typestate.LocalityUnknown || slot.Locality == typestate.LocalityEscaped {
				out = append(out, proof)
				continue
			}
			proof.Found = string(slot.Current)
			if slot.Current != requirement.State {
				proof.Status = TypestateRequirementRefuted
				out = append(out, proof)
			}
		}
	}
	return out
}

package calloutcome

import (
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// WithSupplemental composes two call outcome providers. Result slots and
// post-return state facts are accumulated only until a provider declares
// post-return authority for the call. Pre-call diagnostic obligations are
// accumulated.
func WithSupplemental(primary, supplemental callpayload.CallOutcomeProvider) callpayload.CallOutcomeProvider {
	if primary == nil {
		return supplemental
	}
	if supplemental == nil {
		return primary
	}
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		out := primary(ctx, site, in, read)
		second := supplemental(ctx, site, in, read)
		out = withSupplementalResultSlots(ctx.Registry, out, second.Results)
		return withSupplementalFacts(ctx.Registry, out, second)
	}
}

func withSupplementalResultSlots(reg *axis.Registry, out callpayload.CallOutcome, results []callpayload.CallResult) callpayload.CallOutcome {
	if len(results) == 0 {
		return out
	}
	if out.PostReturnAuthority {
		return out
	}
	if len(out.Results) == 0 {
		out.Results = append(out.Results, results...)
		return out
	}
	position := make(map[int]int, len(out.Results))
	for i, result := range out.Results {
		position[result.Index] = i
	}
	for _, result := range results {
		pos, ok := position[result.Index]
		if !ok {
			position[result.Index] = len(out.Results)
			out.Results = append(out.Results, result)
			continue
		}
		if resultSlotLacksSpecificTypeEvidence(reg, out.Results[pos].Value) && !resultSlotLacksSpecificTypeEvidence(reg, result.Value) {
			out.Results[pos].Value = product.Meet(reg, out.Results[pos].Value, result.Value)
		}
	}
	return out
}

func withSupplementalFacts(reg *axis.Registry, out, second callpayload.CallOutcome) callpayload.CallOutcome {
	out.ParamObligations = append(out.ParamObligations, second.ParamObligations...)
	out.ParamExposures = append(out.ParamExposures, second.ParamExposures...)
	if out.PostReturnAuthority {
		return out
	}
	out.NormalReturnFacts = out.NormalReturnFacts.Append(second.NormalReturnFacts)
	out.HeapTableObjects = withSupplementalHeapTableObjects(reg, out.HeapTableObjects, second.HeapTableObjects)
	out.Placements = withSupplementalPlacements(out.Placements, second.Placements)
	out.ParamPathRefinements = append(out.ParamPathRefinements, second.ParamPathRefinements...)
	out.ParamLengthFloors = append(out.ParamLengthFloors, second.ParamLengthFloors...)
	out.ParamPathInvalidations = append(out.ParamPathInvalidations, second.ParamPathInvalidations...)
	out.ParamConditions = append(out.ParamConditions, second.ParamConditions...)
	out.ParamPathRelations = append(out.ParamPathRelations, second.ParamPathRelations...)
	out.ReturnConditionRefinements = append(out.ReturnConditionRefinements, second.ReturnConditionRefinements...)
	out.ReturnPresenceRelations = append(out.ReturnPresenceRelations, second.ReturnPresenceRelations...)
	out.PostReturnAuthority = second.PostReturnAuthority
	return out
}

func withSupplementalPlacements(
	left, right map[identity.ID]placement.Value,
) map[identity.ID]placement.Value {
	if len(right) == 0 {
		return clonePlacements(left)
	}
	if len(left) == 0 {
		return clonePlacements(right)
	}
	out := clonePlacements(left)
	for id, value := range right {
		if existing, ok := out[id]; ok {
			out[id] = placement.Join(existing, value)
			continue
		}
		out[id] = value
	}
	return out
}

func clonePlacements(in map[identity.ID]placement.Value) map[identity.ID]placement.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]placement.Value, len(in))
	for id, value := range in {
		out[id] = value
	}
	return out
}

func withSupplementalHeapTableObjects(
	reg *axis.Registry,
	left, right map[identity.ID]heapidentity.TableObject,
) map[identity.ID]heapidentity.TableObject {
	if len(right) == 0 {
		return heapidentity.CloneMap(left)
	}
	if len(left) == 0 {
		return heapidentity.CloneMap(right)
	}
	if reg == nil {
		out := heapidentity.CloneMap(left)
		if out == nil {
			out = make(map[identity.ID]heapidentity.TableObject, len(right))
		}
		for id, object := range right {
			if _, ok := out[id]; ok {
				continue
			}
			out[id] = object
		}
		return out
	}
	return heapidentity.CloneMap(heapidentity.MapDomain(reg).Join(left, right))
}

func resultSlotLacksSpecificTypeEvidence(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return true
	}
	return typ.IsAny(t) || typ.IsUnknown(t)
}

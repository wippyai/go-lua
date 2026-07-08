package calloutcome

import (
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	typerefinement "github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// WithSupplemental composes two call outcome providers. Result slots and
// post-return state facts are accumulated only until a provider declares
// post-return authority for the call. Pre-call diagnostic obligations are
// accumulated.
func WithSupplemental(primary, supplemental callpayload.CallOutcomeProvider) callpayload.CallOutcomeProvider {
	return ComposeSupplemental(primary, supplemental)
}

// ComposeSupplemental returns one canonical provider that evaluates every
// non-nil provider in order and merges their outcomes with the same authority
// law as WithSupplemental. It is the preferred assembly point for production
// call-boundary providers because it avoids nested wrapper chains.
func ComposeSupplemental(providers ...callpayload.CallOutcomeProvider) callpayload.CallOutcomeProvider {
	providers = compactProviders(providers)
	switch len(providers) {
	case 0:
		return nil
	case 1:
		return providers[0]
	}
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		out := providers[0](ctx, site, in, read)
		for _, provider := range providers[1:] {
			out = MergeSupplemental(ctx.Registry, out, provider(ctx, site, in, read))
		}
		return out
	}
}

func compactProviders(providers []callpayload.CallOutcomeProvider) []callpayload.CallOutcomeProvider {
	out := providers[:0]
	for _, provider := range providers {
		if provider != nil {
			out = append(out, provider)
		}
	}
	return out
}

// MergeSupplemental merges one already-computed supplemental outcome into an
// already-computed primary outcome using the same authority semantics as
// WithSupplemental. It is for providers that derive a local supplemental payload
// while adapting another source of call evidence.
func MergeSupplemental(reg *axis.Registry, primary, supplemental callpayload.CallOutcome) callpayload.CallOutcome {
	out := withSupplementalResultSlots(reg, primary, supplemental.Results)
	return withSupplementalFacts(reg, out, supplemental)
}

func withSupplementalResultSlots(reg *axis.Registry, out callpayload.CallOutcome, results []callpayload.CallResult) callpayload.CallOutcome {
	if len(results) == 0 {
		return out
	}
	if len(out.Results) == 0 {
		if out.PostReturnAuthority {
			return out
		}
		out.Results = append(out.Results, results...)
		return out
	}
	var position map[int]int
	if len(out.Results) > 4 && len(results) > 1 {
		position = make(map[int]int, len(out.Results))
		for i, result := range out.Results {
			position[result.Index] = i
		}
	}
	for _, result := range results {
		pos, ok := supplementalResultSlotPosition(out.Results, position, result.Index)
		if !ok {
			if out.PostReturnAuthority {
				continue
			}
			if position != nil {
				position[result.Index] = len(out.Results)
			}
			out.Results = append(out.Results, result)
			continue
		}
		if refined, ok := refinedResultSlotValue(reg, out.Results[pos].Value, result.Value); ok {
			out.Results[pos].Value = refined
		}
	}
	return out
}

func supplementalResultSlotPosition(results []callpayload.CallResult, position map[int]int, index int) (int, bool) {
	if position != nil {
		pos, ok := position[index]
		return pos, ok
	}
	for i, result := range results {
		if result.Index == index {
			return i, true
		}
	}
	return 0, false
}

func refinedResultSlotValue(reg *axis.Registry, current, supplemental product.Value) (product.Value, bool) {
	if resultSlotEvidencePromotesGradualToExplicit(reg, current, supplemental) {
		return supplemental, true
	}
	if reg == nil ||
		product.Equal(reg, current, supplemental) ||
		resultSlotLacksSpecificTypeEvidence(reg, supplemental) ||
		resultSlotCarriesUntrustedTopEvidence(reg, supplemental) {
		return product.Value{}, false
	}
	if resultSlotLacksSpecificTypeEvidence(reg, current) {
		return supplemental, true
	}
	if resultSlotCarriesUntrustedTopEvidence(reg, current) {
		return supplemental, true
	}
	if product.LessOrEq(reg, supplemental, current) {
		merged := product.Meet(reg, current, supplemental)
		if product.Equal(reg, merged, product.Bottom(reg)) {
			return supplemental, true
		}
		return product.WithPresence(reg, merged, product.PresenceOf(supplemental)), true
	}
	return product.Value{}, false
}

func resultSlotEvidencePromotesGradualToExplicit(reg *axis.Registry, current, supplemental product.Value) bool {
	if reg == nil {
		return false
	}
	currentEvidence := product.Get(reg, current, evidence.Key)
	supplementalEvidence := product.Get(reg, supplemental, evidence.Key)
	return currentEvidence.IsGradualTop() && supplementalEvidence.IsExplicitTop()
}

func resultSlotCarriesUntrustedTopEvidence(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func withSupplementalFacts(reg *axis.Registry, out, second callpayload.CallOutcome) callpayload.CallOutcome {
	authoritative := out.PostReturnAuthority
	for _, lane := range supplementalFactLanes {
		handler := lane.Value
		if authoritative && lane.Role.PostReturn {
			if handler.mergeAuthoritative != nil {
				handler.mergeAuthoritative(reg, &out, second)
			}
			continue
		}
		handler.merge(reg, &out, second)
	}
	if !authoritative {
		out.PostReturnAuthority = second.PostReturnAuthority
	}
	return out
}

type supplementalFactLaneHandler struct {
	merge              func(*axis.Registry, *callpayload.CallOutcome, callpayload.CallOutcome)
	mergeAuthoritative func(*axis.Registry, *callpayload.CallOutcome, callpayload.CallOutcome)
}

var supplementalFactLanes = callpayload.BindCallOutcomeSupplementalFactRoles("supplemental fact", map[string]supplementalFactLaneHandler{
	"NormalReturnFacts": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.NormalReturnFacts = out.NormalReturnFacts.Append(second.NormalReturnFacts)
		},
	},
	"HeapTableObjects": {
		merge: func(reg *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.HeapTableObjects = withSupplementalHeapTableObjects(reg, out.HeapTableObjects, second.HeapTableObjects)
		},
		mergeAuthoritative: func(reg *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.HeapTableObjects = withAuthoritativeResultHeapTableObjects(reg, out.HeapTableObjects, second.HeapTableObjects, out.Results)
		},
	},
	"Placements": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.Placements = withSupplementalPlacements(out.Placements, second.Placements)
		},
	},
	"ParamObligations": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamObligations = append(out.ParamObligations, second.ParamObligations...)
		},
	},
	"PathObligations": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.PathObligations = append(out.PathObligations, second.PathObligations...)
		},
	},
	"ParamPathRefinements": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamPathRefinements = append(out.ParamPathRefinements, second.ParamPathRefinements...)
		},
	},
	"ParamPathWrites": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamPathWrites = append(out.ParamPathWrites, second.ParamPathWrites...)
		},
	},
	"ParamLengthFloors": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamLengthFloors = append(out.ParamLengthFloors, second.ParamLengthFloors...)
		},
	},
	"ParamPathInvalidations": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamPathInvalidations = append(out.ParamPathInvalidations, second.ParamPathInvalidations...)
		},
	},
	"ParamConditions": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamConditions = append(out.ParamConditions, second.ParamConditions...)
		},
	},
	"ParamPathRelations": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamPathRelations = append(out.ParamPathRelations, second.ParamPathRelations...)
		},
	},
	"ReturnConditionRefinements": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ReturnConditionRefinements = append(out.ReturnConditionRefinements, second.ReturnConditionRefinements...)
		},
	},
	"ReturnConditionSlots": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ReturnConditionSlots = append(out.ReturnConditionSlots, second.ReturnConditionSlots...)
		},
	},
	"ReturnPresenceRelations": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ReturnPresenceRelations = append(out.ReturnPresenceRelations, second.ReturnPresenceRelations...)
		},
	},
	"ParamExposures": {
		merge: func(_ *axis.Registry, out *callpayload.CallOutcome, second callpayload.CallOutcome) {
			out.ParamExposures = append(out.ParamExposures, second.ParamExposures...)
		},
	},
}, func(handler supplementalFactLaneHandler) bool { return handler.merge != nil })

func withAuthoritativeResultHeapTableObjects(
	reg *axis.Registry,
	left, right map[identity.ID]heapidentity.TableObject,
	results []callpayload.CallResult,
) map[identity.ID]heapidentity.TableObject {
	if len(right) == 0 || len(results) == 0 {
		return left
	}
	if reg == nil {
		return left
	}
	allowed := make(map[identity.ID]product.Value, len(results))
	for _, result := range results {
		if id, ok := product.Get(reg, result.Value, identity.Key).ID(); ok {
			allowed[id] = result.Value
		}
	}
	if len(allowed) == 0 {
		return left
	}
	filtered := make(map[identity.ID]heapidentity.TableObject)
	for id, object := range right {
		if resultValue, ok := allowed[id]; ok && supplementalHeapObjectCarriesEvidence(reg, object, resultValue) {
			filtered[id] = object
		}
	}
	if len(filtered) == 0 {
		return left
	}
	return withSupplementalHeapTableObjects(reg, left, filtered)
}

func supplementalHeapObjectCarriesEvidence(reg *axis.Registry, object heapidentity.TableObject, resultValue product.Value) bool {
	if len(object.StaticMembers()) != 0 || len(object.DynamicIndexFacts()) != 0 {
		return true
	}
	if reg == nil {
		return true
	}
	return !product.Equal(reg, object.Root(), resultValue)
}

func withSupplementalPlacements(
	left, right map[identity.ID]placement.Value,
) map[identity.ID]placement.Value {
	if len(right) == 0 {
		return left
	}
	if len(left) == 0 {
		return right
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
		return left
	}
	if len(left) == 0 {
		return right
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
	out := heapidentity.CloneMap(left)
	for id, object := range right {
		if existing, ok := out[id]; ok {
			out[id] = mergeSupplementalHeapTableObject(reg, existing, object)
			continue
		}
		out[id] = object
	}
	return out
}

func mergeSupplementalHeapTableObject(reg *axis.Registry, left, right heapidentity.TableObject) heapidentity.TableObject {
	domain := heapidentity.ObjectDomain(reg)
	switch {
	case domain.Equal(left, domain.Bottom()):
		return right
	case domain.Equal(right, domain.Bottom()):
		return left
	}
	staticMembers := left.StaticMembers()
	for key, value := range right.StaticMembers() {
		if existing, ok := staticMembers[key]; ok {
			staticMembers[key] = product.Join(reg, existing, value)
			continue
		}
		staticMembers[key] = value
	}
	dynamicFacts := left.DynamicIndexFacts()
	dynamicDomain := dynamicindex.Domain(reg)
	for key, fact := range right.DynamicIndexFacts() {
		if existing, ok := dynamicFacts[key]; ok {
			dynamicFacts[key] = dynamicDomain.Join(existing, fact)
			continue
		}
		dynamicFacts[key] = fact
	}
	return heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              product.Join(reg, left.Root(), right.Root()),
		StaticMembers:     staticMembers,
		DynamicIndexFacts: dynamicFacts,
	})
}

func resultSlotLacksSpecificTypeEvidence(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return true
	}
	return typ.IsAny(t) || typ.IsUnknown(t) || typerefinement.ContainsFreeTypeParam(t)
}

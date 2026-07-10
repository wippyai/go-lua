package signature

import (
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SubstituteOperationalTypes returns a copy of effects whose embedded type
// payloads have been substituted with the provided generic bindings. It keeps
// path/identity/effect structure intact; only typ.Type fields are rewritten.
func SubstituteOperationalTypes(e *OperationalEffects, params []*typ.TypeParam, args []typ.Type) *OperationalEffects {
	if e == nil || len(params) == 0 || len(params) != len(args) {
		return cloneOperationalEffects(e)
	}
	out := e.Clone()
	for _, lane := range operationalEffectLanes {
		if lane.substituteTypes != nil {
			lane.substituteTypes(&out, params, args)
		}
	}
	return &out
}

func substitutePathTypeRefinementTypes(e *OperationalEffects, params []*typ.TypeParam, args []typ.Type) {
	for i := range e.NormalReturnTypeRefinements {
		e.NormalReturnTypeRefinements[i].Type = subst.Params(e.NormalReturnTypeRefinements[i].Type, params, args)
	}
}

func substitutePathStaticMemberTypes(e *OperationalEffects, params []*typ.TypeParam, args []typ.Type) {
	for i := range e.PathStaticMembers {
		e.PathStaticMembers[i].Type = subst.Params(e.PathStaticMembers[i].Type, params, args)
	}
}

func substitutePathStaticMemberDeltaTypes(e *OperationalEffects, params []*typ.TypeParam, args []typ.Type) {
	for i := range e.PathStaticMemberDeltas {
		e.PathStaticMemberDeltas[i].Type = subst.Params(e.PathStaticMemberDeltas[i].Type, params, args)
	}
}

func substitutePathPresenceImplicationTypes(e *OperationalEffects, params []*typ.TypeParam, args []typ.Type) {
	for i := range e.PathPresenceImplications {
		e.PathPresenceImplications[i].TriggerType = subst.Params(e.PathPresenceImplications[i].TriggerType, params, args)
	}
}

func substituteDynamicIndexFactTypes(e *OperationalEffects, params []*typ.TypeParam, args []typ.Type) {
	for i := range e.DynamicIndexFacts {
		e.DynamicIndexFacts[i].Key.Type = subst.Params(e.DynamicIndexFacts[i].Key.Type, params, args)
		e.DynamicIndexFacts[i].Value.Type = subst.Params(e.DynamicIndexFacts[i].Value.Type, params, args)
	}
}

func substituteReturnAllocationTemplateTypes(e *OperationalEffects, params []*typ.TypeParam, args []typ.Type) {
	for templateIndex := range e.ReturnAllocationTemplates {
		objects := e.ReturnAllocationTemplates[templateIndex].Objects
		for objectIndex := range objects {
			objects[objectIndex].Type = subst.Params(objects[objectIndex].Type, params, args)
			for entryIndex := range objects[objectIndex].DynamicEntries {
				entry := &objects[objectIndex].DynamicEntries[entryIndex]
				entry.KeyType = subst.Params(entry.KeyType, params, args)
			}
		}
	}
}

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
	for i := range out.NormalReturnTypeRefinements {
		out.NormalReturnTypeRefinements[i].Type = subst.Params(out.NormalReturnTypeRefinements[i].Type, params, args)
	}
	for i := range out.PathStaticMembers {
		out.PathStaticMembers[i].Type = subst.Params(out.PathStaticMembers[i].Type, params, args)
	}
	for i := range out.PathPresenceImplications {
		out.PathPresenceImplications[i].TriggerType = subst.Params(out.PathPresenceImplications[i].TriggerType, params, args)
	}
	for i := range out.DynamicIndexFacts {
		out.DynamicIndexFacts[i].Key.Type = subst.Params(out.DynamicIndexFacts[i].Key.Type, params, args)
		out.DynamicIndexFacts[i].Value.Type = subst.Params(out.DynamicIndexFacts[i].Value.Type, params, args)
	}
	for templateIndex := range out.ReturnAllocationTemplates {
		objects := out.ReturnAllocationTemplates[templateIndex].Objects
		for objectIndex := range objects {
			objects[objectIndex].Type = subst.Params(objects[objectIndex].Type, params, args)
			for entryIndex := range objects[objectIndex].DynamicEntries {
				entry := &objects[objectIndex].DynamicEntries[entryIndex]
				entry.KeyType = subst.Params(entry.KeyType, params, args)
			}
		}
	}
	return &out
}

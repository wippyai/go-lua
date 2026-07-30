package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// StaticSignatureAllocationTemplate returns the sole canonical return
// allocation template when a resolved signature is context independent.
func StaticSignatureAllocationTemplate(sig signature.Function) (signature.ReturnAllocationTemplate, bool) {
	if sig.Type == nil || !sig.Effect.Pure() || len(sig.Type.TypeParams) != 0 || sig.OperationalEffects == nil {
		return signature.ReturnAllocationTemplate{}, false
	}
	effects := sig.OperationalEffects.Clone()
	if len(effects.ReturnAllocationTemplates) != 1 {
		return signature.ReturnAllocationTemplate{}, false
	}
	template := effects.ReturnAllocationTemplates[0]
	effects.ReturnAllocationTemplates = nil
	if !effects.IsEmpty() || !exactStaticAllocationTemplate(sig.Type, template) {
		return signature.ReturnAllocationTemplate{}, false
	}
	return template, true
}

func exactStaticAllocationTemplate(fn *typ.Function, template signature.ReturnAllocationTemplate) bool {
	if len(fn.Returns) != 1 || template.ReturnIndex != 0 || template.Root == "" || len(template.Objects) == 0 {
		return false
	}
	seen := make(map[signature.AllocationTemplateID]struct{}, len(template.Objects))
	rootFound := false
	for _, object := range template.Objects {
		if object.ID == "" || object.Type == nil || typ.ContainsTypeParam(object.Type) {
			return false
		}
		if _, duplicate := seen[object.ID]; duplicate {
			return false
		}
		seen[object.ID] = struct{}{}
		rootFound = rootFound || object.ID == template.Root
		if len(template.Objects) != 1 || object.ID != template.Root || len(object.StaticMembers) != 0 || len(object.DynamicEntries) != 0 {
			return false
		}
		for _, member := range object.StaticMembers {
			if member.Value == "" {
				return false
			}
		}
		for _, entry := range object.DynamicEntries {
			if entry.Value == "" || (entry.KeyType != nil && typ.ContainsTypeParam(entry.KeyType)) {
				return false
			}
		}
	}
	return rootFound
}

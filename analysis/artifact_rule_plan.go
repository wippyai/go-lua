package analysis

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/call/activation"
	"github.com/wippyai/go-lua/domain/composite"
)

// walkArtifactRulePlacements enumerates the sealed artifact placements in
// issuance order. Admission and construction both walk this inventory; there
// is no copied member-ref handoff between them.
func walkArtifactRulePlacements(mounts []mountedProgramArtifact, visit func(key schema.Key, mount, point, occurrence identity.ContentID) bool) (schema.Key, bool) {
	if len(mounts) == 0 {
		return "", false
	}
	for _, mount := range mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() {
			return "", false
		}
		for index := 0; index < mount.artifact.RulePlacementCount(); index++ {
			row, ok := mount.artifact.RulePlacementAt(index)
			if !ok || row.PointCount() == 0 || !row.Key().Available() {
				return row.Key(), false
			}
			key := row.Key()
			for pointIndex := 0; pointIndex < row.PointCount(); pointIndex++ {
				point, pointOK := row.PointAt(pointIndex)
				if !pointOK || !visit(key, mount.moduleKey, point, row.ID()) {
					return key, false
				}
			}
		}
	}
	return "", true
}

// attachArtifactRuleMembers binds every sealed mounted placement to one
// committed topology compilation. Keys without a complete post-commit owner
// bridge fail closed; they are not silently omitted from the solver.
func attachArtifactRuleMembers(binding *composite.ProgramBinding, compilation *engine.ProgramConstruction, mounts []mountedProgramArtifact) bool {
	rules := binding.Rules()
	if rules == nil || compilation == nil {
		return false
	}
	_, ok := walkArtifactRulePlacements(mounts, func(key schema.Key, mount, point, occurrence identity.ContentID) bool {
		return rules.AttachMemberByKey(key, compilation, mount, point, occurrence)
	})
	return ok
}

// attachLinkBootstrapRules admits the one non-mounted bootstrap plane. Each
// Link-lane rule in the table publishes its own occurrence inventory, so the
// plane is admitted by walking the table rather than by naming its members.
func attachLinkBootstrapRules(binding *composite.ProgramBinding, assembly *engine.ReceiptAssembly) bool {
	rules := binding.Rules()
	if rules == nil || assembly == nil {
		return false
	}
	return walkLinkBootstrap(binding, func(key schema.Key, id identity.ContentID) bool {
		return admitLinkRule(rules, assembly, key, id)
	})
}

func attachLinkBootstrapMembers(binding *composite.ProgramBinding, compilation *engine.ProgramConstruction) bool {
	rules := binding.Rules()
	if rules == nil || compilation == nil {
		return false
	}
	return walkLinkBootstrap(binding, func(key schema.Key, id identity.ContentID) bool {
		return rules.AttachLinkMemberByKey(key, compilation, id)
	})
}

// walkLinkBootstrap pairs each sealed Link-lane key with the occurrence
// identities that rule's own catalog published, in table order.
func walkLinkBootstrap(binding *composite.ProgramBinding, admit func(schema.Key, identity.ContentID) bool) bool {
	if binding == nil {
		return false
	}
	keys := composite.LinkKeys()
	if len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		ids, ok := linkOccurrenceIDs(binding, key)
		if !ok {
			return false
		}
		for _, id := range ids {
			if !admit(key, id) {
				return false
			}
		}
	}
	return true
}

// attachArtifactRules is the central declaration-key admission pass. Every
// issued placement is admitted while ReceiptAssembly sources remain open,
// through the sealed rule table; no key is inferred or routed through a
// generic fallback.
func attachArtifactRules(binding *composite.ProgramBinding, assembly *engine.ReceiptAssembly, mounts []mountedProgramArtifact) (AnalyzeDiagnosticRule, bool) {
	rules := binding.Rules()
	if rules == nil || assembly == nil {
		return AnalyzeDiagnosticRuleUnknown, false
	}
	key, ok := walkArtifactRulePlacements(mounts, func(key schema.Key, mount, point, occurrence identity.ContentID) bool {
		return admitMountedRule(rules, assembly, key, mount, point, occurrence)
	})
	if !ok {
		return composite.DiagnosticRuleForKey(key), false
	}
	return AnalyzeDiagnosticRuleUnknown, true
}

func admitMountedRule(rules *composite.RuleBinding, assembly *engine.ReceiptAssembly, key schema.Key, mount, point, occurrence identity.ContentID) bool {
	if hot, ok := composite.RuleHandleByKey[*activation.HotRule](rules, key); ok {
		admit, admitOK := hot.MountedAdmit(mount, point, occurrence)
		return admitOK && engine.AdmitMountedActivationOccurrence(assembly, admit)
	}
	attach, attachOK := rules.ProgramAttachByKey(key)
	capability, capabilityOK := rules.CapabilityByKey(key)
	return attachOK && capabilityOK && capability.Mounted() && attach.AdmitMounted(assembly, capability, mount, point, occurrence)
}

func admitLinkRule(rules *composite.RuleBinding, assembly *engine.ReceiptAssembly, key schema.Key, occurrence identity.ContentID) bool {
	attach, attachOK := rules.ProgramAttachByKey(key)
	capability, capabilityOK := rules.CapabilityByKey(key)
	return attachOK && capabilityOK && capability.Link() && attach.AdmitLink(assembly, capability, occurrence)
}

func diagnosticRuleForMountedRole(binding *composite.ProgramBinding, role engine.RuleSlotCapability) AnalyzeDiagnosticRule {
	rules := binding.Rules()
	if rules == nil || !role.Mounted() {
		return AnalyzeDiagnosticRuleUnknown
	}
	return rules.DiagnosticForCapability(role)
}

func diagnosticRuleForLinkRole(binding *composite.ProgramBinding, role engine.RuleSlotCapability) AnalyzeDiagnosticRule {
	rules := binding.Rules()
	if rules == nil || !role.Link() {
		return AnalyzeDiagnosticRuleUnknown
	}
	return rules.DiagnosticForCapability(role)
}

// mountedCapability resolves a mounted rule by its declared key.
func mountedCapability(binding *composite.ProgramBinding, key schema.Key) (engine.RuleSlotCapability, bool) {
	rules := binding.Rules()
	if rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := rules.CapabilityByKey(key)
	return capability, ok && capability.Mounted()
}

// linkCapability is the mount-neutral counterpart for Link-owned rules.
func linkCapability(binding *composite.ProgramBinding, key schema.Key) (engine.RuleSlotCapability, bool) {
	rules := binding.Rules()
	if rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := rules.CapabilityByKey(key)
	return capability, ok && capability.Link()
}

// linkOccurrenceIDs enumerates one Link rule's admitted occurrences from its
// own published catalog.
func linkOccurrenceIDs(binding *composite.ProgramBinding, key schema.Key) ([]identity.ContentID, bool) {
	rules := binding.Rules()
	if rules == nil {
		return nil, false
	}
	catalog, ok := rules.LinkCatalogByKey(key)
	if !ok {
		return nil, false
	}
	ids := make([]identity.ContentID, catalog.Count())
	for index := range ids {
		id, idOK := catalog.IDAt(index)
		if !idOK {
			return nil, false
		}
		ids[index] = id
	}
	return ids, true
}

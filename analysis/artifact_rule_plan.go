package analysis

import (
	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// artifactRuleMemberRef is the private handoff from mounted-row admission to
// post-commit member attachment. It retains the exact role and authored
// coordinates in admission order; no public receipt or plan vocabulary leaks
// across this boundary.
type artifactRuleMemberRef struct {
	role                     programartifact.RuleRole
	mount, point, occurrence identity.ContentID
}

// attachArtifactRuleMembers binds the already-admitted mounted members to one
// committed topology compilation. Roles without a complete post-commit owner
// bridge fail closed; they are not silently omitted from the solver.
func attachArtifactRuleMembers(binding *composite.ProgramBinding, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mounts []mountedProgramArtifact) bool {
	rules := binding.Rules()
	if rules == nil || compilation == nil || graph == nil || len(mounts) == 0 {
		return false
	}
	for _, mount := range mounts {
		if !mount.ruleMembersReady {
			return false
		}
		for _, member := range mount.ruleMembers {
			if !rules.AttachMember(member.role, compilation, graph, member.mount, member.point, member.occurrence) {
				return false
			}
		}
	}
	return true
}

// attachLinkBootstrapRules admits the one non-mounted bootstrap plane. Each
// Link-lane rule in the table publishes its own occurrence inventory, so the
// plane is admitted by walking the table rather than by naming its members.
func attachLinkBootstrapRules(binding *composite.ProgramBinding, assembly *engine.ReceiptAssembly, valueIDs, heapIDs []identity.ContentID) bool {
	rules := binding.Rules()
	if rules == nil || assembly == nil {
		return false
	}
	return walkLinkBootstrap(valueIDs, heapIDs, func(role programartifact.RuleRole, id identity.ContentID) bool {
		return rules.AttachLink(role, assembly, id)
	})
}

func attachLinkBootstrapMembers(binding *composite.ProgramBinding, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, valueIDs, heapIDs []identity.ContentID) bool {
	rules := binding.Rules()
	if rules == nil || compilation == nil || graph == nil {
		return false
	}
	return walkLinkBootstrap(valueIDs, heapIDs, func(role programartifact.RuleRole, id identity.ContentID) bool {
		return rules.AttachLinkMember(role, compilation, graph, id)
	})
}

// walkLinkBootstrap pairs each Link-lane role with the occurrence identities
// its own catalog published, in table order.
func walkLinkBootstrap(valueIDs, heapIDs []identity.ContentID, admit func(programartifact.RuleRole, identity.ContentID) bool) bool {
	roles := composite.LinkRoles()
	if len(roles) == 0 {
		return false
	}
	for _, role := range roles {
		ids := valueIDs
		if role == programartifact.RuleRoleHeapBootstrap {
			ids = heapIDs
		}
		for _, id := range ids {
			if !admit(role, id) {
				return false
			}
		}
	}
	return true
}

// attachArtifactRules is the central RuleRole admission pass. Every row is
// admitted while ReceiptAssembly sources remain open, through the sealed rule
// table; no role is inferred or routed through a generic fallback.
func attachArtifactRules(binding *composite.ProgramBinding, assembly *engine.ReceiptAssembly, mounts []mountedProgramArtifact) (AnalyzeDiagnosticRule, bool) {
	rules := binding.Rules()
	if rules == nil || assembly == nil || len(mounts) == 0 {
		return AnalyzeDiagnosticRuleUnknown, false
	}
	memberRefs := make([][]artifactRuleMemberRef, len(mounts))
	for mountIndex := range mounts {
		// A failed or repeated admission must not leave stale post-commit
		// references available to a later compilation transaction.
		mounts[mountIndex].ruleMembers = nil
		mounts[mountIndex].ruleMembersReady = false
	}
	for mountIndex, mount := range mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() {
			return AnalyzeDiagnosticRuleUnknown, false
		}
		for roleIndex := 0; roleIndex < programartifact.MountedRuleRoleCount(); roleIndex++ {
			role, roleOK := programartifact.MountedRuleRoleAt(roleIndex)
			if !roleOK {
				return AnalyzeDiagnosticRuleUnknown, false
			}
			for index := 0; index < mount.artifact.RuleOccurrenceCount(role); index++ {
				row, ok := mount.artifact.RuleOccurrenceAt(role, index)
				if !ok || row.PointCount() == 0 {
					return composite.DiagnosticRuleForRole(role), false
				}
				for pointIndex := 0; pointIndex < row.PointCount(); pointIndex++ {
					point, pointOK := row.PointAt(pointIndex)
					if !pointOK || !rules.Attach(role, assembly, mount.moduleKey, point, row.ID()) {
						return composite.DiagnosticRuleForRole(role), false
					}
					memberRefs[mountIndex] = append(memberRefs[mountIndex], artifactRuleMemberRef{role: role, mount: mount.moduleKey, point: point, occurrence: row.ID()})
				}
			}
		}
	}
	for mountIndex := range mounts {
		mounts[mountIndex].ruleMembers = memberRefs[mountIndex]
		mounts[mountIndex].ruleMembersReady = true
	}
	return AnalyzeDiagnosticRuleUnknown, true
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

// mountedCapability resolves a mounted rule by its sealed table role.
func mountedCapability(binding *composite.ProgramBinding, role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
	rules := binding.Rules()
	if rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := rules.Capability(role)
	return capability, ok && capability.Mounted()
}

// linkCapability is the mount-neutral counterpart for Link-owned rules.
func linkCapability(binding *composite.ProgramBinding, role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
	rules := binding.Rules()
	if rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := rules.Capability(role)
	return capability, ok && capability.Link()
}

// linkOccurrenceIDs enumerates one Link rule's admitted occurrences from its
// own published catalog.
func linkOccurrenceIDs(binding *composite.ProgramBinding, role programartifact.RuleRole) ([]identity.ContentID, bool) {
	rules := binding.Rules()
	if rules == nil {
		return nil, false
	}
	catalog, ok := rules.LinkCatalog(role)
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

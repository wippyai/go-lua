package analysis

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/keyspace"
)

// artifactRuleMemberRef is the private handoff from mounted-row admission to
// post-commit member attachment. It retains the exact role and authored
// coordinates in admission order; no public receipt or plan vocabulary leaks
// across this boundary.
type artifactRuleMemberRef struct {
	role                     programartifact.RuleRole
	mount, point, occurrence keyspace.ContentID
}

// attachArtifactRuleMembers binds the already-admitted mounted members to one
// committed topology compilation. Roles without a complete post-commit owner
// bridge fail closed; they are not silently omitted from the solver.
func (binding *programBinding) attachArtifactRuleMembers(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mounts []mountedProgramArtifact) bool {
	if binding == nil || compilation == nil || graph == nil || len(mounts) == 0 {
		return false
	}
	for _, mount := range mounts {
		if !mount.ruleMembersReady {
			return false
		}
		for _, member := range mount.ruleMembers {
			if !attachArtifactRuleMember(binding, compilation, graph, member.role, member.mount, member.point, member.occurrence) {
				return false
			}
		}
	}
	return true
}

// attachLinkBootstrapRules admits the one non-mounted bootstrap plane. Value
// bootstrap requires its owner-specific Link bridge; Heap bootstrap is already
// fully receipt-native here.
func (binding *programBinding) attachLinkBootstrapRules(assembly *engine.ReceiptAssembly, valueIDs, heapIDs []keyspace.ContentID) bool {
	if binding == nil || assembly == nil {
		return false
	}
	for _, id := range valueIDs {
		if _, ok := binding.valueBootstrap.AttachLinkOccurrence(assembly, id); !ok {
			return false
		}
	}
	for _, id := range heapIDs {
		if _, ok := binding.heapBootstrap.AttachLinkOccurrence(assembly, id); !ok {
			return false
		}
	}
	return true
}

func (binding *programBinding) attachLinkBootstrapMembers(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, valueIDs, heapIDs []keyspace.ContentID) bool {
	if binding == nil || compilation == nil || graph == nil {
		return false
	}
	for _, id := range valueIDs {
		if _, ok := binding.valueBootstrap.AttachLinkReceiptMember(compilation, graph, id); !ok {
			return false
		}
	}
	for _, id := range heapIDs {
		if _, ok := binding.heapBootstrap.AttachLinkReceiptMember(compilation, graph, id); !ok {
			return false
		}
	}
	return true
}

func attachArtifactRuleMember(binding *programBinding, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, role programartifact.RuleRole, mount, point, occurrence keyspace.ContentID) bool {
	engineRole, ok := mountedRole(binding, role)
	if !ok {
		return false
	}
	if role == programartifact.RuleRoleCallActivation {
		_, ok := binding.callActivation.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	}
	if _, ok := graph.MountedRuleMember(engineRole, mount, point, occurrence); !ok {
		return false
	}
	switch role {
	case programartifact.RuleRoleValueSource:
		_, ok := binding.valueSource.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRolePackSource:
		_, ok := binding.packSource.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleHeapIngress:
		_, ok := binding.heapIngress.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueAllocation:
		_, ok := binding.valueAllocation.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleHeapEmpty:
		_, ok = binding.heapEmpty.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleHeapClosed:
		_, ok := binding.heapClosed.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleRawGet:
		_, ok := binding.rawGet.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleRawSet:
		_, ok := binding.rawSet.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleEffectSelected:
		_, ok := binding.effectSelected.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleEffectOpaque:
		_, ok := binding.effectOpaque.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleEffectBody:
		_, ok := binding.effectBody.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleCallDispatch:
		_, ok := binding.callDispatch.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueStorageTransfer:
		_, ok = binding.valueTransfer.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueBinaryArithmetic:
		_, ok = binding.valueArithmetic.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueBinaryEquality:
		_, ok = binding.valueEquality.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueBinaryOrder:
		_, ok = binding.valueOrder.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValuePresenceRefinement:
		_, ok = binding.valueRefinement.AttachMountedReceiptMember(compilation, graph, mount, point, occurrence)
		return ok
	default:
		return false
	}
}

func mountedRole(binding *programBinding, role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
	if binding == nil {
		return engine.RuleSlotCapability{}, false
	}
	var semantic engine.SemanticKey
	switch role {
	case programartifact.RuleRoleValueSource:
		semantic = binding.vocabulary.ValueSourceRule.Rule
	case programartifact.RuleRolePackSource:
		semantic = binding.vocabulary.PackSourceRule.Rule
	case programartifact.RuleRoleHeapIngress:
		semantic = binding.vocabulary.HeapIngressRule.Rule
	case programartifact.RuleRoleValueAllocation:
		semantic = binding.vocabulary.ValueAllocationRule.Rule
	case programartifact.RuleRoleHeapEmpty:
		semantic = binding.vocabulary.HeapEmptyRule.Rule
	case programartifact.RuleRoleHeapClosed:
		semantic = binding.vocabulary.HeapClosedRule.Rule
	case programartifact.RuleRoleRawGet:
		semantic = binding.vocabulary.RawGetRule.Rule
	case programartifact.RuleRoleRawSet:
		semantic = binding.vocabulary.RawSetRule.Rule
	case programartifact.RuleRoleCallDispatch:
		semantic = binding.vocabulary.CallDispatchRule.Rule
	case programartifact.RuleRoleEffectSelected:
		semantic = binding.vocabulary.EffectSelectedRule.Rule
	case programartifact.RuleRoleEffectOpaque:
		semantic = binding.vocabulary.EffectOpaqueRule.Rule
	case programartifact.RuleRoleEffectBody:
		semantic = binding.vocabulary.EffectBodyRule.Rule
	case programartifact.RuleRoleCallActivation:
		semantic = binding.vocabulary.CallActivation
	case programartifact.RuleRoleValueStorageTransfer:
		semantic = binding.vocabulary.ValueTransferRule.Rule
	case programartifact.RuleRoleValueBinaryArithmetic:
		semantic = binding.vocabulary.ValueBinaryArithmeticRule.Rule
	case programartifact.RuleRoleValueBinaryEquality:
		semantic = binding.vocabulary.ValueBinaryEqualityRule.Rule
	case programartifact.RuleRoleValueBinaryOrder:
		semantic = binding.vocabulary.ValueBinaryOrderRule.Rule
	case programartifact.RuleRoleValuePresenceRefinement:
		semantic = binding.vocabulary.ValuePresenceRefinementRule.Rule
	default:
		return engine.RuleSlotCapability{}, false
	}
	return binding.mountedCapability(semantic)
}

// attachArtifactRules is the central closed RuleRole dispatcher. Every row is
// admitted while ReceiptAssembly sources remain open; no role is inferred or
// routed through a generic fallback.
func (binding *programBinding) attachArtifactRules(assembly *engine.ReceiptAssembly, mounts []mountedProgramArtifact) (AnalyzeDiagnosticRule, bool) {
	if binding == nil || assembly == nil || len(mounts) == 0 {
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
					return diagnosticRuleForArtifactRole(role), false
				}
				for pointIndex := 0; pointIndex < row.PointCount(); pointIndex++ {
					point, pointOK := row.PointAt(pointIndex)
					if !pointOK || !attachArtifactRule(binding, assembly, role, mount.moduleKey, point, row.ID()) {
						return diagnosticRuleForArtifactRole(role), false
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

func diagnosticRuleForArtifactRole(role programartifact.RuleRole) AnalyzeDiagnosticRule {
	switch role {
	case programartifact.RuleRoleValueSource:
		return AnalyzeDiagnosticRuleValueSource
	case programartifact.RuleRolePackSource:
		return AnalyzeDiagnosticRulePackSource
	case programartifact.RuleRoleHeapIngress:
		return AnalyzeDiagnosticRuleHeapIngress
	case programartifact.RuleRoleValueAllocation:
		return AnalyzeDiagnosticRuleValueAllocation
	case programartifact.RuleRoleHeapEmpty:
		return AnalyzeDiagnosticRuleHeapEmpty
	case programartifact.RuleRoleHeapClosed:
		return AnalyzeDiagnosticRuleHeapClosed
	case programartifact.RuleRoleRawGet:
		return AnalyzeDiagnosticRuleRawGet
	case programartifact.RuleRoleRawSet:
		return AnalyzeDiagnosticRuleRawSet
	case programartifact.RuleRoleCallDispatch:
		return AnalyzeDiagnosticRuleCallDispatch
	case programartifact.RuleRoleEffectSelected:
		return AnalyzeDiagnosticRuleEffectSelected
	case programartifact.RuleRoleEffectOpaque:
		return AnalyzeDiagnosticRuleEffectOpaque
	case programartifact.RuleRoleEffectBody:
		return AnalyzeDiagnosticRuleEffectBody
	case programartifact.RuleRoleCallActivation:
		return AnalyzeDiagnosticRuleCallActivation
	case programartifact.RuleRoleValueStorageTransfer:
		return AnalyzeDiagnosticRuleValueTransfer
	case programartifact.RuleRoleValueBinaryArithmetic:
		return AnalyzeDiagnosticRuleValueBinaryArithmetic
	case programartifact.RuleRoleValueBinaryEquality:
		return AnalyzeDiagnosticRuleValueBinaryEquality
	case programartifact.RuleRoleValueBinaryOrder:
		return AnalyzeDiagnosticRuleValueBinaryOrder
	case programartifact.RuleRoleValuePresenceRefinement:
		return AnalyzeDiagnosticRuleValuePresenceRefinement
	default:
		return AnalyzeDiagnosticRuleUnknown
	}
}

func diagnosticRuleForMountedRole(binding *programBinding, role engine.RuleSlotCapability) AnalyzeDiagnosticRule {
	if binding == nil || !role.Mounted() {
		return AnalyzeDiagnosticRuleUnknown
	}
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		candidate, candidateOK := programartifact.MountedRuleRoleAt(index)
		if !candidateOK {
			return AnalyzeDiagnosticRuleUnknown
		}
		capability, ok := mountedRole(binding, candidate)
		if ok && capability == role {
			return diagnosticRuleForArtifactRole(candidate)
		}
	}
	return AnalyzeDiagnosticRuleUnknown
}

func diagnosticRuleForLinkRole(binding *programBinding, role engine.RuleSlotCapability) AnalyzeDiagnosticRule {
	if binding == nil || !role.Link() {
		return AnalyzeDiagnosticRuleUnknown
	}
	valueCapability, valueOK := binding.linkCapability(binding.vocabulary.ValueBootstrapRule.Rule)
	if valueOK && valueCapability == role {
		return AnalyzeDiagnosticRuleValueBootstrap
	}
	heapCapability, heapOK := binding.linkCapability(binding.vocabulary.HeapBootstrapRule.Rule)
	if heapOK && heapCapability == role {
		return AnalyzeDiagnosticRuleHeapBootstrap
	}
	return AnalyzeDiagnosticRuleUnknown
}

func attachArtifactRule(binding *programBinding, assembly *engine.ReceiptAssembly, role programartifact.RuleRole, mount, point, occurrence keyspace.ContentID) bool {
	switch role {
	case programartifact.RuleRoleValueSource:
		_, ok := binding.valueSource.AttachMountedRule(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRolePackSource:
		_, ok := binding.packSource.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleHeapIngress:
		_, ok := binding.heapIngress.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueAllocation:
		_, ok := binding.valueAllocation.AttachMountedRule(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleHeapEmpty:
		_, ok := binding.heapEmpty.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleHeapClosed:
		_, ok := binding.heapClosed.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleRawGet:
		_, ok := binding.rawGet.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleRawSet:
		_, ok := binding.rawSet.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleCallDispatch:
		_, ok := binding.callDispatch.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleEffectSelected:
		_, ok := binding.effectSelected.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleEffectOpaque:
		_, ok := binding.effectOpaque.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleEffectBody:
		_, ok := binding.effectBody.AttachMountedOccurrence(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleCallActivation:
		return binding.callActivation.AttachMountedOccurrence(assembly, mount, point, occurrence)
	case programartifact.RuleRoleValueStorageTransfer:
		_, ok := binding.valueTransfer.AttachMountedRule(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueBinaryArithmetic:
		_, ok := binding.valueArithmetic.AttachMountedRule(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueBinaryEquality:
		_, ok := binding.valueEquality.AttachMountedRule(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValueBinaryOrder:
		_, ok := binding.valueOrder.AttachMountedRule(assembly, mount, point, occurrence)
		return ok
	case programartifact.RuleRoleValuePresenceRefinement:
		_, ok := binding.valueRefinement.AttachMountedRule(assembly, mount, point, occurrence)
		return ok
	default:
		return false
	}
}

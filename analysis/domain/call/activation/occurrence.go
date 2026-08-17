package activation

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// MountedIssuer is a rule-fenced view over Call's canonical mounted-call
// inverse for one exact module. It retains no duplicate occurrence map; each
// lookup borrows Call's owner-issued dense receipt in O(1).
type MountedIssuer struct {
	rule   *HotRule
	module identity.ContentID
}

func (rule *HotRule) SealOccurrenceReceipts() bool {
	if rule == nil || rule.owner == nil || rule.owner.Algebra() == nil {
		return false
	}
	if rule.receiptsSealed {
		return true
	}
	algebra := rule.owner.Algebra()
	for index := 0; index < algebra.MountedCallCount(); index++ {
		mounted, mountedOK := algebra.MountedCallAtHandle(index)
		applicationID, id, module, _, _, identityOK := algebra.MountedCallIdentity(mounted)
		key, keyOK := algebra.KeyForApplicationID(applicationID)
		inverse, inverseOK := algebra.MountedCallForOccurrence(module, id)
		if !mountedOK || !identityOK || !keyOK || !algebra.OwnsKey(key) || !inverseOK || inverse != mounted || !algebra.OwnsMountedModule(module) {
			return false
		}
	}
	rule.receiptsSealed = true
	return true
}

func (rule *HotRule) ForMount(module identity.ContentID) (MountedIssuer, bool) {
	if rule == nil || !rule.receiptsSealed || rule.owner == nil || rule.owner.Algebra() == nil || !module.Available() || !rule.owner.Algebra().OwnsMountedModule(module) {
		return MountedIssuer{}, false
	}
	issuer := MountedIssuer{rule: rule, module: module}
	return issuer, issuer.valid()
}

func (issuer MountedIssuer) valid() bool {
	return issuer.rule != nil && issuer.rule.receiptsSealed && issuer.rule.owner != nil && issuer.rule.owner.Algebra() != nil && issuer.module.Available() && issuer.rule.owner.Algebra().OwnsMountedModule(issuer.module)
}

func (issuer MountedIssuer) mounted(id identity.ContentID) (calldomain.MountedCall, bool) {
	if !issuer.valid() || !id.Available() {
		return calldomain.MountedCall{}, false
	}
	return issuer.rule.owner.Algebra().MountedCallForOccurrence(issuer.module, id)
}

func (issuer MountedIssuer) occurrenceKey(id identity.ContentID) (calldomain.Key, identity.ContentID, bool) {
	mounted, mountedOK := issuer.mounted(id)
	if !mountedOK {
		return calldomain.Key{}, identity.ContentID{}, false
	}
	applicationID, _, moduleID, _, _, identityOK := issuer.rule.owner.Algebra().MountedCallIdentity(mounted)
	if !identityOK || moduleID != issuer.module || !applicationID.Available() {
		return calldomain.Key{}, identity.ContentID{}, false
	}
	key, keyOK := issuer.rule.owner.Algebra().KeyForApplicationID(applicationID)
	return key, applicationID, keyOK && issuer.rule.owner.Algebra().OwnsKey(key)
}

func (issuer MountedIssuer) occurrenceOperands(id identity.ContentID) (calldomain.Key, identity.SemanticKey, bool) {
	key, applicationID, keyOK := issuer.occurrenceKey(id)
	if !keyOK {
		return calldomain.Key{}, identity.SemanticKey{}, false
	}
	application, applicationOK := identity.NewSemanticKey([32]byte(applicationID), 1)
	return key, application, applicationOK && application.Available()
}

func (issuer MountedIssuer) KeyForOccurrence(id identity.ContentID) (calldomain.Key, bool) {
	key, _, ok := issuer.occurrenceKey(id)
	return key, ok
}

func (issuer MountedIssuer) ApplicationForOccurrence(id identity.ContentID) (identity.SemanticKey, bool) {
	_, applicationID, ok := issuer.occurrenceKey(id)
	if !ok {
		return identity.SemanticKey{}, false
	}
	application, applicationOK := identity.NewSemanticKey([32]byte(applicationID), 1)
	return application, applicationOK && application.Available()
}

func (rule *HotRule) AttachMountedOccurrence(assembly *engine.ReceiptAssembly, mountID, reusablePointID, occurrenceID identity.ContentID) bool {
	if rule == nil || rule.owner == nil || rule.implementation == nil || assembly == nil {
		return false
	}
	issuer, ok := rule.ForMount(mountID)
	key, application, operandOK := issuer.occurrenceOperands(occurrenceID)
	if !ok || !operandOK || rule.catalog == nil || !rule.catalog.valid() {
		return false
	}
	// The reusable Program row remains in the artifact, but a Link with no
	// mounted Program-body targets has no activation candidate or runtime work
	// to instantiate. Exact occurrence ownership was still checked above.
	if len(rule.catalog.rows) == 0 {
		return true
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return false
	}
	occurrence, ok := assembly.AdmitMountedRuleOccurrence(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return false
	}
	implementation, ok := callowner.ResolveActivationRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return false
	}
	transaction, ok := engine.BeginMountedActivationRuleAdmission(assembly, implementation, occurrence, [32]byte(occurrenceID))
	if !ok {
		return false
	}
	ref, ok := rule.owner.Ref(key)
	if !ok || !engine.AddExactRead(transaction, ref) {
		return false
	}
	return assembly.QueueMountedRuleFinalizer(capability, func() bool {
		source, sourceOK := transaction.Seal()
		if !sourceOK {
			return false
		}
		draft, draftOK := implementation.BeginReceiptRuleRow(source)
		readPart, readPartOK := implementation.ReceiptReadPart(source, 0)
		if !draftOK || !readPartOK || !draft.AddRead(readPart) {
			return false
		}
		if !assembly.AddActivationRuleFromDraft(occurrence, draft) {
			return false
		}
		if rule.transport == nil || rule.catalog == nil || !rule.catalog.valid() {
			return false
		}
		for index := range rule.catalog.rows {
			row := rule.catalog.rows[index]
			if !rule.transport.AddMountedActivationCandidate(assembly, occurrence, application, row.target, row.endpoint, row.moduleKey, row.bodyPath) {
				return false
			}
		}
		return rule.transport.CompleteMountedActivationCandidates(assembly, occurrence, application, uint64(len(rule.catalog.rows)))
	})
}

// AttachMountedReceiptMember resolves and attaches one exact activation
// member from the committed activation graph.
func (rule *HotRule) AttachMountedReceiptMember(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mountID, reusablePointID, occurrenceID identity.ContentID) (*engine.AttachedActivationReceiptMember, bool) {
	if rule == nil || compilation == nil || graph == nil || rule.implementation == nil || rule.catalog == nil || !rule.catalog.valid() {
		return nil, false
	}
	issuer, issuerOK := rule.ForMount(mountID)
	_, _, operandOK := issuer.occurrenceKey(occurrenceID)
	if !issuerOK || !operandOK {
		return nil, false
	}
	if len(rule.catalog.rows) == 0 {
		return nil, true
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
		return nil, false
	}
	member, ok := graph.MountedActivationMember(capability, mountID, reusablePointID, occurrenceID)
	if !ok {
		return nil, false
	}
	implementation, ok := callowner.ResolveActivationRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return nil, false
	}
	return engine.AttachReceiptActivationMember(compilation, implementation, member)
}

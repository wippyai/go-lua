package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
)

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

// occurrenceOperands is the one redemption of a sealed mounted activation
// row. Call's own occurrence inverse names the row; the seal already proved
// its key ownership and module residence, so no mount-scoped issuer stands
// between and no fence is retested per accessor.
func (rule *HotRule) occurrenceOperands(mount, occurrence identity.ContentID) (calldomain.Key, identity.SemanticKey, bool) {
	if rule == nil || !rule.receiptsSealed || rule.owner == nil || rule.owner.Algebra() == nil {
		return calldomain.Key{}, identity.SemanticKey{}, false
	}
	algebra := rule.owner.Algebra()
	mounted, mountedOK := algebra.MountedCallForOccurrence(mount, occurrence)
	applicationID, _, _, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	key, keyOK := algebra.KeyForApplicationID(applicationID)
	application, applicationOK := identity.NewSemanticKey([32]byte(applicationID), 1)
	return key, application, mountedOK && identityOK && keyOK && applicationOK && application.Available()
}

// MountedAdmit is the sealed activation admission request. The construction
// plane holds the assembly; this owner supplies only declaration-owned rows.
func (rule *HotRule) MountedAdmit(mountID, reusablePointID, occurrenceID identity.ContentID) (engine.MountedActivationAdmit, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return engine.MountedActivationAdmit{}, false
	}
	key, application, operandOK := rule.occurrenceOperands(mountID, occurrenceID)
	if !operandOK || rule.catalog == nil || !rule.catalog.valid() {
		return engine.MountedActivationAdmit{}, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	implementation, implementationOK := callowner.ResolveActivationRuleImplementationFor(rule.owner, rule.implementation)
	if !capabilityOK || !implementationOK {
		return engine.MountedActivationAdmit{}, false
	}
	if rule.transport == nil {
		return engine.MountedActivationAdmit{}, false
	}
	ref, refOK := rule.owner.Ref(key)
	read, readOK := engine.ExactReadSurface(ref)
	if !refOK || !readOK {
		return engine.MountedActivationAdmit{}, false
	}
	candidates := make([]engine.MountedActivationCandidate, len(rule.catalog.rows))
	for index, row := range rule.catalog.rows {
		candidates[index] = engine.MountedActivationCandidate{
			Target: row.target, Endpoint: row.endpoint, Mount: row.moduleKey, Body: row.bodyPath,
		}
	}
	return engine.MountedActivationAdmit{
		Implementation: implementation,
		Transport:      rule.transport,
		Capability:     capability,
		Mount:          mountID,
		Point:          reusablePointID,
		Occurrence:     occurrenceID,
		Application:    application,
		Read:           read,
		Candidates:     candidates,
	}, true
}

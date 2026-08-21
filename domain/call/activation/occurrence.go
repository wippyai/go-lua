package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// MountedAdmit is the sealed activation admission request. The construction
// plane holds the assembly; this owner supplies only declaration-owned rows.
func (rule *HotRule) MountedAdmit(mountID, reusablePointID, occurrenceID identity.ContentID) (engine.MountedActivationAdmit, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return engine.MountedActivationAdmit{}, false
	}
	// Call's Algebra is the canonical owner of mounted occurrence rows. Its
	// sealed constructor validates the inverse, application key, and module
	// residence; activation consumes that row directly without an intermediate
	// projection or a second sealing lifecycle.
	algebra := rule.owner.Algebra()
	if algebra == nil {
		return engine.MountedActivationAdmit{}, false
	}
	mounted, mountedOK := algebra.MountedCallForOccurrence(mountID, occurrenceID)
	applicationID, _, _, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	key, keyOK := algebra.KeyForApplicationID(applicationID)
	application, applicationOK := identity.NewSemanticKey([32]byte(applicationID), 1)
	operandOK := mountedOK && identityOK && keyOK && applicationOK && application.Available()
	if !operandOK || rule.catalog == nil || !rule.catalog.valid() {
		return engine.MountedActivationAdmit{}, false
	}
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !capabilityOK {
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
		Transport:   rule.transport,
		Capability:  capability,
		Mount:       mountID,
		Point:       reusablePointID,
		Occurrence:  occurrenceID,
		Application: application,
		Read:        read,
		Candidates:  candidates,
	}, true
}

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
	_, key, mountedOK := algebra.MountedCallKeyForOccurrence(mountID, occurrenceID)
	applicationID, applicationIDOK := key.ApplicationID()
	application, applicationOK := identity.NewSemanticKey([32]byte(applicationID), 1)
	operandOK := mountedOK && applicationIDOK && applicationOK && application.Available()
	if !operandOK || !rule.routesValid() {
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
	bodies := algebra.Bodies()
	candidates := make([]engine.MountedActivationCandidate, bodies.Count())
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		moduleKey, moduleOK := body.ModuleKey()
		bodyPath, pathOK := body.BodyPath()
		item, routeOK := rule.routeAt(index)
		if !bodyOK || !moduleOK || !pathOK || !routeOK {
			return engine.MountedActivationAdmit{}, false
		}
		candidates[index] = engine.MountedActivationCandidate{
			Target: item.target, Endpoint: item.endpoint, Mount: moduleKey, Body: bodyPath,
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

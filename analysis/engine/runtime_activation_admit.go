package engine

import "github.com/wippyai/go-lua/analysis/identity"

// MountedActivationCandidate is one sealed body route the activation
// occurrence must instantiate.
type MountedActivationCandidate struct {
	Target   identity.SemanticKey
	Endpoint identity.SemanticKey
	Mount    identity.ContentID
	Body     identity.ContentID
}

// MountedActivationAdmit is the declared issuance of one mounted activation
// occurrence. Every field is a value the owner sealed: Read is the exact
// owner-issued read surface the trigger row places, and Candidates are the
// body routes it must instantiate. The topology builder stays in the engine.
type MountedActivationAdmit struct {
	Implementation *ActivationRuleImplementation
	Transport      *MountedActivationCandidateIssuer
	Capability     RuleSlotCapability
	Mount          identity.ContentID
	Point          identity.ContentID
	Occurrence     identity.ContentID
	Application    identity.SemanticKey
	Read           RuleReadSurface
	Candidates     []MountedActivationCandidate
}

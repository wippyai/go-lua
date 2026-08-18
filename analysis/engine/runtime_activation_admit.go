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

// MountedActivationAdmit is the construction-plane admission request for
// one mounted activation occurrence. The assembly stays in the engine.
type MountedActivationAdmit struct {
	Implementation *ActivationRuleImplementation
	Transport      *MountedActivationCandidateIssuer
	Capability     RuleSlotCapability
	Mount          identity.ContentID
	Point          identity.ContentID
	Occurrence     identity.ContentID
	Application    identity.SemanticKey
	PlaceRead      func(*RuleSourceTransaction) bool
	Candidates     []MountedActivationCandidate
}

// ExactReadPlacer is the construction-plane exact-read admission for one
// owner-issued source key.
func ExactReadPlacer[K ~uint32 | ~uint64](ref Ref[K]) func(*RuleSourceTransaction) bool {
	return func(transaction *RuleSourceTransaction) bool {
		return AddExactRead(transaction, ref)
	}
}

// AdmitMountedActivationOccurrence admits one activation occurrence while
// assembly sources remain open. Empty candidate sets are a successful no-op.
func AdmitMountedActivationOccurrence(assembly *ReceiptAssembly, admit MountedActivationAdmit) bool {
	if assembly == nil || admit.Implementation == nil || !admit.Capability.Mounted() ||
		!admit.Mount.Available() || !admit.Point.Available() || !admit.Occurrence.Available() {
		return false
	}
	if len(admit.Candidates) == 0 {
		return true
	}
	if admit.Transport == nil || admit.PlaceRead == nil || !admit.Application.Available() {
		return false
	}
	occurrence, ok := assembly.AdmitMountedRuleOccurrence(admit.Capability, admit.Mount, admit.Point, admit.Occurrence)
	if !ok {
		return false
	}
	transaction, ok := BeginMountedActivationRuleAdmission(assembly, admit.Implementation, occurrence, [32]byte(admit.Occurrence))
	if !ok || !admit.PlaceRead(transaction) {
		return false
	}
	return assembly.QueueMountedRuleFinalizer(admit.Capability, func() bool {
		source, sourceOK := transaction.Seal()
		if !sourceOK {
			return false
		}
		draft, draftOK := admit.Implementation.BeginReceiptRuleRow(source)
		readPart, readPartOK := admit.Implementation.ReceiptReadPart(source, 0)
		if !draftOK || !readPartOK || !draft.AddRead(readPart) {
			return false
		}
		if !assembly.AddActivationRuleFromDraft(occurrence, draft) {
			return false
		}
		for _, candidate := range admit.Candidates {
			if !admit.Transport.AddMountedActivationCandidate(assembly, occurrence, admit.Application, candidate.Target, candidate.Endpoint, candidate.Mount, candidate.Body) {
				return false
			}
		}
		return admit.Transport.CompleteMountedActivationCandidates(assembly, occurrence, admit.Application, uint64(len(admit.Candidates)))
	})
}

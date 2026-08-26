package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// ProgramActivation is one committed activation trigger: the occurrence the
// construction mounted an activation member for, and the body routes that
// trigger declared. It is the enumeration counterpart of ActivationMember,
// which resolves one trigger a caller already holds the identity of.
//
// The row is a read of the committed program, not a second authority over it.
// Every field was stated by the declaration this program was constructed from
// and is republished here unchanged; the candidate routes in particular are
// the exact rows the issuance sealed, so a consumer reads the trigger's
// declared route set rather than re-deriving one from graph geometry.
type ProgramActivation struct {
	program   *CommittedProgram
	binding   programActivationBinding
	available bool
}

// programActivationBinding is one activation trigger retained by the committed
// program in declaration order. It is appended only for a member row the
// declaration marked as an activation, so a program that declares no trigger
// retains nothing.
type programActivationBinding struct {
	member      identity.ContentID
	activation  identity.ContentID
	mount       identity.ContentID
	point       identity.ContentID
	occurrence  identity.ContentID
	application composition.Key
	candidates  []MountedActivationCandidate
}

// Available reports whether this row addresses a committed trigger.
func (row ProgramActivation) Available() bool {
	return row.available && row.program.valid()
}

// Member is the graph member the construction mounted for this trigger.
func (row ProgramActivation) Member() (ActivationMember, bool) {
	if !row.Available() {
		return ActivationMember{}, false
	}
	return row.program.lookupActivationMember(row.binding.activation)
}

// Mount, Point, and Occurrence are the mounted coordinates the trigger was
// placed at. Mount is the module the trigger itself runs in, which is the
// module every candidate route departs.
func (row ProgramActivation) Mount() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.binding.mount
}

func (row ProgramActivation) Point() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.binding.point
}

func (row ProgramActivation) Occurrence() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.binding.occurrence
}

// Application is the identity every candidate of this trigger is an
// alternative of. A trigger that reaches no body still states it.
func (row ProgramActivation) Application() (identity.SemanticKey, bool) {
	if !row.Available() {
		return identity.SemanticKey{}, false
	}
	return semanticKeyFromComposition(row.binding.application)
}

// CandidateCount reports how many body routes this trigger declared.
func (row ProgramActivation) CandidateCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.binding.candidates)
}

// CandidateAt returns one declared body route in declaration order.
func (row ProgramActivation) CandidateAt(index int) (MountedActivationCandidate, bool) {
	if !row.Available() || index < 0 || index >= len(row.binding.candidates) {
		return MountedActivationCandidate{}, false
	}
	return row.binding.candidates[index], true
}

// Candidates returns a detached copy of this trigger's declared body routes.
func (row ProgramActivation) Candidates() []MountedActivationCandidate {
	if !row.Available() {
		return nil
	}
	return append([]MountedActivationCandidate(nil), row.binding.candidates...)
}

// CandidateTransition resolves one candidate's declared execution-context edge
// against the Link directory the program was committed under. The tuple is
// authenticated here rather than left for a consumer to join, so a route the
// directory does not hold is not published as one it does.
func (row ProgramActivation) CandidateTransition(index int) (executioncontext.Transition, bool) {
	candidate, candidateOK := row.CandidateAt(index)
	if !candidateOK || !row.program.contexts.Available() {
		return executioncontext.Transition{}, false
	}
	transition, transitionOK := row.program.contexts.Transition(candidate.FromContextID, candidate.ToContextID)
	if !transitionOK || transition.ID() != candidate.TransitionID {
		return executioncontext.Transition{}, false
	}
	return transition, true
}

// ActivationCount reports the number of committed activation triggers.
func (committed *CommittedProgram) ActivationCount() int {
	if committed == nil || !committed.valid() {
		return 0
	}
	return len(committed.activations)
}

// ActivationAt returns one committed activation trigger in declaration order.
func (committed *CommittedProgram) ActivationAt(index int) (ProgramActivation, bool) {
	if committed == nil || !committed.valid() || index < 0 || index >= len(committed.activations) {
		return ProgramActivation{}, false
	}
	return ProgramActivation{program: committed, binding: committed.activations[index], available: true}, true
}

// constructProgramActivations folds the declaration's activation member rows
// and its candidate rows into one enumeration, in declaration order.
//
// The two are stated by the same issuance and are joined here by the trigger
// member identity they both name, so a candidate whose trigger the declaration
// did not mark as an activation, and a trigger that lost its coordinates,
// refuse the construction rather than publish a partial enumeration.
func constructProgramActivations(declaration topologyDeclaration) ([]programActivationBinding, bool) {
	triggers := 0
	for _, member := range declaration.members {
		if member.Activation {
			triggers++
		}
	}
	if triggers == 0 {
		return nil, len(declaration.candidates) == 0
	}
	order := make(map[identity.ContentID]int, triggers)
	bindings := make([]programActivationBinding, 0, triggers)
	for _, member := range declaration.members {
		if !member.Activation {
			continue
		}
		if !member.ID.Available() || !member.ActivationID.Available() || !member.Mount.Available() ||
			!member.Point.Available() || !member.Occurrence.Available() || !member.Application.Available() {
			return nil, false
		}
		if _, duplicate := order[member.ID]; duplicate {
			return nil, false
		}
		order[member.ID] = len(bindings)
		bindings = append(bindings, programActivationBinding{
			member: member.ID, activation: member.ActivationID, mount: member.Mount,
			point: member.Point, occurrence: member.Occurrence, application: member.Application,
		})
	}
	for _, candidate := range declaration.candidates {
		position, declared := order[candidate.Member]
		target, targetOK := semanticKeyFromComposition(candidate.Target)
		endpoint, endpointOK := semanticKeyFromComposition(candidate.Endpoint)
		if !declared || !targetOK || !endpointOK || !candidate.Context.Available() {
			return nil, false
		}
		bindings[position].candidates = append(bindings[position].candidates, MountedActivationCandidate{
			Target:        target,
			Endpoint:      endpoint,
			Mount:         candidate.Mount,
			Body:          candidate.Body,
			TransitionID:  candidate.Context.TransitionID,
			FromContextID: candidate.Context.FromContextID,
			ToContextID:   candidate.Context.ToContextID,
		})
	}
	return bindings, true
}

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// An activation candidate declares the execution-context edge its body route
// runs on, and the program authenticates that tuple against the Link's sealed
// directory rather than reconstructing it. A body in the trigger's own module
// rides the canonical reflexive local edge the directory issues for every
// Context, so the complete tuple is admitted. Departures refuse at the exact
// boundary that owns them: a half-filled tuple fails the declaration's own
// well-formedness law before construction sees it, and a well-formed tuple
// that names no edge or names rows outside this directory fails at the candidate
// row - an empty tuple included, since a candidate that names no edge is a
// candidate no directory authenticated. Either way an unauthenticated tuple
// never places an activation on a context no owner declared.
func TestActivationCandidateContextIsAuthenticatedAgainstTheDirectory(t *testing.T) {
	fixture := newSelectedOverlayLawFixtureWithOptions(t, selectedOverlayLawOptions{candidateCount: 1})
	if fixture.graph == nil || fixture.solver == nil {
		t.Fatal("the canonical reflexive local edge was not admitted")
	}

	foreign := func(t testing.TB) executioncontext.Directory {
		t.Helper()
		return explicitTestContextDirectory(t,
			foreignActivationContextID(1), []identity.ContentID{foreignActivationContextID(2)},
			foreignActivationContextID(3), foreignActivationContextID(4))
	}
	cases := []struct {
		name string
		// reachesCandidateRow states which boundary owns the refusal. An
		// empty tuple and a complete one are both well formed, so they reach
		// construction; a half-filled tuple fails the declaration first.
		reachesCandidateRow bool
		rewrite             func(executioncontext.Directory, MountedActivationCandidate) MountedActivationCandidate
	}{
		{"absent tuple", true, func(_ executioncontext.Directory, candidate MountedActivationCandidate) MountedActivationCandidate {
			candidate.TransitionID, candidate.FromContextID, candidate.ToContextID = identity.ContentID{}, identity.ContentID{}, identity.ContentID{}
			return candidate
		}},
		{"transition without endpoints", false, func(_ executioncontext.Directory, candidate MountedActivationCandidate) MountedActivationCandidate {
			candidate.FromContextID, candidate.ToContextID = identity.ContentID{}, identity.ContentID{}
			return candidate
		}},
		{"endpoints without transition", false, func(_ executioncontext.Directory, candidate MountedActivationCandidate) MountedActivationCandidate {
			candidate.TransitionID = identity.ContentID{}
			return candidate
		}},
		{"foreign endpoints", true, func(_ executioncontext.Directory, candidate MountedActivationCandidate) MountedActivationCandidate {
			other := foreign(t)
			context := explicitTestContext(t, other, foreignActivationContextID(2))
			transition, ok := other.Transition(context.ID(), context.ID())
			if !ok {
				t.Fatal("foreign local execution edge")
			}
			candidate.TransitionID, candidate.FromContextID, candidate.ToContextID = transition.ID(), context.ID(), context.ID()
			return candidate
		}},
		{"reflexive pair with a foreign transition identity", true, func(_ executioncontext.Directory, candidate MountedActivationCandidate) MountedActivationCandidate {
			other := foreign(t)
			context := explicitTestContext(t, other, foreignActivationContextID(2))
			transition, ok := other.Transition(context.ID(), context.ID())
			if !ok {
				t.Fatal("foreign local execution edge")
			}
			candidate.TransitionID = transition.ID()
			return candidate
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			refused := newSelectedOverlayLawFixtureWithOptions(t, selectedOverlayLawOptions{
				candidateCount: 1, candidateContext: testCase.rewrite, admitConstructionRefusal: true,
			})
			if refused.constructed {
				t.Fatal("an unauthenticated execution-context tuple reached a constructed program")
			}
			step := refused.constructionRefusal.construction.Step()
			if !testCase.reachesCandidateRow {
				if refused.constructionRefusal.Stage() != ProgramAdmissionSeal || step != topologyConstructionStepNone {
					t.Fatalf("a half-filled tuple refused at stage %v step %v, want the admission seal before construction",
						refused.constructionRefusal.Stage(), step)
				}
				return
			}
			if step != topologyConstructionStepCandidateRow {
				t.Fatalf("refused at construction step %v, want the candidate row", step)
			}
		})
	}
}

func foreignActivationContextID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0], id[1] = 0xf7, value
	return id
}

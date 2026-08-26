package obligation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/domain/typestate/statecell"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// An alternative this analysis cannot follow does not let the alternatives it
// can follow certify the call.
//
// The site is a governed one: at the state the declaration observes, the
// followable target conforms and the fold answers a proved fact. Adding the
// unfollowable alternative to that same dispatch discharges every proof about
// the resource and reports the unproven finding, and it leaves the row where
// it was. Dropping the row would report the call clean by omission, and
// carrying the conformance would certify a callee that was never read: both
// are answers a soundness judgment may not give, and this law excludes both.
func TestUnfollowableAlternativeAtAGovernedSiteIsJudgedAndKept(t *testing.T) {
	fixture := buildJudgmentFixture(t, resourceSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	space, spaceOK := statecell.Seal(fixture.values.LinkID(), fixture.values.Heap().AllocationKeyCount(), 4)
	if !spaceOK {
		t.Fatal("cell space unavailable")
	}
	resource := resourceFact(t, fixture)
	judged := 0
	for index := 0; index < fixture.values.MountedCallArgumentCount(); index++ {
		candidate, candidateOK := fixture.values.MountedCallArgumentAt(index)
		if !candidateOK {
			t.Fatalf("mounted call actual %d unavailable", index)
		}
		actual, actualOK := candidate.ActualIndex()
		if !actualOK {
			t.Fatalf("mounted call actual %d carries no position", index)
		}
		site, siteOK := judgment.DeriveCallSite(candidate)
		if !siteOK {
			continue
		}
		key, keyOK := site.Key()
		if !keyOK {
			t.Fatalf("actual %d resolved a site with no read key", index)
		}
		for support := 0; support < fixture.calls.SupportCount(key); support++ {
			target, targetOK := fixture.calls.SupportTargetAt(key, support)
			if !targetOK {
				t.Fatalf("actual %d support %d unavailable", index, support)
			}
			operation, kind := fixture.calls.ClassifyTargetOperation(target)
			if kind != calldomain.TargetOperationPresent {
				continue
			}
			for protocol := vocabulary.Protocol(1); protocol < 64; protocol++ {
				if _, definitionOK := judgment.sealed.definitionFor(protocol); !definitionOK {
					continue
				}
				declared, declaredOK := judgment.sealed.obligationAt(protocol, operation, actual)
				if !declaredOK || declared.kind == obligationEscape {
					continue
				}
				observed, observedOK := typestate.Exactly(declared.observed)
				if !observedOK {
					t.Fatalf("declared state %q is not a state", declared.observed)
				}
				followable, followableOK := fixture.calls.DispatchValue(key, []calldomain.Target{target}, false)
				if !followableOK {
					t.Fatalf("actual %d support %d dispatches no fact", index, support)
				}
				// A site whose callee the Call algebra proved closed carries
				// no unfollowable alternative to add, so it states nothing
				// about this law.
				unfollowable, unfollowableOK := fixture.calls.DispatchValue(key, []calldomain.Target{target}, true)
				if !unfollowableOK {
					continue
				}
				judged++
				assertUnfollowableAlternativeIsJudged(t, judgment, fixture, space, candidate, resource,
					followable, unfollowable, protocol, observed, declared)
			}
		}
	}
	if judged == 0 {
		t.Fatal("no governed actual of the declared-lifecycle fixture carries an unfollowable alternative, so this law proves nothing")
	}
}

// assertUnfollowableAlternativeIsJudged holds one governed site to the two
// answers the added alternative may not change: the verdict stops being a
// certification, and the cells the actual reads stay the cells it read.
func assertUnfollowableAlternativeIsJudged(
	t *testing.T,
	judgment Judgment,
	fixture judgmentFixture,
	space statecell.Space,
	candidate valuedomain.MountedCallArgument,
	resource valuedomain.Value,
	followable calldomain.Value,
	unfollowable calldomain.Value,
	protocol vocabulary.Protocol,
	observed typestate.Abstract,
	declared edge,
) {
	t.Helper()
	proved, conforms, provedOutcome := judgment.decide(candidate, fixture.values.Top(), followable, uint64(protocol), observed)
	if conforms != typestate.VerdictConforms || provedOutcome != structure.Concrete {
		t.Fatalf("at its declared state the followable target draws %q with outcome %d, want a proved conformance",
			conforms.Spelling(), provedOutcome)
	}
	if declared.kind == obligationRequirement && !proved.Proves(declared.observed) {
		t.Fatalf("a requirement moved the resource out of %q", declared.observed)
	}

	successor, verdict, outcome := judgment.decide(candidate, fixture.values.Top(), unfollowable, uint64(protocol), observed)
	if outcome == structure.Refuse {
		t.Fatal("an unfollowable alternative removed the actual from the population")
	}
	if outcome != structure.AuthenticatedOpaque {
		t.Fatalf("outcome = %d, want the authenticated-opaque admission the read delivered", outcome)
	}
	if !successor.IsUnknown() {
		t.Fatalf("successor = %+v, want every proof about the resource discharged", successor)
	}
	if verdict == typestate.VerdictConforms {
		t.Fatal("a callee the analysis could not read certified the call")
	}
	if !verdict.Reports() {
		t.Fatalf("verdict = %q, want a reported unproven finding", verdict.Spelling())
	}
	if verdict != typestate.VerdictUnprovenRequirement && verdict != typestate.VerdictUnprovenTransition {
		t.Fatalf("verdict = %q, want one of the two unproven answers", verdict.Spelling())
	}

	held, heldOK := judgment.DeriveStateCells(fixture.values.Heap(), space, candidate, resource, followable)
	kept, keptOK := judgment.DeriveStateCells(fixture.values.Heap(), space, candidate, resource, unfollowable)
	if !heldOK || !keptOK {
		t.Fatal("a governed actual was refused a cell plan")
	}
	if kept.Count() != held.Count() {
		t.Fatalf("cell rows = %d with the unfollowable alternative and %d without it", kept.Count(), held.Count())
	}
	for row := 0; row < held.Count(); row++ {
		expected, expectedOK := held.At(row)
		actual, actualOK := kept.At(row)
		if !expectedOK || !actualOK || expected != actual {
			t.Fatalf("cell row %d moved when the unfollowable alternative was added", row)
		}
	}
}

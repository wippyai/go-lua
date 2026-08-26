package obligation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/typestate"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// resourceSource calls the declared-lifecycle host surface: it acquires a
// connection, reads it without moving it, and moves it to its final state.
// Each of those is a member the manifest states an obligation for, so the
// judgment below is exercised against real declared edges.
const resourceSource = "local resource = require(\"resource\")\n" +
	"local connection = resource.connect()\n" +
	"resource.query(connection)\n" +
	"resource.close(connection)\n"

// The arm a call is judged by is the arm its own declaration states.
//
// This is the whole of the selection: the site's Call fact says which
// operation is reached, the sealed authority says what that operation declares
// about this actual, and the judgment applies the requirement, the transition
// or the escape accordingly. Nothing carries the choice to the fold, and no
// arm is reachable that the declaration did not state.
func TestDeclaredObligationDecidesTheArmTheJudgmentApplies(t *testing.T) {
	fixture := buildJudgmentFixture(t, resourceSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	governed := 0
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
			dispatched, dispatchedOK := fixture.calls.DispatchValue(key, []calldomain.Target{target}, false)
			if !dispatchedOK {
				t.Fatalf("actual %d support %d dispatches no fact", index, support)
			}
			for protocol := vocabulary.Protocol(1); protocol < 64; protocol++ {
				definition, definitionOK := judgment.sealed.definitionFor(protocol)
				if !definitionOK {
					continue
				}
				declared, declaredOK := judgment.sealed.obligationAt(protocol, operation, actual)
				if !declaredOK {
					continue
				}
				governed++
				assertDeclaredArm(t, judgment, definition, declared, protocol, candidate, fixture, dispatched)
			}
		}
	}
	if governed == 0 {
		t.Fatal("no actual of the declared-lifecycle fixture carries an obligation, so this law proves nothing")
	}
}

// assertDeclaredArm holds one declared obligation to the two answers it has:
// at the state it observes the call conforms and the successor is the one the
// declaration states, and at a state it excludes the call is reported.
func assertDeclaredArm(
	t *testing.T,
	judgment Judgment,
	definition typestate.Definition,
	declared edge,
	protocol vocabulary.Protocol,
	subject valuedomain.MountedCallArgument,
	fixture judgmentFixture,
	dispatched calldomain.Value,
) {
	t.Helper()
	if declared.kind == obligationEscape {
		successor, _, outcome := judgment.decide(subject, fixture.values.Top(), dispatched, uint64(protocol), typestate.Unknown())
		if !successor.IsUnknown() || outcome != structure.AuthenticatedOpaque {
			t.Fatalf("a declared escape left a proof standing: successor=%+v outcome=%d", successor, outcome)
		}
		return
	}
	observed, observedOK := typestate.Exactly(declared.observed)
	if !observedOK {
		t.Fatalf("declared state %q is not a state", declared.observed)
	}
	successor, verdict, outcome := judgment.decide(subject, fixture.values.Top(), dispatched, uint64(protocol), observed)
	if verdict != typestate.VerdictConforms {
		t.Fatalf("at the declared state %q the call draws %q, want conformance", declared.observed, verdict.Spelling())
	}
	if outcome != structure.Concrete {
		t.Fatalf("at the declared state the outcome is %d, want a proved fact", outcome)
	}
	switch declared.kind {
	case obligationRequirement:
		if !successor.Proves(declared.observed) {
			t.Fatalf("a requirement moved the resource out of %q", declared.observed)
		}
	case obligationTransition:
		if len(declared.arrivals) == 1 && !successor.Proves(declared.arrivals[0]) {
			t.Fatalf("a transition did not complete to its declared arm %q", declared.arrivals[0])
		}
	}
	for _, state := range definition.States {
		if state == declared.observed {
			continue
		}
		excluded, excludedOK := typestate.Exactly(state)
		if !excludedOK {
			t.Fatalf("declared state %q is not a state", state)
		}
		_, refuted, _ := judgment.decide(subject, fixture.values.Top(), dispatched, uint64(protocol), excluded)
		if !refuted.Reports() {
			t.Fatalf("at %q the declaration is violated and the call drew %q", state, refuted.Spelling())
		}
		return
	}
}

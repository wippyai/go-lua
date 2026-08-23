package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func activationLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func activationLawFactor(t *testing.T, seed string) composition.Key {
	t.Helper()
	id, ok := identity.DeriveContentID("go-lua/activation-form-law/factor", []byte(seed))
	if !ok {
		t.Fatal("activation law factor identity")
	}
	return composition.Key{ID: composition.ID(id), Version: 1}
}

func activationLawSpec(t *testing.T, outcome structure.ReductionOutcome) ActivationSpec {
	t.Helper()
	return ActivationSpec{
		TransitionID:  activationLawID(1),
		FromContextID: activationLawID(2),
		ToContextID:   activationLawID(3),
		FromContext:   contextfiber.ContextOrdinal(4),
		ToContext:     contextfiber.ContextOrdinal(5),
		SourcePoint:   contextfiber.PointOrdinal(6),
		TargetPoint:   contextfiber.PointOrdinal(7),
		SourceState:   contextfiber.StateOrdinal(8),
		TargetState:   contextfiber.StateOrdinal(9),
		Port:          activationLawFactor(t, "value"),
		Outcome:       outcome,
	}
}

// TestASealedActivationBranchAnswersEveryCoordinateItWasAuthenticatedWith is
// the whole point of the sealed row: the endpoint Context assignment and the
// State pair are decided once, when the branch is authenticated, and every
// later consumer reads them off the row. A consumer that has the row never
// needs the directory, the point layout, or the execution plan again.
func TestASealedActivationBranchAnswersEveryCoordinateItWasAuthenticatedWith(t *testing.T) {
	row, sealed := NewActivationRow(activationLawSpec(t, structure.Concrete))
	if !sealed || !row.Available() {
		t.Fatal("a complete authenticated tuple seals one activation branch")
	}
	transition, from, to := row.Transition()
	if transition != activationLawID(1) || from != activationLawID(2) || to != activationLawID(3) {
		t.Fatal("the branch carries the transition it was authenticated against")
	}
	fromContext, toContext := row.Contexts()
	if fromContext != contextfiber.ContextOrdinal(4) || toContext != contextfiber.ContextOrdinal(5) {
		t.Fatal("the branch carries its settled endpoint Context assignment")
	}
	sourcePoint, targetPoint := row.Points()
	sourceState, targetState := row.States()
	if sourcePoint != contextfiber.PointOrdinal(6) || targetPoint != contextfiber.PointOrdinal(7) ||
		sourceState != contextfiber.StateOrdinal(8) || targetState != contextfiber.StateOrdinal(9) {
		t.Fatal("the branch carries the State pair its endpoints occupy")
	}
	if row.Port() != activationLawFactor(t, "value") {
		t.Fatal("the branch names the transport port it instantiates")
	}
}

// TestAnIncompleteActivationTupleSealsNoBranch states that the row is an
// authentication receipt and not a struct literal: a missing transition, a
// missing endpoint identity, an absent port, or an undeclared outcome leaves
// the branch unsealed rather than half-authenticated.
func TestAnIncompleteActivationTupleSealsNoBranch(t *testing.T) {
	cases := map[string]func(*ActivationSpec){
		"transition":   func(spec *ActivationSpec) { spec.TransitionID = identity.ContentID{} },
		"from-context": func(spec *ActivationSpec) { spec.FromContextID = identity.ContentID{} },
		"to-context":   func(spec *ActivationSpec) { spec.ToContextID = identity.ContentID{} },
		"port":         func(spec *ActivationSpec) { spec.Port = composition.Key{} },
		"outcome":      func(spec *ActivationSpec) { spec.Outcome = structure.ReductionOutcome(9) },
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			spec := activationLawSpec(t, structure.Concrete)
			damage(&spec)
			if row, sealed := NewActivationRow(spec); sealed || row.Available() {
				t.Fatalf("an activation branch sealed without its %s", name)
			}
		})
	}
}

// TestABranchSettlesExactlyOneOutcome is FG-6 law (c) at the executing end:
// the branch column is five-valued, every sealed branch carries exactly one of
// the five, and the value it carries is the value it was authenticated with.
func TestABranchSettlesExactlyOneOutcome(t *testing.T) {
	for _, outcome := range []structure.ReductionOutcome{
		structure.Refuse, structure.NoSelection, structure.NoCandidate,
		structure.Concrete, structure.AuthenticatedOpaque,
	} {
		row, sealed := NewActivationRow(activationLawSpec(t, outcome))
		if !sealed {
			t.Fatalf("outcome %d is a settleable branch disposition", outcome)
		}
		if settled := row.Outcome(); settled != outcome {
			t.Fatalf("branch settled %d, was authenticated with %d", settled, outcome)
		}
	}
}

// TestACandidateFreeTriggerSettlesNoSelection is the A form's own producer
// obligation. A trigger whose admitted bodies exist but whose activation
// relation admits no route between them is an explicitly empty selection over
// a population that exists - which is NoSelection and not an absent subject,
// a refusal, or a silent success.
func TestACandidateFreeTriggerSettlesNoSelection(t *testing.T) {
	empty, sealed := NewActivationBranches(nil)
	if !sealed {
		t.Fatal("a candidate-free trigger is a sealed branch set, not a malformed one")
	}
	if empty.Count() != 0 || empty.Outcome() != structure.NoSelection {
		t.Fatalf("candidate-free trigger settled %d over %d branches", empty.Outcome(), empty.Count())
	}
}

// TestATriggerSettlesTheDispositionItsBranchesJustify states the trigger fold
// over its branches. Refusal is fatal to the trigger, one instantiated
// transport makes it concrete, an authenticated admission of unknowing
// survives when nothing was instantiated, and branches that all declined leave
// the selection empty.
func TestATriggerSettlesTheDispositionItsBranchesJustify(t *testing.T) {
	branch := func(outcome structure.ReductionOutcome) ActivationRow {
		row, _ := NewActivationRow(activationLawSpec(t, outcome))
		return row
	}
	cases := []struct {
		name    string
		rows    []ActivationRow
		settled structure.ReductionOutcome
	}{
		{"refusal is fatal", []ActivationRow{branch(structure.Concrete), branch(structure.Refuse)}, structure.Refuse},
		{"one transport is concrete", []ActivationRow{branch(structure.NoCandidate), branch(structure.Concrete)}, structure.Concrete},
		{"unknowing survives", []ActivationRow{branch(structure.NoCandidate), branch(structure.AuthenticatedOpaque)}, structure.AuthenticatedOpaque},
		{"all declined", []ActivationRow{branch(structure.NoCandidate), branch(structure.NoCandidate)}, structure.NoCandidate},
		{"an existing population outranks an absent subject", []ActivationRow{branch(structure.NoCandidate), branch(structure.NoSelection)}, structure.NoSelection},
		{"declined selection", []ActivationRow{branch(structure.NoSelection)}, structure.NoSelection},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			branches, sealed := NewActivationBranches(testCase.rows)
			if !sealed {
				t.Fatal("a branch set of sealed rows seals")
			}
			if got := branches.Outcome(); got != testCase.settled {
				t.Fatalf("trigger settled %d, want %d", got, testCase.settled)
			}
		})
	}
}

// TestATriggerSettlesTheSameDispositionUnderAnyBranchOrder states that the
// trigger fold is a property of the branch set and not of the order the
// branches were discovered in. A set is unordered evidence; a disposition that
// moved with the discovery order would make the trigger's answer depend on the
// solver's traversal.
func TestATriggerSettlesTheSameDispositionUnderAnyBranchOrder(t *testing.T) {
	branch := func(outcome structure.ReductionOutcome) ActivationRow {
		row, _ := NewActivationRow(activationLawSpec(t, outcome))
		return row
	}
	pairs := [][2]structure.ReductionOutcome{
		{structure.NoCandidate, structure.NoSelection},
		{structure.NoCandidate, structure.AuthenticatedOpaque},
		{structure.NoSelection, structure.Concrete},
		{structure.AuthenticatedOpaque, structure.Concrete},
		{structure.Concrete, structure.Refuse},
	}
	for _, pair := range pairs {
		forward, forwardOK := NewActivationBranches([]ActivationRow{branch(pair[0]), branch(pair[1])})
		reverse, reverseOK := NewActivationBranches([]ActivationRow{branch(pair[1]), branch(pair[0])})
		if !forwardOK || !reverseOK {
			t.Fatal("a branch set of sealed rows seals in either order")
		}
		if forward.Outcome() != reverse.Outcome() {
			t.Fatalf("branches %d/%d settled %d forward and %d reversed", pair[0], pair[1], forward.Outcome(), reverse.Outcome())
		}
	}
}

// TestAnUnsealedBranchIsNotAdmittedToATriggersBranchSet keeps the trigger fold
// honest: a set is a set of authenticated branches, so one unsealed row makes
// the whole set unsealed rather than contributing a zero disposition.
func TestAnUnsealedBranchIsNotAdmittedToATriggersBranchSet(t *testing.T) {
	if _, sealed := NewActivationBranches([]ActivationRow{{}}); sealed {
		t.Fatal("an unsealed branch was admitted to a trigger's branch set")
	}
}

package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/identity"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
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

// activationFormLawSpec is the sealed call-activation shape: one exact read of
// the trigger candidate, one vector read of the branch set hanging off that
// same candidate row, and one structural output publishing a non-empty
// transport vector.
//
// The branch read is a parent-declaring Summary and not a selection. A branch
// is COLD - the construct topology mounts one activation member per branch
// before any solve, and execution only settles the disposition of branches
// already mounted - while a selection's coordinates are, in this package's own
// words, "the members of a relation that exists only per invocation". A
// structural row drawn from one would publish branches nothing had mounted.
func activationFormLawSpec() generated.CompiledRuleSpec {
	return generated.CompiledRuleSpec{
		AxisCount:  3,
		InputCount: 2,
		Candidate:  ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:    ruleplan.ReducerAddr{Axis: 0, Member: 0},
		Reads: []generated.ReadPlan{
			{
				Input: 0, Factor: 1, Axis: 1,
				Sources:    ruleplan.Span{Start: 0, Count: 1},
				Relation:   ruleplan.RelationAddr{Axis: 1, Member: 4},
				Key:        ruleplan.ProjectionAddr{Axis: 1, Member: 6},
				Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
				Form:       ruleprogram.Exact,
				PointBound: ruleprogram.PointBound,
				Contract: ruleplan.ReadContract{
					Order:        ruleprogram.OrderCanonical,
					Sparse:       ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaqueRefuse,
					Multiplicity: ruleprogram.MultiplicityOne,
				},
				RowCapacity:  2,
				CellCapacity: 3,
			},
			{
				Input:      1, Factor: 0, Axis: 0,
				Sources:    ruleplan.Span{Start: 1, Count: 2},
				Relation:   ruleplan.RelationAddr{Axis: 0, Member: 7},
				Key:        ruleplan.ProjectionAddr{Axis: 0, Member: 8},
				Parent:     ruleplan.RelationAddr{Axis: 0, Member: 0}, ParentPresent: true,
				Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
				Form:       ruleprogram.Summary,
				PointBound: ruleprogram.PointBound,
				Contract: ruleplan.ReadContract{
					Order:        ruleprogram.OrderCanonical,
					Sparse:       ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaquePropagateAuthenticated,
					Multiplicity: ruleprogram.MultiplicityMany,
				},
				Denominator:  ruleplan.DenominatorAddr{Ordinal: 5, Present: true},
				RowCapacity:  4,
				CellCapacity: 8,
			},
		},
		Outputs: []generated.OutputPlan{{
			Factor:      0,
			Axis:        0,
			Address:     ruleplan.OutputAddr{Axis: 0, Frame: 3},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 11},
			Mode:        ruleprogram.ModeStructural,
			Slot:        0,
		}},
		Transports: []ruleplan.Transport{{Axis: 0, Exported: true}, {Axis: 2}},
		Activation: &ruleplan.Activation{
			Branch:      1,
			Application: ruleplan.ProjectionAddr{Axis: 0, Member: 12},
			Target:      ruleplan.ProjectionAddr{Axis: 0, Member: 13},
			Endpoint:    ruleplan.ProjectionAddr{Axis: 0, Member: 14},
			Mount:       ruleplan.ProjectionAddr{Axis: 0, Member: 15},
			Body:        ruleplan.ProjectionAddr{Axis: 0, Member: 16},
		},
	}
}

func activationFormLawDescriptor(t testing.TB) generated.CompiledRule {
	t.Helper()
	descriptor, sealed := generated.NewPlanCompiledRule(activationFormLawSpec())
	if !sealed {
		t.Fatal("sealed activation descriptor")
	}
	return descriptor
}

// TestActivationFormIsDerivedFromItsStructuralPublication states the
// derivation law: a structural publication transports axes across a transition
// instead of writing a fact, and a descriptor carries that transport vector
// exactly when its mode is structural. So the publication mode alone decides
// this form, and the branch set its rows are drawn from names the port the row
// is opened at. Nothing about read order or read count participates.
func TestActivationFormIsDerivedFromItsStructuralPublication(t *testing.T) {
	descriptor := activationFormLawDescriptor(t)
	row, derived := DeclaredForm(descriptor)
	if !derived || row.Form != FormActivation {
		t.Fatalf("derived as %q/%t, want activation", row.Form.Name(), derived)
	}
	if row.Input != 1 {
		t.Fatalf("derived read port = %d, want the branch join", row.Input)
	}
	if row.Relation != 7 {
		t.Fatalf("derived relation = %d, want the relation whose members are the branches", row.Relation)
	}
	if row.Rule.TransportCount() == 0 {
		t.Fatal("a structural publication derived the activation form with no transport vector")
	}
}

// TestAStructuralRowDrawsItsBranchesFromAColdMemberSet is the correction this
// form's derivation rests on. The construct topology mounts one activation
// member per branch before any solve; execution settles dispositions of
// branches that are already mounted and can publish no others. A selection's
// members are resolved per invocation by the reading family, so a structural
// row whose branches came from one would name rows nothing had mounted.
//
// The refusal lands at the SEAL, not at the derivation: the descriptor carries
// the branch vocabulary, and the vocabulary names which read the branches are
// members of, so "that read is a per-invocation selection" is a disagreement
// the descriptor can see in itself. The derivation is checked too, for the
// case where a future seal admits a shape this one does not.
func TestAStructuralRowDrawsItsBranchesFromAColdMemberSet(t *testing.T) {
	spec := activationFormLawSpec()
	spec.Reads[1].Form = ruleprogram.Selected
	spec.Reads[1].Parent, spec.Reads[1].ParentPresent = ruleplan.RelationAddr{}, false
	spec.Reads[1].Addressing, spec.Reads[1].AddressingPresent = ruleplan.RelationAddr{}, false
	spec.Reads[1].Predicate, spec.Reads[1].PredicatePresent = ruleplan.ProjectionAddr{Axis: 0, Member: 9}, true
	spec.Reads[1].Contract.Order = ruleprogram.OrderByTag
	descriptor, sealed := generated.NewPlanCompiledRule(spec)
	if sealed {
		if row, derived := DeclaredForm(descriptor); derived {
			t.Fatalf("a structural row over a per-invocation selection derived %q", row.Form.Name())
		}
		t.Fatal("a structural descriptor whose branch read is a per-invocation selection sealed")
	}
}

// TestAStructuralDescriptorWithNoBranchSetIsNoActivation covers the near
// misses that the derivation itself settles. A structural publication whose
// branches are drawn from no selection has no candidate set to transport, and
// there is no port for the row to open at, so it derives no form at all.
//
// Read ORDER is deliberately not one of these. Which position the trigger
// occupies is not what makes a descriptor an activation, and the form the
// derivation answers has no generic builder: every row of it is authored by an
// installed family that holds the descriptor to its own declared shape. A
// refusal on position would have been the derivation guessing at a geometry it
// does not implement.
func TestAStructuralDescriptorWithNoBranchSetIsNoActivation(t *testing.T) {
	for name, damage := range map[string]func(*generated.CompiledRuleSpec){
		"single read": func(spec *generated.CompiledRuleSpec) {
			spec.Reads = spec.Reads[:1]
			spec.InputCount = 1
		},
		"both exact": func(spec *generated.CompiledRuleSpec) {
			spec.Reads[1].Form = ruleprogram.Exact
			spec.Reads[1].Parent, spec.Reads[1].ParentPresent = ruleplan.RelationAddr{}, false
			spec.Reads[1].Denominator = ruleplan.DenominatorAddr{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := activationFormLawSpec()
			damage(&spec)
			descriptor, sealed := generated.NewPlanCompiledRule(spec)
			if !sealed {
				// The malformed shape never reached the derivation, which
				// already refuses it.
				return
			}
			if row, derived := DeclaredForm(descriptor); derived {
				t.Fatalf("a %s structural descriptor derived %q", name, row.Form.Name())
			}
		})
	}
}

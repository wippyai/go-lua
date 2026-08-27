package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// The read-free transformed carry laws. A rule of this shape declares no read
// at all: its judgment is a function of the directory row it is indexed by, and
// its publication moves every carried coordinate of its Factor through the
// candidate's declared transition.
//
// It is the carry shape's read-free case, not a shape of its own. The installer
// half is identical - the same sealed CarryWrite, the same declared transition -
// so what the declaration reads is the only distinction, and the form the row is
// admitted under is what states it.

// sourceCarryRoster is the specimen vocabulary with a candidate-only reducer:
// the fold declares no input, because there is no read to consume.
func sourceCarryRoster(t testing.TB) definition.Roster {
	t.Helper()
	provider := member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"}
	base := definition.Definition{
		Name:       "Specimen",
		Axis:       "specimen",
		ImportPath: specimenPackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "KeyCarrier",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: specimenMethod("KeyIndex", "Schema", 0),
		}},
		Signature: definition.Signature{Key: "KeyCarrier", Fact: "FactCarrier"},
		Carriers: []definition.Carrier{
			{Name: "KeyCarrier", Key: "carrier/specimen/key", Type: specimenType("Key"), Capability: carrier.Equatable},
			{Name: "FactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact"), Capability: carrier.Ascending},
		},
		Relations: []definition.Relation{{
			Name: "Candidates", Key: "specimen/candidates", Subject: "KeyCarrier",
			CandidateProvider: member.AxisRelationCandidate(provider),
			CandidateResolver: specimenMethod("CandidateForOccurrence", "Schema", 0),
			CandidateOrdinal:  specimenMethod("CandidateOrdinal", "Schema", 0),
			CandidateAt:       specimenMethod("CandidateAt", "Schema", 0),
		}},
		Projections: []definition.Projection{{
			Name: "Coordinate", Key: "specimen/coordinate", Relation: "Candidates",
			CandidateProvider: member.AxisRelationCandidate(provider), Role: member.Destination, Result: "KeyCarrier",
			Accessor: specimenMethod("Coordinate", "Key", -1),
		}},
		CarryTransforms: []definition.CarryTransform{{
			Name: "Transition", Key: "specimen/transition", Candidate: "KeyCarrier",
			Input: "FactCarrier", Output: "FactCarrier",
			Implementation: specimenMethod("Age", "Key", 0),
		}},
	}
	contribution := definition.Contribution{
		Axis: "specimen",
		Rule: "specimen-source-carry",
		Reducers: []definition.Reducer{{
			Name: "SourceCarryReducer", Key: "specimen/reducer/source-carry", Candidate: "KeyCarrier",
			Outputs:        []definition.ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: specimenPackage, Name: "SourceCarryFold", ResultIndex: 0},
		}},
	}
	roster, rosterOK := definition.NewRoster(definition.Source{
		Package: "specimen", Name: "specimen", Base: base,
		Contributions: []definition.Contribution{contribution},
	})
	if !rosterOK {
		t.Fatal("read-free carry member roster is not admissible")
	}
	return roster
}

func sourceCarrySpec() rule.Spec {
	spec := specimenSpec()
	spec.Key = "specimen-source-carry"
	spec.Semantic = "semantic/rule/specimen/source-carry"
	spec.Program.Joins = nil
	spec.Program.Fold.Reducer = member.ReducerRef{Axis: specimenAxis(), Member: "specimen/reducer/source-carry"}
	spec.Program.Fold.Inputs = nil
	return spec
}

func renderSourceCarry(t testing.TB) string {
	t.Helper()
	source, err := Render(Target{
		PackagePath: "example/rule/sourcecarry", PackageName: "sourcecarry", Spec: sourceCarrySpec(),
	}, sourceCarryRoster(t))
	if err != nil {
		t.Fatalf("read-free carry declaration did not emit: %v", err)
	}
	return string(source)
}

// TestAReadFreeCarryJudgesFromItsCandidateAlone is the input law. The
// declaration names no read, so the emitted judgment takes no cell and the
// worker opens no cursor. A reducer that accepted a cell it never read would
// let a declaration name a read the execution silently drops.
func TestAReadFreeCarryJudgesFromItsCandidateAlone(t *testing.T) {
	source := renderSourceCarry(t)
	if !strings.Contains(source, "func (fold familyReducer) Reduce() (specimen.Fact, structure.ReductionOutcome)") {
		t.Fatalf("the emitted judgment does not take the candidate alone:\n%s", source)
	}
	// The write scratch is not a cursor. What must be absent is any READ
	// primitive: a sealed ExactRead on a row, and any cursor step in the worker.
	for _, absent := range []string{"ExactRead", ".Read(ticket", "ReadCellPolicy"} {
		if strings.Contains(source, absent) {
			t.Fatalf("the emitted family opens a cursor for a rule that declares no read (%q):\n%s", absent, source)
		}
	}
}

// TestAReadFreeCarryPublishesThroughItsDeclaredTransition is the publication
// law. The row still carries: every carried coordinate of the Factor passes
// through the candidate's declared transition, in the one patch the row
// publishes through, which is what distinguishes this from a source column.
func TestAReadFreeCarryPublishesThroughItsDeclaredTransition(t *testing.T) {
	source := renderSourceCarry(t)
	for _, fragment := range []string{
		"execution.FoldSourceCarry(ticket, familyReducer{candidate: row.candidate}, row.write, &lane.write)",
		"execution.CarryWrite[specimen.DenseCoordinate, specimen.Fact]",
		"plane.RowCarry(planRow, candidate.Age)",
	} {
		if !strings.Contains(source, fragment) {
			t.Fatalf("the emitted family does not state %q:\n%s", fragment, source)
		}
	}
}

// TestAReadFreeCarryIsAdmittedUnderItsOwnForm is the classification law. The
// carry shape has two forms, chosen by what the declaration reads, and the
// emitted installer holds its plan rows to the one its declaration derives. A
// family admitting rows of the other form would execute a geometry it was not
// sealed for.
func TestAReadFreeCarryIsAdmittedUnderItsOwnForm(t *testing.T) {
	source := renderSourceCarry(t)
	if !strings.Contains(source, "planRow.Form != execution.FormSourceCarry") {
		t.Fatalf("the emitted installer does not fence the read-free carry form:\n%s", source)
	}
	if !strings.Contains(source, "planRow.Rule.ReadCount() != 0") {
		t.Fatalf("the emitted installer does not hold its rows to a read-free plan:\n%s", source)
	}
	carried := renderSpecimen(t, specimenTarget())
	if !strings.Contains(carried, "planRow.Form != execution.FormCarry") {
		t.Fatalf("the one-read carry no longer states its own form:\n%s", carried)
	}
}

// TestAReadFreeCarryStillHoldsItsDeclaredTransform is the refusal law. What
// makes this a carry at all is the owner-issued transition; a declaration that
// names none is a source column and is refused here by name rather than
// emitted as a carry that carries nothing.
func TestAReadFreeCarryStillHoldsItsDeclaredTransform(t *testing.T) {
	spec := sourceCarrySpec()
	spec.Program.Carry = nil
	_, err := Render(Target{
		PackagePath: "example/rule/sourcecarry", PackageName: "sourcecarry", Spec: spec,
	}, sourceCarryRoster(t))
	if err == nil {
		t.Fatal("a read-free rule with no carry was emitted as a carry family")
	}
	if !strings.Contains(err.Error(), "an authored exact fold over no read") {
		t.Fatalf("refusal does not name the clause: %v", err)
	}
}

// TestAReadFreeCarryRefusesAFoldInputItCannotDeliver is the correspondence law.
// A fold input names the join whose cell it receives, so a declaration with no
// join cannot declare one; admitting it would emit a judgment whose parameter
// nothing fills.
func TestAReadFreeCarryRefusesAFoldInputItCannotDeliver(t *testing.T) {
	spec := sourceCarrySpec()
	spec.Program.Fold.Inputs = []program.JoinRef{0}
	_, err := Render(Target{
		PackagePath: "example/rule/sourcecarry", PackageName: "sourcecarry", Spec: spec,
	}, sourceCarryRoster(t))
	if err == nil {
		t.Fatal("a fold input with no join was admitted")
	}
}

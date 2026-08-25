package codegen

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

func callShapeType(name string) definition.GoType {
	return definition.GoType{PackagePath: "example/callshape", Name: name}
}

// routedCall is the call shape of a routed fold: a candidate carrier, an
// untagged exact input, and the tagged selected input the output publishes
// over.
func routedCall() ReducerCall {
	return ReducerCall{
		Key:              "reducer/routed",
		Candidate:        callShapeType("Candidate"),
		CandidatePresent: true,
		Inputs: []ReducerInput{
			{Join: 0, Type: callShapeType("Source"), Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
			{Join: 1, Type: callShapeType("Fact"), Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: callShapeType("Route"), Tagged: true},
		},
		Outputs: []Output{{valueType: callShapeType("Fact")}},
		Outcome: ReducerOutcomeType,
	}
}

// TestReducerArgumentsAreCarrierValuesOnly is the call-shape law. Every
// position of a reducer's direct call carries a carrier value the declaration
// named: the candidate carrier, an input's route coordinate, an input's tag
// carrier, or the input's own fact carrier - delivered directly, or through
// the analyzer's sealed view when the read is many-valued. An owner schema, a
// derived route plan, a projection, or an ordinal can never appear, because
// there is no role for one: those are the sealed state of the installed Family
// that calls the reducer, bound once when the owner installs it.
func TestReducerArgumentsAreCarrierValuesOnly(t *testing.T) {
	for _, call := range []ReducerCall{routedCall(), summaryCall()} {
		declared := map[definition.GoType]bool{call.Candidate: true}
		for _, input := range call.Inputs {
			declared[input.Type] = true
			if input.Routed {
				declared[input.Route] = true
			}
			if input.Tagged {
				declared[input.Tag] = true
			}
		}
		for position, argument := range call.Arguments() {
			carried := argument.Type
			switch argument.Role {
			case ReducerArgumentCandidate, ReducerArgumentRoute, ReducerArgumentTag, ReducerArgumentFact:
			case ReducerArgumentVector:
				// A vector position's own type is the analyzer's view, which no
				// owner declares. What it CARRIES is the declared carrier the
				// view is instantiated at, and that is what the law is about.
				if argument.Type != SummaryVectorType {
					t.Fatalf("argument %d is delivered through %v, not the sealed view", position, argument.Type)
				}
				carried = argument.Element
			default:
				t.Fatalf("argument %d carries role %d, which is not a declared carrier role", position, argument.Role)
			}
			if !declared[carried] {
				t.Fatalf("argument %d is %v, a type no carrier row declared", position, carried)
			}
		}
	}
}

// TestReducerArgumentsPlaceTheCandidateFirstAndEachTagBeforeItsFact fixes the
// order the emitter writes and a fold is written against. A tag precedes the
// fact it names because it is what says which member of the selection this
// invocation is folding.
func TestReducerArgumentsPlaceTheCandidateFirstAndEachTagBeforeItsFact(t *testing.T) {
	arguments := routedCall().Arguments()
	want := []ReducerArgument{
		{Role: ReducerArgumentCandidate, Type: callShapeType("Candidate"), Input: -1},
		{Role: ReducerArgumentFact, Type: callShapeType("Source"), Input: 0},
		{Role: ReducerArgumentTag, Type: callShapeType("Route"), Input: 1},
		{Role: ReducerArgumentFact, Type: callShapeType("Fact"), Input: 1},
	}
	if len(arguments) != len(want) {
		t.Fatalf("argument count = %d, want %d", len(arguments), len(want))
	}
	for index, argument := range arguments {
		if argument != want[index] {
			t.Fatalf("argument %d = %+v, want %+v", index, argument, want[index])
		}
	}
}

// TestReducerSignatureIsAFunctionOfTheDeclarationAlone is the law that keeps
// the signature from growing plumbing. Its width is decided entirely by the
// declared rows - one optional candidate, plus one position per input and one
// more for each tagged input - so nothing an implementation happens to need can
// widen it. A fold that wants more owner knowledge takes it from its Family's
// sealed state, which is invisible to this contract.
func TestReducerSignatureIsAFunctionOfTheDeclarationAlone(t *testing.T) {
	for _, test := range []struct {
		name      string
		call      ReducerCall
		arguments int
	}{
		{name: "routed", call: routedCall(), arguments: 4},
		{name: "no-candidate", call: func() ReducerCall {
			call := routedCall()
			call.Candidate, call.CandidatePresent, call.CandidateConstant = definition.GoType{}, false, true
			return call
		}(), arguments: 3},
		{name: "no-inputs", call: func() ReducerCall {
			call := routedCall()
			call.Inputs = nil
			return call
		}(), arguments: 1},
		{name: "untagged-only", call: func() ReducerCall {
			call := routedCall()
			call.Inputs = call.Inputs[:1]
			return call
		}(), arguments: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := len(test.call.Arguments()); got != test.arguments {
				t.Fatalf("argument count = %d, want %d", got, test.arguments)
			}
		})
	}
}

// TestReducerResultsEndWithTheSealedDisposition states that a reducer answers
// with its declared output carriers and then exactly one disposition, and that
// the disposition is the analyzer's single sealed vocabulary rather than one
// the owner chose.
func TestReducerResultsEndWithTheSealedDisposition(t *testing.T) {
	results := routedCall().Results()
	if len(results) != 2 {
		t.Fatalf("result count = %d, want one output carrier and one disposition", len(results))
	}
	if results[0] != callShapeType("Fact") {
		t.Fatalf("first result = %v, want the declared output carrier", results[0])
	}
	if results[1] != ReducerOutcomeType {
		t.Fatalf("last result = %v, want %v", results[1], ReducerOutcomeType)
	}
}

// summaryCall is the call shape of a fold over a whole denominator: a
// candidate carrier and one many-valued input whose declaration also names a
// tag carrier.
func summaryCall() ReducerCall {
	return ReducerCall{
		Key:              "reducer/summary",
		Candidate:        callShapeType("Candidate"),
		CandidatePresent: true,
		Inputs: []ReducerInput{
			{Join: 0, Type: callShapeType("Source"), Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
			{Join: 1, Type: callShapeType("Cell"), Form: member.ReadFormSummary, Multiplicity: member.MultiplicityMany, Tag: callShapeType("Coordinate"), Tagged: true},
		},
		Outputs: []Output{{valueType: callShapeType("Fact")}},
		Outcome: ReducerOutcomeType,
	}
}

// TestAManyValuedInputIsOneVectorPositionOverItsOwnCarrier states how a fold
// over a whole denominator is called. The read delivers every cell of its
// sealed denominator in one row, so the fold is handed that vector: there is
// no per-cell invocation, and decomposing the row into one would ask the fold
// to rebuild a correlation the read already established.
//
// The view is the execution layer's, instantiated at the input's own declared
// fact carrier, so an owner names no container of its own and the cells are
// read where they were materialized.
func TestAManyValuedInputIsOneVectorPositionOverItsOwnCarrier(t *testing.T) {
	arguments := summaryCall().Arguments()
	want := []ReducerArgument{
		{Role: ReducerArgumentCandidate, Type: callShapeType("Candidate"), Input: -1},
		{Role: ReducerArgumentFact, Type: callShapeType("Source"), Input: 0},
		{Role: ReducerArgumentVector, Type: SummaryVectorType, Element: callShapeType("Cell"), Input: 1},
	}
	if len(arguments) != len(want) {
		t.Fatalf("argument count = %d, want %d", len(arguments), len(want))
	}
	for index, argument := range arguments {
		if argument != want[index] {
			t.Fatalf("argument %d = %+v, want %+v", index, argument, want[index])
		}
	}
}

// TestAManyValuedInputCarriesNoTag states why the tag position disappears. A
// tag says WHICH member of a selection an invocation folds; a vector delivers
// them all, in the order its sealed denominator declares, so the position of a
// cell is already its identity. A tag argument beside the whole vector would
// name one member the call is not about.
func TestAManyValuedInputCarriesNoTag(t *testing.T) {
	for _, argument := range summaryCall().Arguments() {
		if argument.Role == ReducerArgumentTag {
			t.Fatalf("a many-valued input contributed a tag position carrying %v", argument.Type)
		}
	}
	tagged := summaryCall()
	tagged.Inputs[1].Multiplicity = member.MultiplicityOne
	found := false
	for _, argument := range tagged.Arguments() {
		if argument.Role == ReducerArgumentTag {
			found = true
		}
		if argument.Role == ReducerArgumentVector {
			t.Fatal("a single-valued input was delivered as a vector")
		}
	}
	if !found {
		t.Fatal("the same declaration read at one member contributed no tag position")
	}
}

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
// position of a reducer's direct call is a carrier value the declaration named:
// the candidate carrier, an input's tag carrier, or an input's fact carrier.
// There is no fourth role, so an owner schema, a derived route plan, a
// projection, or an ordinal can never appear in the signature - those are the
// sealed state of the installed Family that calls the reducer, bound once when
// the owner installs it.
func TestReducerArgumentsAreCarrierValuesOnly(t *testing.T) {
	call := routedCall()
	declared := map[definition.GoType]bool{call.Candidate: true}
	for _, input := range call.Inputs {
		declared[input.Type] = true
		if input.Tagged {
			declared[input.Tag] = true
		}
	}
	for position, argument := range call.Arguments() {
		switch argument.Role {
		case ReducerArgumentCandidate, ReducerArgumentTag, ReducerArgumentFact:
		default:
			t.Fatalf("argument %d carries role %d, which is not a declared carrier role", position, argument.Role)
		}
		if !declared[argument.Type] {
			t.Fatalf("argument %d is %v, a type no carrier row declared", position, argument.Type)
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

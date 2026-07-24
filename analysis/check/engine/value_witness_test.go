package engine

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func encodedWitness(t *testing.T, value typ.Type) []byte {
	t.Helper()
	encoded, ok := shapefact.EncodeTarget(value)
	if !ok {
		t.Fatalf("EncodeTarget(%v) failed", value)
	}
	return encoded
}

// A boolean witness holds both true and false. Deciding such a condition drops
// the false edge, which lets everything the guard dominates be published as
// proven on a path that does not have to execute.
func TestLuaTruthyKeepsBothEdgesOfABooleanWitness(t *testing.T) {
	if _, err := luaTruthy(encodedWitness(t, typ.Boolean)); !errors.Is(err, errUnknownScalar) {
		t.Fatalf("luaTruthy(boolean witness) err = %v, want errUnknownScalar", err)
	}
	if _, err := luaTruthy(encodedWitness(t, normalize.Optional(typ.Boolean))); !errors.Is(err, errUnknownScalar) {
		t.Fatalf("luaTruthy(boolean? witness) err = %v, want errUnknownScalar", err)
	}
	if _, err := luaTruthy(encodedWitness(t, normalize.UnionForEvidence(typ.String, typ.False))); !errors.Is(err, errUnknownScalar) {
		t.Fatalf("luaTruthy(string | false witness) err = %v, want errUnknownScalar", err)
	}
}

func TestLuaTruthyDecidesAWitnessWithOnlyOneReachableEdge(t *testing.T) {
	for _, testcase := range []struct {
		name  string
		value typ.Type
		truth bool
	}{
		{name: "string", value: typ.String, truth: true},
		{name: "integer", value: typ.Integer, truth: true},
		{name: "record", value: typetable.NewRecord().Field("id", typ.String).Build(), truth: true},
		{name: "true", value: typ.True, truth: true},
		{name: "nil", value: typ.Nil},
		{name: "false", value: typ.False},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			truth, err := luaTruthy(encodedWitness(t, testcase.value))
			if err != nil {
				t.Fatalf("luaTruthy(%s witness) err = %v, want a decision", testcase.name, err)
			}
			if truth != testcase.truth {
				t.Fatalf("luaTruthy(%s witness) = %v, want %v", testcase.name, truth, testcase.truth)
			}
		})
	}
}

func TestLuaTruthyLeavesAnOptionalValueArmUndecided(t *testing.T) {
	if _, err := luaTruthy(encodedWitness(t, normalize.Optional(typ.String))); !errors.Is(err, errUnknownScalar) {
		t.Fatalf("luaTruthy(string? witness) err = %v, want errUnknownScalar", err)
	}
}

// A published type witness has to be judged by the values it holds. Refuting it
// from the shape of its encoding produced contradictions of the form
// "argument 2 is boolean, not boolean".
func TestProvenValueNotSubtypeAcceptsAWitnessOfItsOwnContract(t *testing.T) {
	for _, testcase := range []struct {
		spelling string
		witness  typ.Type
	}{
		{spelling: "boolean", witness: typ.Boolean},
		{spelling: "boolean", witness: typ.True},
		{spelling: "string", witness: typ.String},
		{spelling: "number", witness: typ.Number},
		{spelling: "number", witness: typ.Integer},
		{spelling: "integer", witness: typ.Integer},
		{spelling: "nil", witness: typ.Nil},
		{spelling: "string?", witness: normalize.Optional(typ.String)},
		{spelling: "string?", witness: typ.Nil},
		{spelling: "string?", witness: typ.String},
	} {
		t.Run(testcase.spelling+"/"+testcase.witness.String(), func(t *testing.T) {
			if provenValueNotSubtype(encodedWitness(t, testcase.witness), testcase.spelling) {
				t.Fatalf("provenValueNotSubtype(%v, %q) refuted a witness its contract admits", testcase.witness, testcase.spelling)
			}
		})
	}
}

func TestProvenValueNotSubtypeRefutesAWitnessItsContractExcludes(t *testing.T) {
	for _, testcase := range []struct {
		spelling string
		witness  typ.Type
	}{
		{spelling: "boolean", witness: typ.String},
		{spelling: "string", witness: typ.Boolean},
		{spelling: "number", witness: typ.String},
		{spelling: "integer", witness: typ.Number},
		{spelling: "nil", witness: typ.String},
		{spelling: "string?", witness: typ.Integer},
	} {
		t.Run(testcase.spelling+"/"+testcase.witness.String(), func(t *testing.T) {
			if !provenValueNotSubtype(encodedWitness(t, testcase.witness), testcase.spelling) {
				t.Fatalf("provenValueNotSubtype(%v, %q) accepted a witness its contract excludes", testcase.witness, testcase.spelling)
			}
		})
	}
}

// The scalar vocabulary keeps its existing verdicts: the decoded comparison
// replaces the encoding test, it does not relax it.
func TestProvenValueNotSubtypeKeepsTheScalarVocabularyVerdicts(t *testing.T) {
	for _, testcase := range []struct {
		value    string
		spelling string
		refuted  bool
	}{
		{value: "scalar/nil", spelling: "nil"},
		{value: "scalar/bool/true", spelling: "boolean"},
		{value: "scalar/boolean", spelling: "boolean"},
		{value: `scalar/string/"text"`, spelling: "string"},
		{value: "scalar/number/3", spelling: "integer"},
		{value: "scalar/number/3.5", spelling: "integer", refuted: true},
		{value: "scalar/number/3.5", spelling: "number"},
		{value: "scalar/table", spelling: "string", refuted: true},
		{value: "scalar/function", spelling: "boolean", refuted: true},
		{value: "scalar/top", spelling: "string"},
		{value: "scalar/nil", spelling: "string", refuted: true},
		{value: `scalar/string/"text"`, spelling: "number", refuted: true},
	} {
		t.Run(testcase.value+"/"+testcase.spelling, func(t *testing.T) {
			if got := provenValueNotSubtype([]byte(testcase.value), testcase.spelling); got != testcase.refuted {
				t.Fatalf("provenValueNotSubtype(%q, %q) = %v, want %v", testcase.value, testcase.spelling, got, testcase.refuted)
			}
		})
	}
}

func TestScalarWitnessTypeDecodesTheClosedValueVocabulary(t *testing.T) {
	for _, testcase := range []struct {
		value string
		want  typ.Type
	}{
		{value: "scalar/nil", want: typ.Nil},
		{value: "scalar/boolean", want: typ.Boolean},
		{value: "scalar/bool/true", want: typ.True},
		{value: "scalar/bool/false", want: typ.False},
		{value: "scalar/number/7", want: typ.LiteralInt(7)},
		{value: `scalar/string/"text"`, want: typ.LiteralString("text")},
	} {
		t.Run(testcase.value, func(t *testing.T) {
			got, ok := scalarWitnessType([]byte(testcase.value))
			if !ok || !typ.TypeEquals(got, testcase.want) {
				t.Fatalf("scalarWitnessType(%q) = %v/%v, want %v", testcase.value, got, ok, testcase.want)
			}
		})
	}
	for _, value := range []string{"scalar/top", "scalar/claim/1", "scalar/table", "scalar/function", optionalNilComparison} {
		if _, ok := scalarWitnessType([]byte(value)); ok {
			t.Fatalf("scalarWitnessType(%q) reported a witness type for a value that carries none", value)
		}
	}
}

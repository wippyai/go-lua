package generator

import (
	"strings"
	"testing"
)

func TestExactBinaryDispatchIsGeneratedFromTheOwnerReducerTable(t *testing.T) {
	artifact, err := Render("value", composedSource(t, "value"))
	if err != nil {
		t.Fatal(err)
	}
	generated := string(artifact.ExactBinary)
	for _, required := range []string{
		"func SupportsExactBinaryReducer(reducerOrdinal uint32) bool",
		"func (schema *Schema) ExactBinaryCandidateAvailable(",
		"func (schema *Schema) ReduceExactBinary(",
		"schema.BinaryArithmeticAt(int(candidateOrdinal))",
		"ArithmeticValue(candidate, left, right)",
	} {
		if !strings.Contains(generated, required) {
			t.Fatalf("generated exact-binary dispatch is missing %q:\n%s", required, generated)
		}
	}
	for _, forbidden := range []string{"map[", "func(", "any", "reflect"} {
		if strings.Contains(generated, forbidden) {
			t.Fatalf("generated exact-binary dispatch retained runtime indirection %q:\n%s", forbidden, generated)
		}
	}
}

func TestExactBinaryDispatchDoesNotCoerceAnotherReducerShape(t *testing.T) {
	artifact, err := Render("placement", externalProviderDefinition())
	if err != nil {
		t.Fatal(err)
	}
	generated := string(artifact.ExactBinary)
	if strings.Contains(generated, "case 0:") {
		t.Fatalf("non-binary reducer was coerced into exact-binary dispatch:\n%s", generated)
	}
}

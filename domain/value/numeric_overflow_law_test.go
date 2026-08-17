package value

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

// The overflow discipline is Value's law, not a consumer's table. These laws
// state it over the whole sealed operator vocabulary: every arithmetic
// operator answers for every operand shape, every operator outside that range
// answers for none, and each discipline carries exactly one spelling.

func TestBinaryNumericOverflowCoversTheSealedArithmeticOperatorsLaw(t *testing.T) {
	representations := []programartifact.NumericRepresentation{
		programartifact.NumericRepresentationInteger,
		programartifact.NumericRepresentationFloat,
		programartifact.NumericRepresentationNumber,
	}
	for op := flowkind.BinaryAdd; op <= flowkind.BinaryPow; op++ {
		for _, left := range representations {
			for _, right := range representations {
				overflow, ok := BinaryNumericOverflow(op, left, right)
				if !ok || !overflow.Valid() || overflow.String() == "" {
					t.Fatalf("operator %d over (%d,%d) states no overflow discipline", op, left, right)
				}
			}
		}
	}
}

func TestBinaryNumericOverflowIsTheDeclaredLawLaw(t *testing.T) {
	for _, law := range []struct {
		op      flowkind.BinaryOp
		integer NumericOverflow
		widened NumericOverflow
	}{
		{op: flowkind.BinaryAdd, integer: NumericOverflowPromoteIntegerToNumber, widened: NumericOverflowIEEE754},
		{op: flowkind.BinarySub, integer: NumericOverflowPromoteIntegerToNumber, widened: NumericOverflowIEEE754},
		{op: flowkind.BinaryMul, integer: NumericOverflowPromoteIntegerToNumber, widened: NumericOverflowIEEE754},
		{op: flowkind.BinaryDiv, integer: NumericOverflowIEEE754, widened: NumericOverflowIEEE754},
		{op: flowkind.BinaryIDiv, integer: NumericOverflowClosedInteger, widened: NumericOverflowIEEE754},
		{op: flowkind.BinaryMod, integer: NumericOverflowClosedInteger, widened: NumericOverflowIEEE754},
		{op: flowkind.BinaryPow, integer: NumericOverflowIEEE754, widened: NumericOverflowIEEE754},
	} {
		integer, integerOK := BinaryNumericOverflow(law.op, programartifact.NumericRepresentationInteger, programartifact.NumericRepresentationInteger)
		if !integerOK || integer != law.integer {
			t.Fatalf("operator %d over two integers is %q, want %q", law.op, integer, law.integer)
		}
		for _, mixed := range [][2]programartifact.NumericRepresentation{
			{programartifact.NumericRepresentationInteger, programartifact.NumericRepresentationFloat},
			{programartifact.NumericRepresentationFloat, programartifact.NumericRepresentationInteger},
			{programartifact.NumericRepresentationNumber, programartifact.NumericRepresentationInteger},
			{programartifact.NumericRepresentationFloat, programartifact.NumericRepresentationFloat},
			{programartifact.NumericRepresentationNumber, programartifact.NumericRepresentationNumber},
		} {
			widened, widenedOK := BinaryNumericOverflow(law.op, mixed[0], mixed[1])
			if !widenedOK || widened != law.widened {
				t.Fatalf("operator %d over (%d,%d) is %q, want %q", law.op, mixed[0], mixed[1], widened, law.widened)
			}
		}
	}
}

func TestNumericOverflowRejectsOperatorsWithoutArithmeticLaw(t *testing.T) {
	for op := flowkind.BinaryConcat; op <= flowkind.BinaryGreaterEqual; op++ {
		if overflow, ok := BinaryNumericOverflow(op, programartifact.NumericRepresentationInteger, programartifact.NumericRepresentationInteger); ok || overflow != NumericOverflowInvalid {
			t.Fatalf("non-arithmetic operator %d stated the overflow discipline %q", op, overflow)
		}
	}
	if _, ok := BinaryNumericOverflow(flowkind.BinaryAdd, programartifact.NumericRepresentationInvalid, programartifact.NumericRepresentationInteger); ok {
		t.Fatal("unknown left representation stated an overflow discipline")
	}
	if _, ok := BinaryNumericOverflow(flowkind.BinaryAdd, programartifact.NumericRepresentationInteger, programartifact.NumericRepresentationInvalid); ok {
		t.Fatal("unknown right representation stated an overflow discipline")
	}
	for _, op := range []flowkind.UnaryOp{flowkind.UnaryNot, flowkind.UnaryLen, flowkind.UnaryBitNot} {
		if _, ok := UnaryNumericOverflow(op, programartifact.NumericRepresentationInteger); ok {
			t.Fatalf("non-numeric unary operator %d stated an overflow discipline", op)
		}
	}
	if _, ok := UnaryNumericOverflow(flowkind.UnaryNeg, programartifact.NumericRepresentationInvalid); ok {
		t.Fatal("unknown unary operand representation stated an overflow discipline")
	}
}

func TestUnaryNumericOverflowIsTheDeclaredLawLaw(t *testing.T) {
	integer, integerOK := UnaryNumericOverflow(flowkind.UnaryNeg, programartifact.NumericRepresentationInteger)
	if !integerOK || integer != NumericOverflowClosedInteger {
		t.Fatalf("negated integer is %q, want %q", integer, NumericOverflowClosedInteger)
	}
	for _, operand := range []programartifact.NumericRepresentation{programartifact.NumericRepresentationFloat, programartifact.NumericRepresentationNumber} {
		widened, widenedOK := UnaryNumericOverflow(flowkind.UnaryNeg, operand)
		if !widenedOK || widened != NumericOverflowIEEE754 {
			t.Fatalf("negated representation %d is %q, want %q", operand, widened, NumericOverflowIEEE754)
		}
	}
}

// TestNumericOverflowSpellingsAreStatedOnceLaw pins the published names. They
// are the only spelling of the discipline in the analyzer, so a consumer reads
// them from here instead of keeping a second list beside its renderer.
func TestNumericOverflowSpellingsAreStatedOnceLaw(t *testing.T) {
	spellings := map[NumericOverflow]string{
		NumericOverflowClosedInteger:          "closed_integer",
		NumericOverflowPromoteIntegerToNumber: "promote_integer_to_number",
		NumericOverflowIEEE754:                "ieee754",
	}
	seen := make(map[string]NumericOverflow, len(spellings))
	for overflow, spelling := range spellings {
		if overflow.String() != spelling {
			t.Fatalf("discipline %d renders as %q, want %q", overflow, overflow.String(), spelling)
		}
		if previous, duplicate := seen[spelling]; duplicate {
			t.Fatalf("disciplines %d and %d share the spelling %q", previous, overflow, spelling)
		}
		seen[spelling] = overflow
	}
	if NumericOverflowInvalid.String() != "" || NumericOverflowInvalid.Valid() {
		t.Fatal("the invalid discipline carries a spelling")
	}
	if NumericOverflow(NumericOverflowIEEE754 + 1).Valid() {
		t.Fatal("a discipline above the declared vocabulary is valid")
	}
}

package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/value"
)

// Value owns the primitive arithmetic operator range: value.BinaryArithmeticOperator
// is the single spelling every Value consumer reads. ProgramArtifact keeps its
// own spelling behind its package fence, and these laws state the agreement
// between the two over the whole closed BinaryOp vocabulary: an operator the
// artifact carries on a binary-arithmetic occurrence is an operator Value
// admits, and every operator Value admits is one the artifact does carry.
//
// A verdict names the drifted side, because the two predicates decide the same
// question about the same closed vocabulary. Until the artifact consumes
// Value's spelling directly, this agreement is what keeps a range widened or
// narrowed on one side alone a rejected build rather than a silent split
// between the occurrence Program publishes and the relation Value mounts.

// artifactArithmeticOperators is the operator set ProgramArtifact carries on
// binary-arithmetic occurrences compiled from an authored program that spells
// every member of the closed binary vocabulary.
func artifactArithmeticOperators(t *testing.T) map[flowkind.BinaryOp]bool {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "binary-arithmetic-operator-pin.lua", Text: []byte(`
local a = 3
local b = 4
local s = "s"
local t = "t"
return a + b, a - b, a * b, a / b, a // b, a % b, a ^ b,
	s .. t,
	a & b, a | b, a ~ b, a << b, a >> b,
	a == b, a ~= b, a < b, a <= b, a > b, a >= b
`)})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK || !receipt.Available() {
		t.Fatal("global program schema unavailable")
	}
	artifact, compiled := composite.CompileArtifact(program, receipt)
	if !compiled || artifact == nil || !artifact.Available() {
		t.Fatal("ProgramArtifact compilation failed")
	}
	operators := make(map[flowkind.BinaryOp]bool)
	for index := 0; index < artifact.OccurrenceCount(); index++ {
		row, rowOK := artifact.OccurrenceAt(index)
		if !rowOK {
			t.Fatalf("occurrence row %d is unavailable", index)
		}
		if row.Kind() != programartifact.OccurrenceBinaryArithmetic {
			continue
		}
		_, _, op, arithmeticOK := row.BinaryArithmetic()
		if !arithmeticOK {
			t.Fatalf("binary arithmetic occurrence %d carries no operator", index)
		}
		operators[op] = true
	}
	return operators
}

// TestArtifactArithmeticOperatorRangeIsValues states the agreement ordinal for
// ordinal over the closed binary vocabulary.
func TestArtifactArithmeticOperatorRangeIsValues(t *testing.T) {
	carried := artifactArithmeticOperators(t)
	for _, member := range []struct {
		op       flowkind.BinaryOp
		spelling string
	}{
		{flowkind.BinaryAdd, "flowkind.BinaryAdd"},
		{flowkind.BinarySub, "flowkind.BinarySub"},
		{flowkind.BinaryMul, "flowkind.BinaryMul"},
		{flowkind.BinaryDiv, "flowkind.BinaryDiv"},
		{flowkind.BinaryIDiv, "flowkind.BinaryIDiv"},
		{flowkind.BinaryMod, "flowkind.BinaryMod"},
		{flowkind.BinaryPow, "flowkind.BinaryPow"},
		{flowkind.BinaryConcat, "flowkind.BinaryConcat"},
		{flowkind.BinaryBitAnd, "flowkind.BinaryBitAnd"},
		{flowkind.BinaryBitOr, "flowkind.BinaryBitOr"},
		{flowkind.BinaryBitXor, "flowkind.BinaryBitXor"},
		{flowkind.BinaryShiftLeft, "flowkind.BinaryShiftLeft"},
		{flowkind.BinaryShiftRight, "flowkind.BinaryShiftRight"},
		{flowkind.BinaryEqual, "flowkind.BinaryEqual"},
		{flowkind.BinaryNotEqual, "flowkind.BinaryNotEqual"},
		{flowkind.BinaryLess, "flowkind.BinaryLess"},
		{flowkind.BinaryLessEqual, "flowkind.BinaryLessEqual"},
		{flowkind.BinaryGreater, "flowkind.BinaryGreater"},
		{flowkind.BinaryGreaterEqual, "flowkind.BinaryGreaterEqual"},
	} {
		owned := value.BinaryArithmeticOperator(member.op)
		if carried[member.op] != owned {
			if owned {
				t.Fatalf("%s is a Value arithmetic operator, but the artifact carries no binary-arithmetic occurrence for it", member.spelling)
			}
			t.Fatalf("the artifact carries %s on a binary-arithmetic occurrence, but Value does not admit it", member.spelling)
		}
	}
}

// TestValueArithmeticOperatorRangeIsClosed states the other half: the owned
// predicate answers the whole BinaryOp width, so an ordinal outside the closed
// vocabulary is rejected rather than admitted by a range bound that runs past
// the last declared member.
func TestValueArithmeticOperatorRangeIsClosed(t *testing.T) {
	for ordinal := 0; ordinal <= 255; ordinal++ {
		op := flowkind.BinaryOp(ordinal)
		admitted := op >= flowkind.BinaryAdd && op <= flowkind.BinaryPow
		if value.BinaryArithmeticOperator(op) != admitted {
			t.Fatalf("value.BinaryArithmeticOperator(%d) = %v, want %v", ordinal, !admitted, admitted)
		}
		if ordinal > int(flowkind.BinaryGreaterEqual) && value.BinaryArithmeticOperator(op) {
			t.Fatalf("value.BinaryArithmeticOperator admitted the undeclared ordinal %d", ordinal)
		}
	}
}

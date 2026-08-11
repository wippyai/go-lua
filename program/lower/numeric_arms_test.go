package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestProgramNumericCandidateVocabularyAndSources(t *testing.T) {
	p := parseBindLower(t, `
local a, b = 1, 2
local n = -a
return a + b, a - b, a * b, a / b, a // b, a % b, a ^ b,
  a .. b, a & b, a | b, a ~ b, a << b, a >> b,
  a == b, a ~= b, a < b, a <= b, a > b, a >= b, n
`)
	flow := p.Flow()
	binaries := flow.Authored().Operators().Binaries()
	unaries := flow.Authored().Operators().Unaries()
	binaryCandidates := flow.Candidates().Binary()

	assertBinaryBucket := func(name string, count int, at func(int) (keyspace.Term, bool), valid func(kind.BinaryOp) bool) {
		t.Helper()
		for index := 0; index < count; index++ {
			term, ok := at(index)
			if !ok || !flow.Executable().Contains(term) {
				t.Fatalf("%s candidate %d = %v/%v", name, index, term, ok)
			}
			_, op, left, right, rowOK := binaries.Get(term)
			if !rowOK || left == 0 || right == 0 || !valid(op) {
				t.Fatalf("%s candidate %d source = op %v left %v right %v ok %v", name, index, op, left, right, rowOK)
			}
		}
	}

	assertBinaryBucket("arithmetic", binaryCandidates.ArithmeticCount(), binaryCandidates.ArithmeticAt, func(op kind.BinaryOp) bool {
		return op >= kind.BinaryAdd && op <= kind.BinaryPow
	})
	assertBinaryBucket("concat", binaryCandidates.ConcatCount(), binaryCandidates.ConcatAt, func(op kind.BinaryOp) bool {
		return op == kind.BinaryConcat
	})
	assertBinaryBucket("bitwise", binaryCandidates.BitwiseCount(), binaryCandidates.BitwiseAt, func(op kind.BinaryOp) bool {
		return op >= kind.BinaryBitAnd && op <= kind.BinaryShiftRight
	})
	assertBinaryBucket("equality", binaryCandidates.EqualityCount(), binaryCandidates.EqualityAt, func(op kind.BinaryOp) bool {
		return op == kind.BinaryEqual || op == kind.BinaryNotEqual
	})
	assertBinaryBucket("order", binaryCandidates.OrderCount(), binaryCandidates.OrderAt, func(op kind.BinaryOp) bool {
		return op >= kind.BinaryLess && op <= kind.BinaryGreaterEqual
	})
	if got, want := binaryCandidates.ArithmeticCount(), 7; got != want {
		t.Fatalf("ArithmeticCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.ConcatCount(), 1; got != want {
		t.Fatalf("ConcatCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.BitwiseCount(), 5; got != want {
		t.Fatalf("BitwiseCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.EqualityCount(), 2; got != want {
		t.Fatalf("EqualityCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.OrderCount(), 4; got != want {
		t.Fatalf("OrderCount = %d, want %d", got, want)
	}

	numeric := flow.Candidates().Unary()
	if got, want := numeric.NumericCount(), 1; got != want {
		t.Fatalf("UnaryNumericCount = %d, want %d", got, want)
	}
	term, ok := numeric.NumericAt(0)
	if !ok || !flow.Executable().Contains(term) {
		t.Fatalf("UnaryNumericAt(0) = %v/%v", term, ok)
	}
	_, op, operand, unaryOK := unaries.Get(term)
	if !unaryOK || op != kind.UnaryNeg || operand == 0 {
		t.Fatalf("UnaryNumeric source = op %v operand %v ok %v", op, operand, unaryOK)
	}
}

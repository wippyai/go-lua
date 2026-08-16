package program_test

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

func TestTransformerBinaryArithmeticOccurrencesRetainExactProgramGeometry(t *testing.T) {
	programValue := lowerProofRepairProgram(t, "binary-arithmetic.lua", `
local a, b = 10, 3
return a + b, a - b, a * b, a / b, a // b, a % b, a ^ b
`)
	input := programValue.TransformerInput()
	want := []flowkind.BinaryOp{
		flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv,
		flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow,
	}
	if input.BinaryArithmeticOccurrenceCount() != len(want) {
		t.Fatalf("BinaryArithmeticOccurrenceCount = %d, want %d", input.BinaryArithmeticOccurrenceCount(), len(want))
	}
	seen := make(map[flowkind.BinaryOp]bool, len(want))
	for index := 0; index < input.BinaryArithmeticOccurrenceCount(); index++ {
		row, ok := input.BinaryArithmeticOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		span, spanOK := row.Span()
		left, leftOK := row.LeftSpan()
		right, rightOK := row.RightSpan()
		if !ok || !entryOK || !finishOK || !spanOK || !leftOK || !rightOK ||
			!input.OwnsSite(entry) || !input.OwnsSite(finish) || !input.OwnsSpan(span) ||
			!input.OwnsSpan(left) || !input.OwnsSpan(right) || !row.ContextID().Available() ||
			!row.BodyPathID().Available() || !row.LeftID().Available() || !row.RightID().Available() {
			t.Fatalf("BinaryArithmeticOccurrenceAt(%d) lost owner-fenced geometry", index)
		}
		seen[row.Op()] = true
	}
	for _, op := range want {
		if !seen[op] {
			t.Fatalf("arithmetic operator %d was not retained", op)
		}
	}
	if _, ok := input.BinaryArithmeticOccurrenceAt(-1); ok {
		t.Fatal("negative arithmetic occurrence index was accepted")
	}
}

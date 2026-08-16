package program_test

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

func TestTransformerBinaryOrderOccurrencesRetainExactProgramGeometry(t *testing.T) {
	programValue := lowerProofRepairProgram(t, "binary-order.lua", `
local a, b = 3, 5
if a < b then a = b end
if a <= b then a = b end
if a > b then a = b end
if a >= b then a = b end
return a
`)
	input := programValue.TransformerInput()
	want := []flowkind.BinaryOp{
		flowkind.BinaryLess,
		flowkind.BinaryLessEqual,
		flowkind.BinaryGreater,
		flowkind.BinaryGreaterEqual,
	}
	if input.BinaryOrderOccurrenceCount() != len(want) {
		t.Fatalf("BinaryOrderOccurrenceCount = %d, want %d", input.BinaryOrderOccurrenceCount(), len(want))
	}
	seen := make(map[flowkind.BinaryOp]bool, len(want))
	for index := 0; index < input.BinaryOrderOccurrenceCount(); index++ {
		row, ok := input.BinaryOrderOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		span, spanOK := row.Span()
		left, leftOK := row.LeftSpan()
		right, rightOK := row.RightSpan()
		if !ok || !entryOK || !finishOK || !spanOK || !leftOK || !rightOK ||
			!input.OwnsSite(entry) || !input.OwnsSite(finish) || !input.OwnsSpan(span) ||
			!input.OwnsSpan(left) || !input.OwnsSpan(right) || !row.ContextID().Available() ||
			!row.BodyPathID().Available() || !row.LeftID().Available() || !row.RightID().Available() {
			t.Fatalf("BinaryOrderOccurrenceAt(%d) lost owner-fenced geometry", index)
		}
		seen[row.Op()] = true
	}
	for _, op := range want {
		if !seen[op] {
			t.Fatalf("order operator %d was not retained", op)
		}
	}
	if _, ok := input.BinaryOrderOccurrenceAt(-1); ok {
		t.Fatal("negative order occurrence index was accepted")
	}
}

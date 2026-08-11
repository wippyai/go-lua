package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

var (
	edgeResetCountSink int
	edgeResetTermSink  keyspace.Term
	edgeResetBoolSink  bool
)

// Reset support belongs to one final Causal Edge.  The index returned by the
// sealed Edges owner is the only handle accepted by ResetCount/At/Contains;
// no Program-wide Mu or decision-set forwarding surface remains.
func TestFlowCausalRecurrenceEdgesCarryTheirOwnResetSupport(t *testing.T) {
	p := parseBindLower(t, `while again() do tick() end`)
	loop, ok := p.Flow().Authored().Control().Loops().At(0)
	if !ok {
		t.Fatal("missing while Loop")
	}
	edges := p.Flow().Causal().Edges()
	feedback := -1
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if edgeOK && edge.Mu == loop {
			feedback = index
			break
		}
	}
	if feedback < 0 {
		t.Fatal("while recurrence has no final Causal Edge")
	}
	count, ok := edges.ResetCount(feedback)
	if !ok || count == 0 || !edges.ResetContains(feedback, loop) {
		t.Fatalf("while reset support = %d/%v, contains Loop=%v", count, ok, edges.ResetContains(feedback, loop))
	}
	for index := 0; index < count; index++ {
		decision, decisionOK := edges.ResetAt(feedback, index)
		if !decisionOK || decision == 0 {
			t.Fatalf("while reset[%d] = %v/%v", index, decision, decisionOK)
		}
	}
	if decision, decisionOK := edges.ResetAt(feedback, count); decisionOK || decision != 0 {
		t.Fatalf("while reset past end = %v/%v", decision, decisionOK)
	}
}

func TestFlowCausalGotoRecurrenceCanRetainEmptyResetSupport(t *testing.T) {
	p := parseBindLower(t, `::again::; goto again`)
	label, ok := p.Flow().Authored().Control().Labels().At(0)
	if !ok {
		t.Fatal("missing label")
	}
	edges := p.Flow().Causal().Edges()
	feedback := -1
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if edgeOK && edge.Mu == label {
			feedback = index
			break
		}
	}
	if feedback < 0 {
		t.Fatal("backward goto has no final recurrence Edge")
	}
	if count, ok := edges.ResetCount(feedback); !ok || count != 0 {
		t.Fatalf("empty goto reset = %d/%v, want 0/true", count, ok)
	}
	if decision, ok := edges.ResetAt(feedback, 0); ok || decision != 0 {
		t.Fatalf("empty goto reset[0] = %v/%v", decision, ok)
	}
}

func TestFlowCausalNestedRecurrencesKeepSeparateEdgeSupport(t *testing.T) {
	p := parseBindLower(t, `
while outer() do
  while inner() do tick() end
end
`)
	loops := p.Flow().Authored().Control().Loops()
	outer, outerOK := loops.At(0)
	inner, innerOK := loops.At(1)
	if !outerOK || !innerOK {
		t.Fatalf("nested Loops = %v/%v, %v/%v", outer, outerOK, inner, innerOK)
	}
	edges := p.Flow().Causal().Edges()
	outerFeedback, innerFeedback := -1, -1
	for index := 0; index < edges.Count(); index++ {
		_, edgeOK := edges.At(index)
		if !edgeOK || !edges.ResetContains(index, inner) {
			continue
		}
		if edges.ResetContains(index, outer) {
			outerFeedback = index
		} else {
			innerFeedback = index
		}
	}
	if outerFeedback < 0 || innerFeedback < 0 {
		t.Fatalf("nested recurrence edges outer=%d inner=%d", outerFeedback, innerFeedback)
	}
	if !edges.ResetContains(outerFeedback, outer) || !edges.ResetContains(outerFeedback, inner) {
		t.Fatal("outer recurrence reset did not cover its nested Loop decision")
	}
	if !edges.ResetContains(innerFeedback, inner) {
		t.Fatal("inner recurrence reset did not cover its own Loop decision")
	}
}

func TestFlowCausalResetQueriesDoNotAllocate(t *testing.T) {
	p := parseBindLower(t, `while left() and right() do tick() end`)
	loop, ok := p.Flow().Authored().Control().Loops().At(0)
	if !ok {
		t.Fatal("missing Loop")
	}
	edges := p.Flow().Causal().Edges()
	feedback := -1
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if edgeOK && edge.Mu == loop {
			feedback = index
			break
		}
	}
	if feedback < 0 {
		t.Fatal("missing recurrence Edge")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		edgeResetCountSink, edgeResetBoolSink = edges.ResetCount(feedback)
		edgeResetTermSink, edgeResetBoolSink = edges.ResetAt(feedback, 0)
		edgeResetBoolSink = edges.ResetContains(feedback, loop)
	}); allocations != 0 {
		t.Fatalf("Causal reset queries allocate %v times", allocations)
	}
}

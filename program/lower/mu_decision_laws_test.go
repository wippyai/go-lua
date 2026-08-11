package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
)

// recurrenceResetEdge finds the one retained final Edge that carries a Mu
// head. Reset membership belongs to that Edge; there is no Program-wide Mu
// decision query.
func recurrenceResetEdge(t *testing.T, p *program.Program, head keyspace.Term) int {
	t.Helper()
	edges := p.Flow().Causal().Edges()
	for index := 0; index < edges.Count(); index++ {
		edge, ok := edges.At(index)
		if ok && edge.Mu == head {
			return index
		}
	}
	t.Fatalf("no final causal Edge carries Mu %v", head)
	return -1
}

func requireEdgeReset(t *testing.T, p *program.Program, edgeIndex int, want ...keyspace.Term) {
	t.Helper()
	edges := p.Flow().Causal().Edges()
	count, ok := edges.ResetCount(edgeIndex)
	if !ok || count != len(want) {
		t.Fatalf("Edge %d ResetCount = %d/%v, want %d", edgeIndex, count, ok, len(want))
	}
	for offset := 0; offset < count; offset++ {
		decision, decisionOK := edges.ResetAt(edgeIndex, offset)
		if !decisionOK || !edges.ResetContains(edgeIndex, decision) {
			t.Fatalf("Edge %d ResetAt(%d) = %v/%v without membership", edgeIndex, offset, decision, decisionOK)
		}
	}
	for _, decision := range want {
		if !edges.ResetContains(edgeIndex, decision) {
			t.Fatalf("Edge %d reset omits decision %v", edgeIndex, decision)
		}
	}
}

func TestFinalLoopResetOwnsOnlyReevaluatedDecisions(t *testing.T) {
	p := parseBindLower(t, `while left() and right() do tick() end`)
	loop, ok := p.Flow().Authored().Control().Loops().At(0)
	if !ok {
		t.Fatal("Loop is absent")
	}
	selection, ok := p.Flow().Authored().Operators().Selects().At(0)
	if !ok {
		t.Fatal("loop short-circuit Select is absent")
	}
	recurrence := recurrenceResetEdge(t, p, loop)
	requireEdgeReset(t, p, recurrence, selection, loop)
}

func TestFinalNestedLoopResetsRemainHeadLocal(t *testing.T) {
	p := parseBindLower(t, `
while outer() do
  while inner() do tick() end
end
`)
	loops := p.Flow().Authored().Control().Loops()
	outer, outerOK := loops.At(0)
	inner, innerOK := loops.At(1)
	if !outerOK || !innerOK {
		t.Fatal("nested Loops are absent")
	}
	requireEdgeReset(t, p, recurrenceResetEdge(t, p, outer), outer, inner)
	requireEdgeReset(t, p, recurrenceResetEdge(t, p, inner), inner)
}

func TestFinalBackwardGotoRetainsAnEmptyResetInterval(t *testing.T) {
	p := parseBindLower(t, "::again::; goto again")
	label, ok := p.Flow().Authored().Control().Labels().At(0)
	if !ok {
		t.Fatal("Label is absent")
	}
	recurrence := recurrenceResetEdge(t, p, label)
	requireEdgeReset(t, p, recurrence)
}

func TestFinalResetQueriesAreAllocationFree(t *testing.T) {
	p := parseBindLower(t, `while left() and right() do tick() end`)
	loop, _ := p.Flow().Authored().Control().Loops().At(0)
	edge := recurrenceResetEdge(t, p, loop)
	if count, ok := p.Flow().Causal().Edges().ResetCount(edge); !ok || count != 2 {
		t.Fatalf("ResetCount = %d/%v, want 2", count, ok)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = p.Flow().Causal().Edges().ResetCount(edge)
		_, _ = p.Flow().Causal().Edges().ResetAt(edge, 0)
		_ = p.Flow().Causal().Edges().ResetContains(edge, loop)
	})
	if allocations != 0 {
		t.Fatalf("final Edge reset queries allocate %f times", allocations)
	}
}

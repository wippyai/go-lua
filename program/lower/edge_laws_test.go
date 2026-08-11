package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

var (
	edgeCountSink int
	edgeSink      keyspace.Term
	edgeBoolSink  bool
)

// Causal Edges are immutable Flow rows.  They are selected through a Body or
// activation owner; there is no Program-level edge capability or parallel CFG.
func TestFlowCausalEdgesAreIndexedByExactBodyAndActivation(t *testing.T) {
	p := parseBindLower(t, `
if first() then second() end
while third() do fourth() end
return fifth()
`)
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	edges := p.Flow().Causal().Edges()
	bodyCount, bodyOK := edges.BodyCount(entry)
	activationCount, activationOK := edges.ActivationCount(entry)
	if !bodyOK || !activationOK || bodyCount == 0 || activationCount == 0 {
		t.Fatalf("entry edge counts body=%d/%v activation=%d/%v", bodyCount, bodyOK, activationCount, activationOK)
	}
	calls := p.Flow().Authored().Calls()
	for index := 0; index < activationCount; index++ {
		edge, edgeOK := edges.ActivationAt(entry, index)
		if !edgeOK || edge.From == 0 || edge.To == 0 {
			t.Fatalf("activation Edge[%d] = %#v/%v", index, edge, edgeOK)
		}
		if _, _, _, _, callOK := calls.Get(edge.From); callOK {
			t.Fatalf("Call %v bypasses its Causal boundary through Edge %#v", edge.From, edge)
		}
	}
	if edge, edgeOK := edges.ActivationAt(entry, activationCount); edgeOK || edge.From != 0 {
		t.Fatalf("ActivationAt past end = %#v/%v", edge, edgeOK)
	}
	for index := 0; index < bodyCount; index++ {
		edge, edgeOK := edges.BodyAt(entry, index)
		if !edgeOK || edge.From == 0 || edge.To == 0 {
			t.Fatalf("body Edge[%d] = %#v/%v", index, edge, edgeOK)
		}
	}
}

func TestFlowCausalBoundariesCutExecutableCalls(t *testing.T) {
	p := parseBindLower(t, `
if invoke() then end
while retry() do break end
return finish()
`)
	edges := p.Flow().Causal().Edges()
	boundaries := p.Flow().Causal().Boundaries()
	calls := p.Flow().Authored().Calls()
	if calls.Count() != 3 {
		t.Fatalf("Call count = %d, want 3", calls.Count())
	}
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		boundary, boundaryOK := boundaries.For(call)
		if !boundaryOK || boundary.Call != call || boundary.Normal == 0 || boundary.Throw == 0 || boundary.Yield == 0 || boundary.Cancel == 0 {
			t.Fatalf("Call boundary[%d] = %#v/%v", index, boundary, boundaryOK)
		}
		for edgeIndex := 0; edgeIndex < edges.Count(); edgeIndex++ {
			edge, edgeOK := edges.At(edgeIndex)
			if edgeOK && edge.From == call {
				t.Fatalf("Call %v bypasses boundary through Edge %#v", call, edge)
			}
		}
	}
}

func TestFlowCausalEdgeQueriesDoNotAllocate(t *testing.T) {
	p := parseBindLower(t, `while test() do tick() end`)
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	edges := p.Flow().Causal().Edges()
	count, ok := edges.ActivationCount(entry)
	if !ok || count == 0 {
		t.Fatalf("ActivationCount = %d/%v", count, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		edgeCountSink, edgeBoolSink = edges.ActivationCount(entry)
		edge, edgeOK := edges.ActivationAt(entry, 0)
		edgeBoolSink = edgeOK
		edgeSink = edge.From
		edgeCountSink, edgeBoolSink = edges.BodyCount(entry)
	}); allocations != 0 {
		t.Fatalf("Flow Causal edge queries allocate %v times", allocations)
	}
}

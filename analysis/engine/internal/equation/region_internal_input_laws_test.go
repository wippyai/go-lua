package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func regionInputKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

// A Region owns the exact membership of every ordered Group input occurrence
// it encloses. An internal Group publishes which of its own input ordinals lie
// inside the Region, and the complementary ordinals are that Region's named
// interfaces. Neither half may be rediscovered by a consumer re-testing Point
// containment.
func TestRegionInternalGroupInputOrdinalsLaw(t *testing.T) {
	factor := regionInputKey(1)
	bodyRule, backRule := regionInputKey(2), regionInputKey(3)
	bodyFamily, backFamily := regionInputKey(4), regionInputKey(5)
	read := Surface{Factor: factor, Form: SurfaceReadExact, Local: 1}
	write := Surface{Factor: factor, Form: SurfaceWriteExact, Local: 1, Mode: TargetModeStrong}
	cold, coldOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Rules: []composition.Rule{{
			Key: bodyRule, OperandFamily: bodyFamily,
			OutputKind: composition.FactorOutput, Output: factor, Inputs: 1,
			Reads:  []composition.Read{{Kind: composition.ReadExact, Input: 0, Factor: factor}},
			Writes: []composition.Write{{Kind: composition.WriteExact, Factor: factor}},
		}, {
			Key: backRule, OperandFamily: backFamily,
			OutputKind: composition.FactorOutput, Output: factor, Inputs: 2,
			Reads: []composition.Read{
				{Kind: composition.ReadExact, Input: 0, Factor: factor},
				{Kind: composition.ReadExact, Input: 1, Factor: factor},
			},
			Writes: []composition.Write{{Kind: composition.WriteExact, Factor: factor}},
		}},
	})
	if !coldOK || cold == nil {
		t.Fatal("cold composition")
	}

	scope := EmptyScope()
	batch := NewBatch()
	entry, entryOK := batch.AdmitSite(regionInputKey(10), scope, TrueExpr(), InitPresent)
	head, headOK := batch.AdmitSite(regionInputKey(11), scope, TrueExpr(), InitPresent)
	body, bodyOK := batch.AdmitSite(regionInputKey(12), scope, TrueExpr(), InitPresent)
	if !entryOK || !headOK || !bodyOK {
		t.Fatal("recurrence sites")
	}
	headOccurrence, headOccurrenceOK := batch.At(head)
	bodyOccurrence, bodyOccurrenceOK := batch.At(body)
	headOperand, headOperandOK := batch.AdmitOperand(headOccurrence, regionInputKey(13))
	bodyOperand, bodyOperandOK := batch.AdmitOperand(bodyOccurrence, regionInputKey(14))
	if !headOccurrenceOK || !bodyOccurrenceOK || !headOperandOK || !bodyOperandOK || !batch.Seal() {
		t.Fatal("recurrence batch")
	}

	// entry -> head -> body -> head. The recurrence encloses head and body; the
	// entry Point is the only source outside it.
	headFromBody := BoundaryInput(body, head, regionInputKey(20), TrueExpr(), IdentityReindex(scope), TrueExpr())
	headFromEntry := BoundaryInput(entry, head, regionInputKey(21), TrueExpr(), IdentityReindex(scope), TrueExpr())
	bodyFromHead := BoundaryInput(head, body, regionInputKey(22), TrueExpr(), IdentityReindex(scope), TrueExpr())
	if !headFromBody.Available() || !headFromEntry.Available() || !bodyFromHead.Available() {
		t.Fatal("recurrence inputs")
	}
	topology, sealed := SealTopology(cold, TopologySpec{
		Batch: batch,
		Rules: []RuleInstance{{
			Schema: bodyRule, OperandFamily: bodyFamily, Occurrence: bodyOccurrence, Operand: bodyOperand,
			Reads:  []ResolvedRead{{Index: 0, Surface: read}},
			Writes: []ResolvedWrite{{Index: 0, Surface: write}},
		}, {
			Schema: backRule, OperandFamily: backFamily, Occurrence: headOccurrence, Operand: headOperand,
			Reads:  []ResolvedRead{{Index: 0, Surface: read}, {Index: 1, Surface: read}},
			Writes: []ResolvedWrite{{Index: 0, Surface: write}},
		}},
		Points: []PointSpec{{Site: entry}, {Site: head}, {Site: body}},
		Groups: []Group{
			{Members: []RuleRef{RuleAt(1)}, Output: PointAt(1), Inputs: []Input{headFromEntry, headFromBody}},
			{Members: []RuleRef{RuleAt(0)}, Output: PointAt(2), Inputs: []Input{bodyFromHead}},
		},
	})
	if !sealed || topology == nil {
		t.Fatal("recurrence topology seal")
	}
	graph, graphOK := initialGraph(topology)
	if !graphOK || graph == nil || graph.RegionCount() != 1 {
		t.Fatal("recurrence graph")
	}
	region, regionOK := graph.RegionAt(0)
	if !regionOK {
		t.Fatal("recurrence region")
	}
	headPoint, headPointOK := region.Head()
	if !headPointOK || headPoint.Site().Key() != head.Key() {
		t.Fatal("recurrence head")
	}

	backIndex, bodyIndex := -1, -1
	for index := 0; index < region.InternalGroupCount(); index++ {
		group, groupOK := region.InternalHyperedgeAt(index)
		if !groupOK {
			t.Fatal("internal Group")
		}
		switch group.Output().Site().Key() {
		case head.Key():
			backIndex = index
		case body.Key():
			bodyIndex = index
		}
	}
	if backIndex < 0 || bodyIndex < 0 {
		t.Fatal("recurrence producers were not published as internal Groups")
	}
	back, backOK := region.InternalHyperedgeAt(backIndex)
	if !backOK || back.InputCount() != 2 {
		t.Fatal("back Group inputs")
	}
	if region.InternalGroupInputCount(backIndex) != 1 {
		t.Fatalf("back Group internal input count = %d, want 1", region.InternalGroupInputCount(backIndex))
	}
	ordinal, ordinalOK := region.InternalGroupInputAt(backIndex, 0)
	if !ordinalOK {
		t.Fatal("back Group internal input ordinal")
	}
	inside, insideOK := back.InputAt(ordinal)
	if !insideOK || ordinal != 1 || inside.Source().Key() != body.Key() {
		t.Fatalf("published internal ordinal %d did not name the Region-internal input", ordinal)
	}
	if _, over := region.InternalGroupInputAt(backIndex, 1); over {
		t.Fatal("internal input row exceeded its sealed interval")
	}

	// The body producer reads only the head, so its one ordinal is internal and
	// it contributes no interface.
	bodyGroup, bodyGroupOK := region.InternalHyperedgeAt(bodyIndex)
	bodyOrdinal, bodyOrdinalOK := region.InternalGroupInputAt(bodyIndex, 0)
	if !bodyGroupOK || !bodyOrdinalOK || region.InternalGroupInputCount(bodyIndex) != 1 || bodyOrdinal != 0 {
		t.Fatal("body producer internal input membership")
	}
	bodyInput, bodyInputOK := bodyGroup.InputAt(bodyOrdinal)
	if !bodyInputOK || bodyInput.Source().Key() != head.Key() {
		t.Fatal("body producer ordinal did not name its Region-internal input")
	}

	if region.InterfaceCount() != 1 {
		t.Fatalf("region interface count = %d, want 1", region.InterfaceCount())
	}
	external, externalOK := region.InterfaceAt(0)
	interfaceGroup, interfaceGroupOK := external.Group()
	interfaceInput, interfaceInputOK := external.Input()
	if !externalOK || !interfaceGroupOK || !interfaceInputOK {
		t.Fatal("region interface")
	}
	if interfaceGroup.Key() != back.Key() || interfaceInput.Source().Key() != entry.Key() {
		t.Fatal("region interface did not name the exact external Group input occurrence")
	}
	if _, over := region.InterfaceAt(1); over {
		t.Fatal("interface row exceeded its sealed interval")
	}
}

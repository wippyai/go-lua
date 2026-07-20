package transformer

import (
	"reflect"
	"testing"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPresenceConsequenceTopologyFreezesAcyclicCondensation(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	one := statekey.SymbolValue(symbol.ID(9901))
	two := statekey.SymbolValue(symbol.ID(9902))
	three := statekey.SymbolValue(symbol.ID(9903))
	inventory := presenceConsequenceInventory{stages: []presenceConsequenceStageInventory{{
		blocks: []presenceConsequenceBlockInventory{
			{valueWrites: []statekey.Value{one}, mayContradict: true},
			{valueWrites: []statekey.Value{two}, predecessors: []int{0}, mayContradict: true},
			{valueWrites: []statekey.Value{three}, predecessors: []int{1, 0}, mayContradict: true},
		},
	}}}
	topology, err := freezePresenceConsequenceTopology(domain, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if topology.segmentCount != 3 || topology.intermediateCoordinateCount() != 2 || topology.feedbackNodes != 0 {
		t.Fatalf("acyclic topology = segments %d intermediates %d feedback %d, want 3/2/0",
			topology.segmentCount, topology.intermediateCoordinateCount(), topology.feedbackNodes)
	}
	if topology.valueValues != 3 || topology.feasibilityBits != 3 || topology.coordinateValues != 0 || topology.laneValues != 0 {
		t.Fatalf("acyclic typed payload totals = coordinates %d values %d lanes %d feasibility %d",
			topology.coordinateValues, topology.valueValues, topology.laneValues, topology.feasibilityBits)
	}
	blocks := topology.stages[0].blocks
	for index, block := range blocks {
		if block.feedbackNode != -1 || block.payload.valueOffset != index || block.payload.valueCount != 1 ||
			block.payload.feasibility != index {
			t.Fatalf("acyclic block %d layout = %#v feedback %d", index, block.payload, block.feedbackNode)
		}
	}
	if got := blocks[2].predecessors; !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("condensation predecessors = %v, want deterministic [0 1]", got)
	}
}

func TestPresenceConsequenceTopologyAllocatesOneTypedFeedbackNodePerCyclicBlock(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	left := statekey.SymbolValue(symbol.ID(9911))
	right := statekey.SymbolValue(symbol.ID(9912))
	inventory := presenceConsequenceInventory{stages: []presenceConsequenceStageInventory{{
		reducerSkeleton: true,
		blocks: []presenceConsequenceBlockInventory{{
			valueWrites: []statekey.Value{right, left}, pathMutation: true,
			mayContradict: true, requiresFeedback: true,
		}},
	}}}
	topology, err := freezePresenceConsequenceTopology(domain, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if topology.segmentCount != 2 || topology.intermediateCoordinateCount() != 1 || topology.feedbackNodes != 1 {
		t.Fatalf("cyclic topology = segments %d intermediates %d feedback %d, want reducer+one SCC/1/1",
			topology.segmentCount, topology.intermediateCoordinateCount(), topology.feedbackNodes)
	}
	block := topology.stages[0].blocks[0]
	if block.feedbackNode != 0 || block.payload.valueOffset != 0 || block.payload.valueCount != 2 ||
		block.payload.laneOffset != 0 || block.payload.laneCount != len(domain.PathDescendantMutationParticipantLanes()) ||
		block.payload.feasibility != 0 {
		t.Fatalf("cyclic typed feedback layout = %#v node %d", block.payload, block.feedbackNode)
	}
	if !reflect.DeepEqual(block.payload.valueWrites, []statekey.Value{left, right}) {
		t.Fatalf("cyclic Values layout = %v, want canonical [%d %d]", block.payload.valueWrites, left, right)
	}
}

func TestPresenceConsequenceTopologyIsDeterministicAndRejectsUncondensedEdges(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	left := statekey.SymbolValue(symbol.ID(9921))
	right := statekey.SymbolValue(symbol.ID(9922))
	freeze := func(values []statekey.Value, predecessors []int) presenceConsequenceTopology {
		t.Helper()
		topology, err := freezePresenceConsequenceTopology(domain, presenceConsequenceInventory{stages: []presenceConsequenceStageInventory{{
			blocks: []presenceConsequenceBlockInventory{
				{valueWrites: []statekey.Value{left}},
				{valueWrites: values, predecessors: predecessors, requiresFeedback: true},
			},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		return topology
	}
	first := freeze([]statekey.Value{right, left}, []int{0})
	second := freeze([]statekey.Value{left, right}, []int{0})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("presence feedback topology changed with input order:\n%#v\n%#v", first, second)
	}
	_, err := freezePresenceConsequenceTopology(domain, presenceConsequenceInventory{stages: []presenceConsequenceStageInventory{{
		blocks: []presenceConsequenceBlockInventory{{predecessors: []int{0}}},
	}}})
	if err == nil {
		t.Fatal("self-edge escaped its SCC instead of being represented by RequiresFeedback")
	}
}

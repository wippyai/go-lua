package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

func TestCyclicArtifactDemandRetainsContractReverseReachability(t *testing.T) {
	body := testBody(91)
	entry := EntryParameter{Body: body, Name: "entry"}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "seed"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: testID(91)}, KernelID: "seed", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
		{Target: Coordinate{Body: body, Name: "loop"}, Entry: entry, Occurrence: Occurrence{Kind: "loop-control", ContractID: testID(92)}, KernelID: "loop", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
		{Target: Coordinate{Body: body, Name: "result"}, Entry: entry, Occurrence: Occurrence{Kind: "outcome", ContractID: testID(93)}, KernelID: "result", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
	}}
	cells := []CellID{"seed", "loop", "result"}
	plan, err := solve.FreezeWTOPlan(cells,
		[]solve.WTOElement[CellID]{{Vertex: "seed"}, {Vertex: "loop", Body: []solve.WTOElement[CellID]{}}, {Vertex: "result"}},
		[]solve.WTOInfluence[CellID]{{From: "seed", To: "loop"}, {From: "loop", To: "loop"}, {From: "loop", To: "result"}})
	if err != nil {
		t.Fatal(err)
	}
	cyclic, err := NewCyclicArtifact(artifact, map[Coordinate]CellID{
		artifact.Equations[0].Target: "seed", artifact.Equations[1].Target: "loop", artifact.Equations[2].Target: "result",
	}, plan, []SemanticDependency{
		{From: "seed", To: "loop", Reason: EdgeContractRead, Evidence: "contract/read"},
		{From: "loop", To: "loop", Reason: EdgeContractAdvance, Evidence: "contract/advance"},
		{From: "loop", To: "result", Reason: EdgeContractOutcome, Evidence: "contract/outcome"},
	}, []OutputSelector{{ID: "normal", Cells: []CellID{"result"}}}, []CellID{"seed", "loop"})
	if err != nil {
		t.Fatal(err)
	}
	demand, err := cyclic.Demand([]string{"normal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(demand) != 3 || demand[0] != "loop" || demand[1] != "result" || demand[2] != "seed" {
		t.Fatalf("demand = %#v, want complete reverse reachability", demand)
	}
	restricted, err := cyclic.RestrictPlan([]string{"normal"})
	if err != nil || restricted.ComponentCount() != 1 {
		t.Fatalf("restricted plan = %#v, %v", restricted, err)
	}
}

func TestCyclicArtifactRejectsUnscheduleableContractEdge(t *testing.T) {
	body := testBody(92)
	entry := EntryParameter{Body: body, Name: "entry"}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "a"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: testID(94)}, KernelID: "a", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
		{Target: Coordinate{Body: body, Name: "b"}, Entry: entry, Occurrence: Occurrence{Kind: "outcome", ContractID: testID(95)}, KernelID: "b", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
	}}
	plan, err := solve.FreezeWTOPlan([]CellID{"a", "b"}, []solve.WTOElement[CellID]{{Vertex: "a"}, {Vertex: "b"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewCyclicArtifact(artifact, map[Coordinate]CellID{artifact.Equations[0].Target: "a", artifact.Equations[1].Target: "b"}, plan,
		[]SemanticDependency{{From: "b", To: "a", Reason: EdgeContractRead}}, nil, nil)
	if err == nil {
		t.Fatal("unscheduled backward contract edge was accepted")
	}
}

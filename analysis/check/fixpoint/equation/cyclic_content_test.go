package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

func TestCyclicArtifactContentCommitsToFrozenSchedule(t *testing.T) {
	cyclic := cyclicContentFixture(t)
	first := cyclic.CanonicalBytes()
	if len(first) == 0 || !cyclic.ContentID().Valid() {
		t.Fatal("valid cyclic artifact has no canonical content")
	}

	changed := cyclic
	changed.WidenCells = nil
	if string(first) == string(changed.CanonicalBytes()) || cyclic.ContentID() == changed.ContentID() {
		t.Fatal("cyclic content omitted widening policy")
	}
}

func TestCyclicArtifactContentRejectsIncompleteCertificate(t *testing.T) {
	cyclic := cyclicContentFixture(t)
	delete(cyclic.CellForTarget, cyclic.Artifact.Equations[0].Target)
	if cyclic.CanonicalBytes() != nil || cyclic.ContentID().Valid() {
		t.Fatal("incomplete cyclic artifact acquired a content identity")
	}
}

func TestCyclicArtifactContentCanonicalizesMutableDirectoryOrder(t *testing.T) {
	body := testBody(124)
	entry := EntryParameter{Body: body, Name: "entry"}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "a"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: testID(124)}, KernelID: "a", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
		{Target: Coordinate{Body: body, Name: "b"}, Entry: entry, Occurrence: Occurrence{Kind: "outcome", ContractID: testID(125)}, KernelID: "b", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
	}}
	plan, err := solve.FreezeWTOPlan([]CellID{"a", "b"}, []solve.WTOElement[CellID]{{Vertex: "a"}, {Vertex: "b"}}, []solve.WTOInfluence[CellID]{{From: "a", To: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	cyclic, err := NewCyclicArtifact(artifact, map[Coordinate]CellID{artifact.Equations[0].Target: "a", artifact.Equations[1].Target: "b"}, plan,
		[]SemanticDependency{{From: "a", To: "b", Reason: EdgeContractRead, Evidence: "first"}, {From: "a", To: "b", Reason: EdgeContractOutcome, Evidence: "second"}},
		[]OutputSelector{{ID: "a", Cells: []CellID{"a"}}, {ID: "b", Cells: []CellID{"b"}}}, []CellID{"a", "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := cyclic.ContentID()
	cyclic.Dependencies[0], cyclic.Dependencies[1] = cyclic.Dependencies[1], cyclic.Dependencies[0]
	cyclic.Selectors[0], cyclic.Selectors[1] = cyclic.Selectors[1], cyclic.Selectors[0]
	cyclic.ParameterCells[0], cyclic.ParameterCells[1] = cyclic.ParameterCells[1], cyclic.ParameterCells[0]
	if cyclic.ContentID() != first {
		t.Fatal("content identity depends on mutable directory order")
	}
}

func cyclicContentFixture(t *testing.T) CyclicArtifact {
	t.Helper()
	body := testBody(123)
	entry := EntryParameter{Body: body, Name: "entry"}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "seed"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: testID(123)}, KernelID: "seed", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
	}}
	plan, err := solve.FreezeWTOPlan([]CellID{"seed"}, []solve.WTOElement[CellID]{{Vertex: "seed"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cyclic, err := NewCyclicArtifact(artifact, map[Coordinate]CellID{artifact.Equations[0].Target: "seed"}, plan,
		nil, []OutputSelector{{ID: "normal", Cells: []CellID{"seed"}}}, []CellID{"seed"}, []CellID{"seed"})
	if err != nil {
		t.Fatal(err)
	}
	return cyclic
}

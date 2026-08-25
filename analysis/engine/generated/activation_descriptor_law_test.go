package generated

import (
	"testing"

	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// activationPlanLawSpec is the sealed call-activation shape: one exact read of
// the candidate, one structural publication, the transport vector each branch
// instantiates when it crosses its transition, and the relation plus
// identities its branches are mounted by. The branch set is enumerated through
// its owner and read nowhere.
func activationPlanLawSpec() CompiledRuleSpec {
	spec := heterogeneousPlanLawSpec()
	spec.Carry = nil
	spec.InputCount = 1
	// One read: the trigger's. The branch set is named by the vocabulary and
	// enumerated through its owner, so it is not among the reads.
	spec.Reads = spec.Reads[:1]
	spec.Activation = &ruleplan.Activation{
		Branch:      ruleplan.RelationAddr{Axis: 0, Member: 7},
		Application: ruleplan.ProjectionAddr{Axis: 0, Member: 12},
		Target:      ruleplan.ProjectionAddr{Axis: 0, Member: 13},
		Endpoint:    ruleplan.ProjectionAddr{Axis: 0, Member: 14},
		Mount:       ruleplan.ProjectionAddr{Axis: 0, Member: 15},
		Body:        ruleplan.ProjectionAddr{Axis: 0, Member: 16},
	}
	spec.Outputs = []OutputPlan{{
		Factor:      0,
		Axis:        0,
		Address:     ruleplan.OutputAddr{Axis: 0, Frame: 3},
		Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 11},
		Mode:        ruleprogram.ModeStructural,
		Slot:        0,
	}}
	spec.Reducer = ruleplan.ReducerAddr{Axis: 0, Member: 0}
	spec.Transports = []ruleplan.Transport{
		{Axis: 0, Exported: true},
		{Axis: 1},
		{Axis: 2, Exported: true},
	}
	return spec
}

// TestAStructuralPublicationCarriesItsTransportVector states what a structural
// output IS: an activation candidate route instantiating a transport across a
// transition. The vector is the publication's content, so a descriptor keeps it
// and answers it by ordinal, and the export direction is a property of the row.
func TestAStructuralPublicationCarriesItsTransportVector(t *testing.T) {
	descriptor, ok := NewPlanCompiledRule(activationPlanLawSpec())
	if !ok || !descriptor.Available() {
		t.Fatalf("structural activation descriptor refused: %+v/%t", descriptor, ok)
	}
	mode, modeOK := descriptor.OutputMode()
	if !modeOK || mode != ruleprogram.ModeStructural {
		t.Fatalf("structural output mode = %v/%t", mode, modeOK)
	}
	if descriptor.TransportCount() != 3 {
		t.Fatalf("transport census = %d", descriptor.TransportCount())
	}
	exported := 0
	for index := 0; index < descriptor.TransportCount(); index++ {
		row, rowOK := descriptor.TransportAt(index)
		if !rowOK {
			t.Fatalf("transport %d of a published census must resolve", index)
		}
		if row.Exported {
			exported++
		}
	}
	if exported != 2 {
		t.Fatalf("exported transports = %d, want the two rows that declared the return direction", exported)
	}
	if _, ok := descriptor.TransportAt(descriptor.TransportCount()); ok {
		t.Fatal("a transport beyond the published census is not a row of the vector")
	}
}

// TestAStructuralPublicationAndATransportVectorImplyEachOther is the
// biconditional that keeps the two halves from drifting apart. A structural
// publication with no vector instantiates nothing, and a vector on a rule that
// writes a fact is a transport no candidate route crosses: neither is a
// declaration the engine can execute, and neither is silently converted into
// the other.
func TestAStructuralPublicationAndATransportVectorImplyEachOther(t *testing.T) {
	unvectored := activationPlanLawSpec()
	unvectored.Transports = nil
	if descriptor, ok := NewPlanCompiledRule(unvectored); ok || descriptor.Available() {
		t.Fatal("a structural publication with no transport vector was admitted")
	}
	vectoredExact := heterogeneousPlanLawSpec()
	vectoredExact.Transports = []ruleplan.Transport{{Axis: 0}}
	if descriptor, ok := NewPlanCompiledRule(vectoredExact); ok || descriptor.Available() {
		t.Fatal("a fact-writing rule was admitted with an activation transport vector")
	}
}

// TestAStructuralPublicationIsNoExactWriteCapability keeps the capability
// evidence honest: Exact and Strong are the exact writer's proof, and a
// structural publication stages no fact, so a descriptor claiming both is two
// dispositions for one output.
func TestAStructuralPublicationIsNoExactWriteCapability(t *testing.T) {
	for name, damage := range map[string]func(*OutputPlan){
		"exact":  func(output *OutputPlan) { output.Exact = true },
		"strong": func(output *OutputPlan) { output.Strong = true },
		"route":  func(output *OutputPlan) { output.RouteJoinPresent = true; output.RouteJoin = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			spec := activationPlanLawSpec()
			damage(&spec.Outputs[0])
			if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
				t.Fatalf("a structural publication claimed the %s disposition", name)
			}
		})
	}
}

// TestATransportedAxisIsOneOfTheSealedAxes fences the vector against the same
// axis directory as every other descriptor address.
func TestATransportedAxisIsOneOfTheSealedAxes(t *testing.T) {
	spec := activationPlanLawSpec()
	spec.Transports = []ruleplan.Transport{{Axis: uint32(spec.AxisCount)}}
	if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
		t.Fatal("a transport named an axis outside the sealed directory")
	}
}

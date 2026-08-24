package generated

import (
	"testing"

	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// TestASealedDescriptorCarriesNoCoordinateOfItsOwn is the one-assigner law at
// the descriptor.
//
// A descriptor is a ROW of the sealed rule table and its coordinate is its
// POSITION there. The seal assigns that position once; a copy kept inside the
// row would be a second place the same number lives, and the only thing such a
// copy can do later is disagree - which is what rebasing it on every reseal
// existed to paper over. So two descriptors sealed from one specification are
// equal, and nothing about where either is placed can distinguish them.
func TestASealedDescriptorCarriesNoCoordinateOfItsOwn(t *testing.T) {
	spec := addressingLawSpec(exactAddressedRead())
	first, firstOK := NewPlanCompiledRule(spec)
	second, secondOK := NewPlanCompiledRule(spec)
	if !firstOK || !secondOK || !first.Available() || !second.Available() {
		t.Fatalf("one specification did not seal twice: %t/%t", firstOK, secondOK)
	}
	firstRead, firstReadOK := first.ReadAt(0)
	secondRead, secondReadOK := second.ReadAt(0)
	if !firstReadOK || !secondReadOK || firstRead != secondRead {
		t.Fatal("two descriptors of one specification differ in their sealed geometry")
	}
	if first.CandidateRelation() != second.CandidateRelation() || first.Reducer() != second.Reducer() ||
		first.OutputAddress() != second.OutputAddress() || first.InputCount() != second.InputCount() {
		t.Fatal("two descriptors of one specification differ")
	}
	firstMode, firstModeOK := first.OutputMode()
	secondMode, secondModeOK := second.OutputMode()
	if !firstModeOK || !secondModeOK || firstMode != secondMode || firstMode != ruleprogram.ModeExact {
		t.Fatal("two descriptors of one specification differ in their publication")
	}
}

// TestAnIssuedCandidateDescriptorIsStillCoordinateless keeps the other arm
// honest. An issued candidate already means the rule's rows are Program rows
// rather than a directory's; it must not become the one shape that carries a
// rule number, because the table's position is what names it either way.
func TestAnIssuedCandidateDescriptorIsStillCoordinateless(t *testing.T) {
	spec := planLawSpec([]ReadPlan{}, nil, 0)
	spec.IssuedCandidate = true
	spec.Candidate = ruleplan.RelationAddr{}
	spec.Outputs[0].Mode = ruleprogram.ModeExact
	first, firstOK := NewPlanCompiledRule(spec)
	second, secondOK := NewPlanCompiledRule(spec)
	if !firstOK || !secondOK {
		t.Fatalf("an issued-candidate specification did not seal twice: %t/%t", firstOK, secondOK)
	}
	if first.IssuedCandidate() != second.IssuedCandidate() || first.CandidateRelation() != second.CandidateRelation() {
		t.Fatal("two issued-candidate descriptors of one specification differ")
	}
}

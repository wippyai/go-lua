package transformer

import "testing"

func TestCyclicDependencyCertificateCopiesFrozenPlanAndTypedEdges(t *testing.T) {
	loop := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeLoopMu, binder: loopMuTerm(1), body: 2, exits: []relationRootRef{3}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: loopMuTerm(1)}}},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	program := formalRegionTestProgram(loop)
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	certificate, err := program.CyclicDependencyCertificate()
	if err != nil {
		t.Fatal(err)
	}
	if certificate.Plan == nil || !certificate.Plan.Matches(certificate.Cells) || certificate.Plan.ComponentCount() == 0 {
		t.Fatalf("certificate plan = %#v, want copied cyclic production schedule", certificate.Plan)
	}
	if len(certificate.Dependencies) == 0 {
		t.Fatal("certificate omitted region influences")
	}
	for _, dependency := range certificate.Dependencies {
		if !certificate.Plan.CoversInfluence(dependency.From, dependency.To) || dependency.Evidence == "" {
			t.Fatalf("certificate dependency is not frozen/auditable: %#v", dependency)
		}
	}
}

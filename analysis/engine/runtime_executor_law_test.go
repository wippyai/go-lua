package engine

import (
	"context"
	"testing"
)

func TestRegionPostfixCertificateRejectsExactRecomputeWithUnchangedInputs(t *testing.T) {
	runtime := &solverRuntime{regions: []runtimeRegion{{active: true, head: 0}}, activeRegions: []bool{true}}
	epoch := &executorEpoch{
		runtime: runtime,
		regions: []regionEpoch{{phase: phaseAscent, episode: 1, hasExact: true}},
	}
	epoch.operands.clock = 1
	if !epoch.rememberRegionPostfix(0) || !epoch.regionPostfixProved(0) {
		t.Fatal("initial postfix certificate was not admitted")
	}
	if !epoch.regions[0].dropPostfixProof() {
		t.Fatal("exact recomputation did not drop the certificate")
	}
	if epoch.regionPostfixProved(0) {
		t.Fatal("stale postfix certificate survived exact recomputation")
	}
	if !epoch.rememberRegionPostfix(0) || !epoch.regionPostfixProved(0) {
		t.Fatal("a fresh certificate was refused after the drop")
	}
	epoch.regions[0].backAt = epoch.operands.clock
	if epoch.regionPostfixProved(0) {
		t.Fatal("a back ingress mark taken after the certificate left it usable")
	}
}

func TestRuntimeInstantiatesOnlyDemandedProducerGroups(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	if len(fixture.solver.runtime.producers) != fixture.solver.runtime.graph.GroupCount() {
		t.Fatal("runtime producer table is not graph-total")
	}
	for index, producer := range fixture.solver.runtime.producers {
		if producer.span.count() == 0 || !producer.group.Key().Available() {
			t.Fatalf("producer %d has no sealed descriptor", index)
		}
	}
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("demanded solve state=%t status=%v", state != nil, status)
	}
}

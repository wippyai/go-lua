package engine

import (
	"context"
	"testing"
)

func TestRegionPostfixCertificateRejectsExactRecomputeWithUnchangedInputs(t *testing.T) {
	runtime := &solverRuntime{regions: []runtimeRegion{{active: true, head: 0}}, activeRegions: []bool{true}}
	epoch := &executorEpoch{
		runtime:  runtime,
		versions: []uint64{7},
		regions:  []regionEpoch{{phase: phaseAscent, episode: 1, hasExact: true, exactInputsVersion: 3, exactRevision: 1}},
	}
	if !epoch.rememberRegionPostfix(0) || !epoch.regionPostfixProved(0) {
		t.Fatal("initial postfix certificate was not admitted")
	}
	if !epoch.regions[0].nextExactRevision() || epoch.regions[0].exactRevision != 2 {
		t.Fatal("exact revision did not advance")
	}
	if epoch.regionPostfixProved(0) {
		t.Fatal("stale postfix certificate survived exact recomputation")
	}
	epoch.regions[0].exactRevision = ^uint64(0)
	if epoch.regions[0].nextExactRevision() || epoch.regionPostfixProved(0) {
		t.Fatal("exact revision overflow retained a usable certificate")
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

package engine

import "testing"

func TestRegionPostfixCertificateRejectsExactRecomputeWithUnchangedInputs(t *testing.T) {
	runtime := &solverRuntime{
		regions:       []runtimeRegion{{active: true, head: 0}},
		activeRegions: []bool{true},
	}
	epoch := &executorEpoch{
		runtime:  runtime,
		versions: []uint64{7},
		regions:  []regionEpoch{{phase: phaseAscent, episode: 1, hasExact: true, exactInputsVersion: 3, exactRevision: 1}},
	}
	if !epoch.rememberRegionPostfix(0) || !epoch.regionPostfixProved(0) {
		t.Fatal("initial postfix certificate was not admitted")
	}
	if epoch.regions[0].exactInputsVersion != 3 {
		t.Fatal("test changed exact-input evidence before recompute")
	}
	if !epoch.regions[0].nextExactRevision() || epoch.regions[0].exactRevision != 2 {
		t.Fatal("exact revision did not advance")
	}
	if epoch.regionPostfixProved(0) {
		t.Fatal("stale postfix certificate survived exact recomputation")
	}

	epoch.regions[0].exactRevision = ^uint64(0)
	if epoch.regions[0].nextExactRevision() {
		t.Fatal("exact revision wrapped instead of failing closed")
	}
	if epoch.regionPostfixProved(0) {
		t.Fatal("overflow retained a usable postfix certificate")
	}
}

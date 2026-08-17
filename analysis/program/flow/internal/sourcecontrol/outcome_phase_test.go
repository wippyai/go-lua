package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestOutcomePhasePathIsDistinctAndStableFromOutcomePath(t *testing.T) {
	bodyPath, outcomePath := identity.ContentID{7}, identity.ContentID{8}
	tail := vertexPhasePath(vertexBodyTailDomain, bodyPath)
	phase := vertexPhasePath(vertexOutcomePhaseDomain, outcomePath)
	if !tail.Available() || !phase.Available() || tail == phase {
		t.Fatalf("BodyTail/OutcomePhase paths = %x/%x, want distinct available paths", tail, phase)
	}
	if replay := vertexPhasePath(vertexOutcomePhaseDomain, outcomePath); replay != phase {
		t.Fatalf("OutcomePhase replay = %x, want %x", replay, phase)
	}
}

func TestPhaseRefClassRejectsForgedOutcomeTag(t *testing.T) {
	result := lifecycleLawResult()
	csr, ok := result.phaseRefAt(0)
	if !ok || csr.OutcomePhase() {
		t.Fatal("ordinary CSR phase was classified as Outcome phase")
	}
	forged := csr
	forged.class = phaseInvalid
	if forged.Available() || forged.OutcomePhase() {
		t.Fatal("forged phase class remained available")
	}
}

func TestOutcomePhaseOrderPreservesChildBeforeParentFanInAndCoverage(t *testing.T) {
	path := func(value byte) identity.ContentID { return identity.ContentID{value} }
	// Deliberately unsorted input: radix order is the semantic tie-break for
	// the two independent children, then Kahn releases their shared parent.
	candidates := []outcomePhaseCandidate{
		{path: path(4)},
		{path: path(2), parent: path(3)},
		{path: path(3), parent: path(4)},
		{path: path(1), parent: path(3)},
	}
	identity.SortByContentID(candidates, outcomePhaseCandidatePath)
	ordered, ok := outcomePhaseOrder(candidates)
	if !ok || len(ordered) != len(candidates) {
		t.Fatalf("fan-in Outcome order = %d/%v", len(ordered), ok)
	}
	for index, want := range []identity.ContentID{path(1), path(2), path(3), path(4)} {
		got, available := ordered[index].VertexPath()
		if !available || got != want {
			t.Fatalf("fan-in order[%d] = %x/%v, want %x", index, got, available, want)
		}
	}
}

func TestOutcomePhaseOrderKeepsDeepParentChainComplete(t *testing.T) {
	const depth = 96
	candidates := make([]outcomePhaseCandidate, depth)
	for index := range candidates {
		path := identity.ContentID{byte(index + 1)}
		candidates[index].path = path
		if index+1 < len(candidates) {
			candidates[index].parent = identity.ContentID{byte(index + 2)}
		}
	}
	ordered, ok := outcomePhaseOrder(candidates)
	if !ok || len(ordered) != depth {
		t.Fatalf("deep Outcome order = %d/%v, want %d", len(ordered), ok, depth)
	}
	for index, phase := range ordered {
		path, available := phase.VertexPath()
		if !available || path != (identity.ContentID{byte(index + 1)}) {
			t.Fatalf("deep Outcome order[%d] = %x/%v", index, path, available)
		}
	}
}

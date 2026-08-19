package sourcecontrol

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func lifecycleLawResult() *Result {
	id := identity.ContentID{1}
	state := &catalogLifecycle{
		data:  vertexCatalog{paths: []identity.ContentID{{2}}, canonicalNodes: []uint32{0}},
		phase: catalogInstalled,
	}
	result := &Result{
		sourceID: id, flowID: id, staticID: id, moduleID: id,
		coordinates: coordinateProof{nodeCount: 1}, catalog: state,
	}
	state.owner = result
	return result
}

// A Result copy shares the lifecycle state: releasing the assembler owner
// invalidates every copy and cannot be bypassed by retaining the old payload.
func TestVertexCatalogCopiedResultSharesTerminalRelease(t *testing.T) {
	result := lifecycleLawResult()
	lease := &VertexCatalogLease{state: result.catalog, owner: result}
	clone := *result
	if !clone.VertexCatalogAvailable() {
		t.Fatal("copied Result did not observe installed catalog")
	}
	if !result.ReleaseVertexCatalog(lease) {
		t.Fatal("owner release rejected")
	}
	if clone.VertexCatalogAvailable() {
		t.Fatal("copied Result retained released catalog")
	}
	if _, ok := clone.VertexPathAt(0); ok {
		t.Fatal("copied Result queried a released path")
	}
	if clone.ReleaseVertexCatalog(lease) {
		t.Fatal("copied Result released with foreign owner token")
	}
}

func TestVertexCatalogRejectsForeignAndReusedLease(t *testing.T) {
	result := lifecycleLawResult()
	foreign := lifecycleLawResult()
	lease := &VertexCatalogLease{state: result.catalog, owner: result}
	if foreign.ReleaseVertexCatalog(lease) {
		t.Fatal("foreign Result accepted catalog lease")
	}
	if !result.ReleaseVertexCatalog(lease) {
		t.Fatal("first owner release rejected")
	}
	if result.ReleaseVertexCatalog(lease) {
		t.Fatal("reused catalog lease accepted")
	}
}

func TestVertexCatalogQueryReleaseIsCoherent(t *testing.T) {
	result := lifecycleLawResult()
	lease := &VertexCatalogLease{state: result.catalog, owner: result}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for index := 0; index < 1000; index++ {
			result.VertexCatalogAvailable()
			result.VertexPathAt(0)
			result.CanonicalNodeAt(0)
		}
	}()
	go func() {
		defer wg.Done()
		result.ReleaseVertexCatalog(lease)
	}()
	wg.Wait()
	if result.VertexCatalogAvailable() {
		t.Fatal("catalog remained available after terminal release")
	}
}

func TestVertexCatalogConcurrentDoubleReleaseHasOneWinner(t *testing.T) {
	result := lifecycleLawResult()
	lease := &VertexCatalogLease{state: result.catalog, owner: result}
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for index := 0; index < 2; index++ {
		go func() {
			defer wg.Done()
			results <- result.ReleaseVertexCatalog(lease)
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for released := range results {
		if released {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent lease release winners = %d, want 1", wins)
	}
}

func TestVertexCatalogPreInstallCopyCannotStealLifecycle(t *testing.T) {
	result := lifecycleLawResult()
	result.catalog.phase = catalogUninstalled
	result.catalog.data = vertexCatalog{}
	clone := *result
	if _, err := clone.InstallVertexCatalogLease(nil, nil); err == nil {
		t.Fatal("pre-install Result copy accepted catalog installation")
	}
	if _, err := result.InstallVertexCatalogLease(nil, nil); err == nil {
		t.Fatal("canonical Result accepted installation without exact vertex row")
	}
	// Both values still share one uninstalled lifecycle; a failed foreign
	// attempt did not fork or consume the canonical installation state.
	if result.catalog != clone.catalog || result.catalog.phase != catalogUninstalled {
		t.Fatal("failed pre-install attempt forked lifecycle state")
	}
}

func TestCarrierlessLoopOutcomeRequiresTypedOperationRole(t *testing.T) {
	result := lifecycleLawResult()
	csr, ok := result.phaseRefAt(0)
	if !ok {
		t.Fatal("CSR phase unavailable")
	}
	outcome := PhaseRef{result: result, path: identity.ContentID{3}, class: phaseOutcome, node: noNode}
	base := Segment{owner: result, from: csr, to: outcome, carrier: NoSegmentCarrier(), role: identity.ContentID{4}, relation: segmentRelationRootOutcome, fromFamily: keyspace.FamilyLoop}
	if base.valid() {
		t.Fatal("untagged carrierless Loop Outcome entered subdivision authority")
	}
	base.operation = operationOutcomeNumericFor
	if !base.valid() {
		t.Fatal("typed NumericFor Loop Outcome was rejected")
	}
}

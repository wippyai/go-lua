package sourcecontrol

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func lifecycleLawResult() *Result {
	id := keyspace.ContentID{1}
	state := &catalogLifecycle{
		data:  vertexCatalog{paths: []keyspace.ContentID{{2}}, canonicalNodes: []uint32{0}},
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
		t.Fatal("canonical Result accepted installation without exact receipt")
	}
	// Both values still share one uninstalled lifecycle; a failed foreign
	// attempt did not fork or consume the canonical installation state.
	if result.catalog != clone.catalog || result.catalog.phase != catalogUninstalled {
		t.Fatal("failed pre-install attempt forked lifecycle state")
	}
}

func TestOutcomePhasePathIsDistinctAndStableFromOutcomePath(t *testing.T) {
	bodyPath, outcomePath := keyspace.ContentID{7}, keyspace.ContentID{8}
	tail := vertexPhasePath(vertexBodyTailDomain, bodyPath)
	phase := vertexPhasePath(vertexOutcomePhaseDomain, outcomePath)
	if !tail.Available() || !phase.Available() || tail == phase {
		t.Fatalf("BodyTail/OutcomePhase paths = %x/%x, want distinct available paths", tail, phase)
	}
	if replay := vertexPhasePath(vertexOutcomePhaseDomain, outcomePath); replay != phase {
		t.Fatalf("OutcomePhase replay = %x, want %x", replay, phase)
	}
}

func TestOutcomePhaseReceiptForeignConsumeBurnsExactRetry(t *testing.T) {
	result := lifecycleLawResult()
	state := &outcomePhaseLifecycle{
		owner:  result,
		state:  outcomePhaseIssued,
		phases: []OutcomePhase{{path: keyspace.ContentID{9}}},
		byTerm: []keyspace.ContentID{{}, {9}},
	}
	result.outcomePhases = state
	receipt := &OutcomePhaseReceipt{state: state, owner: result}
	foreign := lifecycleLawResult()
	if _, ok := receipt.Consume(foreign); ok {
		t.Fatal("foreign Outcome-phase consume succeeded")
	}
	if _, ok := receipt.Consume(result); ok {
		t.Fatal("exact Outcome-phase retry survived foreign consume")
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

func TestOutcomeSubdivisionForeignConsumeBurnsAndCannotSpliceCSRCarrier(t *testing.T) {
	result := lifecycleLawResult()
	csr, ok := result.phaseRefAt(0)
	if !ok {
		t.Fatal("CSR phase unavailable")
	}
	outcome := PhaseRef{result: result, path: keyspace.ContentID{3}, class: phaseOutcome, node: noNode}
	result.outcomePhases = &outcomePhaseLifecycle{
		owner:     result,
		state:     outcomePhaseIssued,
		byTerm:    []keyspace.ContentID{{}, outcome.path},
		nonNormal: []bool{false, true},
	}
	from, to := keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	receipt := &RouteSegmentReceipt{
		state: &routeSegmentLifecycle{}, owner: result, from: csr, to: outcome,
		carrier: NoSegmentCarrier(), role: routeSegmentRole(from, to), relation: segmentRelationRootOutcome,
		operation: operationOutcomeReturn, fromFamily: keyspace.FamilyReturn,
	}
	foreign := lifecycleLawResult()
	if _, ok := receipt.Consume(foreign); ok {
		t.Fatal("foreign Outcome subdivision consume succeeded")
	}
	if _, ok := receipt.Consume(result); ok {
		t.Fatal("exact Outcome subdivision retry survived foreign consume")
	}
	if segment := (RouteSegment{from: csr, to: csr, carrier: NoSegmentCarrier(), role: routeSegmentRole(from, to), relation: segmentRelationRootOutcome, operation: operationOutcomeReturn, fromFamily: keyspace.FamilyReturn}); segment.valid() {
		t.Fatal("plain CSR pair entered Outcome subdivision authority")
	}
}

func TestCarrierlessLoopOutcomeRequiresTypedOperationRole(t *testing.T) {
	result := lifecycleLawResult()
	csr, ok := result.phaseRefAt(0)
	if !ok {
		t.Fatal("CSR phase unavailable")
	}
	outcome := PhaseRef{result: result, path: keyspace.ContentID{3}, class: phaseOutcome, node: noNode}
	base := RouteSegment{from: csr, to: outcome, carrier: NoSegmentCarrier(), role: keyspace.ContentID{4}, relation: segmentRelationRootOutcome, fromFamily: keyspace.FamilyLoop}
	if base.valid() {
		t.Fatal("untagged carrierless Loop Outcome entered subdivision authority")
	}
	base.operation = operationOutcomeNumericFor
	if !base.valid() {
		t.Fatal("typed NumericFor Loop Outcome was rejected")
	}
}

func TestCallTailReceiptForeignCopyConsumeBurnsExactRetry(t *testing.T) {
	result := lifecycleLawResult()
	foreign := lifecycleLawResult()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	exit := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	proof := &CallTailReturnReceipt{
		state: &callTailReturnLifecycle{}, owner: result,
		callID: routeTermID(call), exitID: routeTermID(exit), ownerID: routeTermID(body),
	}
	copyProof := *proof
	if copyProof.consume(foreign, call, exit, body) {
		t.Fatal("foreign copied Call-tail receipt consumed")
	}
	if proof.consume(result, call, exit, body) {
		t.Fatal("exact Call-tail retry survived foreign copied consume")
	}
}

func TestOutcomePhaseOrderPreservesChildBeforeParentFanInAndCoverage(t *testing.T) {
	path := func(value byte) keyspace.ContentID { return keyspace.ContentID{value} }
	// Deliberately unsorted input: radix order is the semantic tie-break for
	// the two independent children, then Kahn releases their shared parent.
	candidates := []outcomePhaseCandidate{
		{path: path(4)},
		{path: path(2), parent: path(3)},
		{path: path(3), parent: path(4)},
		{path: path(1), parent: path(3)},
	}
	radixOutcomePhaseCandidates(candidates)
	ordered, ok := outcomePhaseOrder(candidates)
	if !ok || len(ordered) != len(candidates) {
		t.Fatalf("fan-in Outcome order = %d/%v", len(ordered), ok)
	}
	for index, want := range []keyspace.ContentID{path(1), path(2), path(3), path(4)} {
		got, available := ordered[index].VertexPath()
		if !available || got != want {
			t.Fatalf("fan-in order[%d] = %x/%v, want %x", index, got, available, want)
		}
	}
}

func TestOutcomePhaseOrderKeepsDeepParentChainReceiptComplete(t *testing.T) {
	const depth = 96
	candidates := make([]outcomePhaseCandidate, depth)
	for index := range candidates {
		path := keyspace.ContentID{byte(index + 1)}
		candidates[index].path = path
		if index+1 < len(candidates) {
			candidates[index].parent = keyspace.ContentID{byte(index + 2)}
		}
	}
	ordered, ok := outcomePhaseOrder(candidates)
	if !ok || len(ordered) != depth {
		t.Fatalf("deep Outcome order = %d/%v, want %d", len(ordered), ok, depth)
	}
	for index, phase := range ordered {
		path, available := phase.VertexPath()
		if !available || path != (keyspace.ContentID{byte(index + 1)}) {
			t.Fatalf("deep Outcome order[%d] = %x/%v", index, path, available)
		}
	}
}

func TestTableFieldEligibilityForeignCopyConsumeBurnsExactRetry(t *testing.T) {
	result := lifecycleLawResult()
	foreign := lifecycleLawResult()
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	exit := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	proof := &TableFieldThrowEligibility{
		state: &tableFieldEligibilityState{}, owner: result,
		field: routeTermID(field), body: routeTermID(body), exit: routeTermID(exit),
	}
	copyProof := *proof
	if copyProof.consume(foreign, field, exit, body) {
		t.Fatal("foreign copied TableField eligibility receipt consumed")
	}
	if proof.consume(result, field, exit, body) {
		t.Fatal("exact TableField eligibility retry survived foreign copied consume")
	}
}

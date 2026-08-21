package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/snapshot"
	calldomain "github.com/wippyai/go-lua/domain/call"
	manifesttarget "github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// TestManifestFormalPlacementDispatchSolveAndPublish is the composite cut
// law for formal ownership. A provider signature is sealed through the
// manifest-to-Target adapter, mounted as a real Link artifact, and selected
// by the exact Call key/Target operation support before the Placement formal
// Rule widens the actual roots. The solve is read only through the typed
// Placement publication contract. If the current artifact occurrence seam is
// unavailable, the strongest manifest/selector law remains runnable and this
// end-to-end law records that predecessor explicitly rather than recreating a
// second artifact or legacy-result path.
func TestManifestFormalPlacementDispatchSolveAndPublish(t *testing.T) {
	target := sealCompositeFormalTarget(t)
	record, failure, mounted := mountFormalTargetRecord(t, target, "formal-placement-e2e", `
local formal = require("formal-placement")
local ownedFirst = { value = 1 }
local ownedSecond = { value = 2 }
local ownedThird = { value = 3 }
local sharedFirst = { value = 4 }
local sharedSecond = { value = 5 }
local sharedThird = { value = 6 }
local owned = formal.owned(ownedFirst, ownedSecond, ownedThird)
local shared = formal.shared(sharedFirst, sharedSecond, sharedThird)
return { owned = owned, shared = shared }
`)
	if !mounted {
		if failure.Stage() == artifactcompiler.CompileStageOccurrences && failure.Reason() == artifactcompiler.CompileReasonOccurrenceUnavailable {
			t.Fatalf("formal Placement solve cannot execute: artifact occurrence-unavailable at predecessor 908a7308f (move occurrences/rule placements to canonical Program rows); failure=%s", failure.Error())
		}
		t.Fatalf("compile and mount manifest formal fixture: %s", failure.Error())
	}
	if record.Source == nil || record.Source.Boundary() == nil || record.targetContract != target {
		t.Fatal("mounted formal fixture did not retain the exact manifest Target contract")
	}
	formalOperations := make(map[string]vocabulary.Operation, 2)
	for _, member := range []string{"owned", "shared"} {
		operation, operationOK := target.Operations.Lookup(vocabulary.BindingSpec{
			Namespace: vocabulary.BindingModule,
			Owner:     []string{"formal-placement"},
			Member:    []string{member},
		})
		if !operationOK {
			t.Fatalf("manifest Target omitted the %s formal operation", member)
		}
		if got := target.Operations.FormalEffectCount(operation); got == 0 {
			t.Fatalf("manifest Target omitted formal effects for %s", member)
		}
		formalOperations[member] = operation
	}
	if got := target.Operations.FormalEffectCount(formalOperations["owned"]); got != 2 {
		t.Fatalf("owned formal effect count = %d, want retain/store", got)
	}
	if got := target.Operations.FormalEffectCount(formalOperations["shared"]); got != 3 {
		t.Fatalf("shared formal effect count = %d, want send/export/opaque", got)
	}

	// The Call-owned actual denominator is redeemed from exact MountedCall
	// receipts. Static Call support is intentionally a may-envelope:
	// both provider operations can reach either syntactic call. The solve-time
	// Call factor, consumed by the formal rule, performs the actual operation
	// selection; this cold law authenticates the complete envelope without
	// pretending it is a dispatch result.
	formalInvocationCount := 0
	formalInvocations := make([]calldomain.MountedCall, 0, 2)
	for index := 0; index < record.CallAlgebra.MountedCallCount(); index++ {
		mounted, mountedOK := record.CallAlgebra.MountedCallAtHandle(index)
		if !mountedOK {
			t.Fatalf("mounted Call %d is unavailable", index)
		}
		actual, actualOK := mountedActualProjectionFor(record.CallAlgebra, record.PackSchema, mounted)
		if !actualOK {
			t.Fatalf("mounted actual projection %d did not redeem the exact Call receipt", index)
		}
		// The fixture also calls require; it belongs to the complete mounted
		// call denominator but is not one of the two three-actual formal calls.
		if actual.ActualCount() != 3 {
			continue
		}
		formalInvocationCount++
		formalInvocations = append(formalInvocations, mounted)
		key, keyOK := record.CallAlgebra.KeyForMountedCall(mounted)
		if !keyOK {
			t.Fatal("mounted formal Call did not retain its exact Call key")
		}
		support := make(map[vocabulary.Operation]int, len(formalOperations))
		for supportIndex := 0; supportIndex < record.CallAlgebra.SupportCount(key); supportIndex++ {
			candidate, candidateOK := record.CallAlgebra.SupportTargetAt(key, supportIndex)
			if !candidateOK || !record.CallAlgebra.OwnsTarget(candidate) {
				t.Fatalf("Call target support %d crossed its owner fence", supportIndex)
			}
			operation, operationOK := candidate.Operation()
			if operationOK {
				support[operation]++
			}
		}
		for _, operation := range formalOperations {
			if support[operation] != 1 {
				t.Fatalf("formal invocation support for operation %d = %d, want one may-target", operation, support[operation])
			}
		}
		applicationID, occurrenceID, moduleID, _, _, mountedIdentityOK := record.CallAlgebra.MountedCallIdentity(mounted)
		if !mountedIdentityOK {
			t.Fatal("mounted formal Call lost its mounted occurrence identity")
		}
		canonical, canonicalOK := record.CallAlgebra.MountedCallForOccurrence(moduleID, occurrenceID)
		if !canonicalOK || canonical != mounted || !applicationID.Available() {
			t.Fatal("mounted formal Call did not resolve through exact Call evidence")
		}
	}
	if formalInvocationCount != 2 {
		t.Fatalf("three-actual formal invocation count = %d, want owned and shared calls", formalInvocationCount)
	}
	formalExpectations := formalPlacementExpectations(t, record, formalInvocations)

	bound := materializerBinding(t, record)
	committed, sites := queryCanonicalProgram(t, record, bound)
	formalQuerySites := make(map[identity.ContentID]struct{}, len(formalExpectations))
	for _, site := range sites {
		if site.Family == QueryFamilyPlacementSummary {
			if _, formal := formalExpectations[site.Point]; formal {
				formalQuerySites[site.Point] = struct{}{}
			}
		}
	}
	if len(formalQuerySites) != len(formalExpectations) {
		t.Fatalf("selected Placement queries cover %d/%d declaration-framed formal call-effect cuts", len(formalQuerySites), len(formalExpectations))
	}
	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal formal Placement program: %v", sealFailure)
	}
	state, solveStatus := sealed.Solve(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		t.Fatalf("solve formal Placement program: status=%v state=%v", solveStatus, state)
	}
	published, publishedOK := sealed.PublishedSnapshot(state)
	if !publishedOK {
		t.Fatal("formal Placement solve published no snapshot")
	}
	view := published.Snapshot()
	queryPlan, queryPlanOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, published.QueryFamily())
	if !queryPlanOK {
		t.Fatal("open typed Placement query publication")
	}
	publications, publicationsOK := bound.QueryPublications(committed, sites)
	if !publicationsOK {
		t.Fatal("resolve typed Placement query publications")
	}
	allocationRootIDs := formalHeapAllocationRootIDs(t, record)
	placementHits := 0
	formalPointHits := 0
	for _, publication := range publications {
		if publication.Site.Family != QueryFamilyPlacementSummary {
			continue
		}
		answer, answerStatus := snapshot.Query(&view, queryPlan, publication.Key)
		if answerStatus == snapshot.ReadProvenAbsent {
			if _, expected := formalExpectations[publication.Site.Point]; expected {
				t.Fatalf("formal Placement point %s was proven absent", publication.Site.Point)
			}
			continue
		}
		if answerStatus != snapshot.ReadHit || !answer.Available() {
			t.Fatalf("typed Placement publication status = %s, want a hit", answerStatus)
		}
		cell, cellOK := publication.CanonicalCell(answer)
		if !cellOK || !cell.Available() || cell.ContractID() != publication.Contract().ContentID() {
			t.Fatal("Placement formal answer did not close under its typed contract")
		}
		result, resultOK := placementdomain.DecodeSummaryResult(record.PlacementSchema, cell.Present(), cell.RowCount(), cell.Payload())
		if !resultOK || !result.Available() || result.SchemaID() != record.PlacementSchema.ContentID() {
			t.Fatal("typed Placement publication did not decode under the mounted schema")
		}
		if result.AllocationCount() != len(allocationRootIDs) {
			t.Fatalf("typed Placement denominator = %d, want exact Heap allocation-root count %d", result.AllocationCount(), len(allocationRootIDs))
		}
		placementHits++
		if expected, formalPoint := formalExpectations[publication.Site.Point]; formalPoint {
			rows := decodeFormalPlacementRows(t, result)
			assertFormalPlacementRows(t, publication.Site.Point, rows, expected, allocationRootIDs)
			formalPointHits++
		}
	}
	if placementHits == 0 {
		t.Fatal("formal fixture published no typed Placement summary hit")
	}
	if formalPointHits != len(formalExpectations) {
		t.Fatalf("formal Placement points with exact root assertions = %d, want %d", formalPointHits, len(formalExpectations))
	}
}

// formalAllocationRoot is the cold fixture geometry used by the assertions
// below. Heap supplies the allocation Key and mounted Value identity; Value's
// allocation receipt independently proves that the row is a Program root.
// The fixture's authored table order then joins its six actual aliases to the
// first six Program roots without pretending an SSA/member ValueID is itself
// a Heap allocation identity.
type formalAllocationRoot struct {
	id         identity.ContentID
	valueID    identity.ContentID
	coordinate uint32
}

// formalPlacementExpectations joins each three-actual invocation to the exact
// sealed placement-formal call-effect point. The fixture's authored order is
// intentional: the first three-actual call is formal.owned and the second is
// formal.shared. Call support is checked separately above as a may-envelope;
// these expectations are read only from the selected solve points.
func formalPlacementExpectations(t testing.TB, record LinkInputs, invocations []calldomain.MountedCall) map[identity.ContentID]map[identity.ContentID]placementdomain.Placement {
	t.Helper()
	if len(invocations) != 2 {
		t.Fatalf("formal placement expectation invocations = %d, want two authored calls", len(invocations))
	}
	roots := formalAllocationRoots(t, record)
	if len(roots) != len(invocations)*3+1 {
		t.Fatalf("formal fixture Program allocation geometry = %d roots, want six call arguments plus one returned table", len(roots))
	}
	formalInvocationOrdinals(t, record, invocations)
	byFormalRoot := make(map[identity.ContentID]struct{}, len(invocations)*3)
	invocationRoots := make([][]identity.ContentID, len(invocations))
	for invocationIndex, mounted := range invocations {
		actuals, actualsOK := mountedActualProjectionFor(record.CallAlgebra, record.PackSchema, mounted)
		if !actualsOK {
			t.Fatalf("formal invocation %d did not redeem its exact Pack projection", invocationIndex)
		}
		if actuals.ActualCount() != 3 {
			t.Fatalf("formal invocation %d actual count = %d, want three", invocationIndex, actuals.ActualCount())
		}
		_, _, moduleID, _, _, mountedIdentityOK := record.CallAlgebra.MountedCallIdentity(mounted)
		if !mountedIdentityOK {
			t.Fatalf("formal invocation %d lost its mounted Call identity", invocationIndex)
		}
		invocationRoots[invocationIndex] = make([]identity.ContentID, actuals.ActualCount())
		for actualIndex := 0; actualIndex < actuals.ActualCount(); actualIndex++ {
			source, sourceOK := actuals.ActualAt(actualIndex)
			if !sourceOK || !source.Available() || source.Module() != moduleID || !record.PackSchema.OwnsSemanticSource(source) {
				t.Fatalf("formal invocation %d actual %d is unavailable", invocationIndex, actualIndex)
			}
			// The fixture deliberately authors six table allocations before the
			// returned result table. Artifact allocation rows preserve that
			// authored order, and Heap's dense RootAllocation rows preserve the
			// artifact order. The actual ValueID is an SSA/member alias (not the
			// allocation-root identity), so this cold geometry is the correct
			// owner join for the fixture.
			rootID := roots[invocationIndex*3+actualIndex].id
			coordinate, coordinateIssued := record.ValueSchema.CoordinateForMountedSemantic(source.Module(), source.ID())
			if !coordinateIssued {
				t.Fatalf("formal invocation %d actual %d has no Value coordinate", invocationIndex, actualIndex)
			}
			if _, coordinateOK := record.ValueSchema.CoordinateIndex(coordinate); !coordinateOK {
				t.Fatalf("formal invocation %d actual %d lost its Value coordinate owner", invocationIndex, actualIndex)
			}
			if _, duplicate := byFormalRoot[rootID]; duplicate {
				t.Fatalf("formal actual %d/%d aliases a previously assigned formal allocation root %s", invocationIndex, actualIndex, rootID)
			}
			byFormalRoot[rootID] = struct{}{}
			invocationRoots[invocationIndex][actualIndex] = rootID
		}
	}

	result := make(map[identity.ContentID]map[identity.ContentID]placementdomain.Placement, len(invocations))
	for invocationIndex, invocation := range invocations {
		point := formalCallEffectPoint(t, record, invocation)
		if _, duplicate := result[point]; duplicate {
			t.Fatalf("formal calls share placement-formal point %s", point)
		}
		// All six argument roots are already allocated before either call. The
		// first call selects owned's retain/store row; the second selects
		// shared's send/export/opaque row. An untouched actual must retain the
		// allocation seed's Stack placement at both cuts.
		classes := []placementdomain.Placement{
			placementdomain.OwnedHeap, placementdomain.OwnedHeap, placementdomain.Stack,
		}
		if invocationIndex == 1 {
			classes = []placementdomain.Placement{
				placementdomain.SharedHeap, placementdomain.SharedHeap, placementdomain.SharedHeap,
			}
		}
		expected := make(map[identity.ContentID]placementdomain.Placement, len(byFormalRoot))
		for rootID := range byFormalRoot {
			expected[rootID] = placementdomain.Stack
		}
		for prior := 0; prior <= invocationIndex; prior++ {
			priorClasses := classes
			if prior == 0 {
				priorClasses = []placementdomain.Placement{
					placementdomain.OwnedHeap, placementdomain.OwnedHeap, placementdomain.Stack,
				}
			}
			for actualIndex, rootID := range invocationRoots[prior] {
				expected[rootID] = priorClasses[actualIndex]
			}
		}
		result[point] = expected
	}
	return result
}

// formalInvocationOrdinals proves the ordering seam used by the authored
// fixture. Call's MountedCall rows retain the mounted Program CallAt ordinal
// through their exact occurrence identity; the artifact compiler emits those
// rows while walking Flow's authored Calls in source order. Recheck the first
// identity at this test cut so a future catalogue reorder cannot silently swap
// owned and shared expectations.
func formalInvocationOrdinals(t testing.TB, record LinkInputs, invocations []calldomain.MountedCall) []int {
	t.Helper()
	ordinals := make(map[identity.ContentID]int, len(invocations))
	for _, mount := range record.Artifacts {
		if mount.Snapshot == nil {
			continue
		}
		program := mount.Snapshot.Program()
		count, countOK := program.CallCount()
		if !countOK {
			t.Fatal("formal fixture has no Program call family")
		}
		for ordinal := 0; ordinal < count; ordinal++ {
			call, callOK := program.CallAt(ordinal)
			if !callOK || !call.ID().Available() {
				t.Fatalf("formal Program call ordinal %d is unavailable", ordinal)
			}
			if _, duplicate := ordinals[call.ID()]; duplicate {
				t.Fatalf("formal Program call %s is duplicated across mounted artifacts", call.ID())
			}
			ordinals[call.ID()] = ordinal
		}
	}
	result := make([]int, len(invocations))
	for index, mounted := range invocations {
		_, occurrence, _, _, _, identityOK := record.CallAlgebra.MountedCallIdentity(mounted)
		ordinal, ordinalOK := ordinals[occurrence]
		if !identityOK || !ordinalOK {
			t.Fatalf("formal invocation %d has no authored Program call ordinal", index)
		}
		if index > 0 && ordinal <= result[index-1] {
			t.Fatalf("formal invocation %d ordinal %d did not follow prior authored call ordinal %d", index, ordinal, result[index-1])
		}
		result[index] = ordinal
	}
	return result
}

// formalAllocationRoots reads every Program allocation root with the
// independent Value allocation receipt needed to authenticate its geometry.
// Heap's RootAllocation carrier also contains Target/fresh roots; those roots
// remain in the Placement denominator but are skipped from the authored
// six-argument prefix.
func formalAllocationRoots(t testing.TB, record LinkInputs) []formalAllocationRoot {
	t.Helper()
	if record.ValueSchema == nil || !record.ValueSchema.Valid() || !record.HeapSchema.Valid() {
		t.Fatal("formal fixture has no sealed Heap/Value authorities")
	}
	roots := make([]formalAllocationRoot, 0)
	seenIDs := make(map[identity.ContentID]struct{})
	seenValues := make(map[identity.ContentID]struct{})
	seenCoordinates := make(map[uint32]struct{})
	for index := 0; index < record.HeapSchema.KeyCount(); index++ {
		key, keyOK := record.HeapSchema.KeyAt(index)
		if !keyOK {
			t.Fatalf("Heap root %d is unavailable", index)
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		keyID, keyIDOK := key.ContentID()
		if !keyIDOK || !keyID.Available() {
			t.Fatalf("allocation root %d has no Heap identity", index)
		}
		valueID, valueIDOK := record.HeapSchema.AllocationRootValueID(key)
		allocation, allocationOK := record.ValueSchema.AllocationResultFor(key)
		// Target/fresh roots share Heap's RootAllocation carrier but are not
		// backed by a Program allocation receipt in Value. They cannot be
		// selected by the formal fixture's actuals, so leave them to the
		// denominator helper below.
		if !valueIDOK || !valueID.Available() || !allocationOK {
			continue
		}
		coordinate, coordinateOK := allocation.Coordinate()
		coordinateIndex, coordinateIndexOK := record.ValueSchema.CoordinateIndex(coordinate)
		if !coordinateOK || !coordinateIndexOK {
			t.Fatalf("allocation root %d lost its Heap/Value owner receipts", index)
		}
		if _, duplicate := seenIDs[keyID]; duplicate {
			t.Fatalf("allocation root %s is duplicated in Heap dense order", keyID)
		}
		if _, duplicate := seenValues[valueID]; duplicate {
			t.Fatalf("allocation root Value identity %s is duplicated", valueID)
		}
		if _, duplicate := seenCoordinates[coordinateIndex]; duplicate {
			t.Fatalf("allocation root Value coordinate %d is duplicated", coordinateIndex)
		}
		seenIDs[keyID] = struct{}{}
		seenValues[valueID] = struct{}{}
		seenCoordinates[coordinateIndex] = struct{}{}
		roots = append(roots, formalAllocationRoot{id: keyID, valueID: valueID, coordinate: coordinateIndex})
	}
	if len(roots) == 0 {
		t.Fatal("formal fixture sealed no allocation roots")
	}
	return roots
}

func formalHeapAllocationRootIDs(t testing.TB, record LinkInputs) map[identity.ContentID]struct{} {
	t.Helper()
	result := make(map[identity.ContentID]struct{}, record.HeapSchema.KeyCount())
	if !record.HeapSchema.Valid() {
		t.Fatal("formal fixture has no sealed Heap authority")
	}
	for index := 0; index < record.HeapSchema.KeyCount(); index++ {
		key, keyOK := record.HeapSchema.KeyAt(index)
		if !keyOK {
			t.Fatalf("Heap root %d is unavailable", index)
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		id, idOK := key.ContentID()
		if !idOK || !id.Available() {
			t.Fatalf("Heap allocation root %d has no identity", index)
		}
		if _, duplicate := result[id]; duplicate {
			t.Fatalf("Heap allocation root %s is duplicated", id)
		}
		result[id] = struct{}{}
	}
	if len(result) == 0 {
		t.Fatal("formal fixture sealed no Heap allocation roots")
	}
	return result
}

// formalCallEffectPoint resolves the exact Program placement row for one call
// occurrence. This keeps the solve assertion at the formal rule's selected
// operation cut instead of accidentally checking an earlier may-envelope.
func formalCallEffectPoint(t testing.TB, record LinkInputs, mounted calldomain.MountedCall) identity.ContentID {
	t.Helper()
	_, occurrenceID, moduleID, _, _, identityOK := record.CallAlgebra.MountedCallIdentity(mounted)
	if !identityOK {
		t.Fatal("formal invocation lost its occurrence/module identity")
	}
	for _, mount := range record.Artifacts {
		if mount.ModuleKey != moduleID || mount.Snapshot == nil {
			continue
		}
		program := mount.Snapshot.Program()
		occurrenceOrdinal, ordinalOK := program.OccurrenceOrdinalForID(programschema.OccurrenceCall, occurrenceID)
		if !ordinalOK {
			t.Fatalf("formal occurrence %s has no Program call row", occurrenceID)
		}
		count, countOK := program.RuleOccurrenceCount()
		if !countOK {
			t.Fatal("formal fixture has no Program rule-occurrence family")
		}
		var point identity.ContentID
		for index := 0; index < count; index++ {
			row, rowOK := program.RuleOccurrenceAt(index)
			rowOccurrence, rowOccurrenceOK := row.Occurrence()
			if !rowOK || !rowOccurrenceOK || int(rowOccurrence) != occurrenceOrdinal || string(row.Key()) != "placement-formal" || row.Stage() != programschema.RuleStageCallEffect {
				continue
			}
			candidate := row.PointID()
			if !candidate.Available() {
				t.Fatal("placement-formal row has no call-effect point")
			}
			if point.Available() {
				t.Fatalf("formal occurrence %s has duplicate placement-formal call-effect rows", occurrenceID)
			}
			point = candidate
		}
		if !point.Available() {
			t.Fatalf("formal occurrence %s has no placement-formal call-effect row", occurrenceID)
		}
		return point
	}
	t.Fatalf("formal occurrence %s has no mounted artifact for module %s", occurrenceID, moduleID)
	return identity.ContentID{}
}

func decodeFormalPlacementRows(t testing.TB, result placementdomain.SummaryResult) map[identity.ContentID]placementdomain.Placement {
	t.Helper()
	rows := make(map[identity.ContentID]placementdomain.Placement, result.AllocationCount())
	allocations := result.Allocations()
	for index := 0; ; index++ {
		allocation, allocationOK := allocations.Next()
		if !allocationOK {
			break
		}
		if !allocation.Available() {
			t.Fatalf("typed Placement allocation row %d is unavailable", index)
		}
		id := allocation.AllocationID()
		if !id.Available() {
			t.Fatalf("typed Placement allocation row %d has no owner identity", index)
		}
		if _, duplicate := rows[id]; duplicate {
			t.Fatalf("typed Placement allocation row %s is duplicated", id)
		}
		class := placementdomain.Bottom
		if allocation.Present() {
			decoded, classOK := allocation.Placement()
			if !classOK {
				t.Fatalf("typed Placement allocation row %d is present without a class", index)
			}
			class = decoded
		}
		rows[id] = class
	}
	return rows
}

func assertFormalPlacementRows(t testing.TB, point identity.ContentID, rows map[identity.ContentID]placementdomain.Placement, expected map[identity.ContentID]placementdomain.Placement, allocationRootIDs map[identity.ContentID]struct{}) {
	t.Helper()
	if len(rows) != len(allocationRootIDs) {
		t.Fatalf("formal Placement point %s returned %d allocation identities, want exact Heap denominator %d", point, len(rows), len(allocationRootIDs))
	}
	for id := range allocationRootIDs {
		if _, present := rows[id]; !present {
			t.Fatalf("formal Placement point %s omitted Heap allocation root %s", point, id)
		}
	}
	for id, class := range rows {
		if _, formalRoot := expected[id]; formalRoot {
			continue
		}
		if class == placementdomain.OwnedHeap || class == placementdomain.SharedHeap {
			t.Fatalf("formal Placement point %s assigned unexpected %s class %s", point, class, id)
		}
	}
	for id, want := range expected {
		got, present := rows[id]
		if !present {
			t.Fatalf("formal Placement point %s omitted formal allocation root %s", point, id)
		}
		if got != want {
			t.Fatalf("formal Placement point %s root %s = %s, want %s", point, id, got, want)
		}
	}
}

func sealCompositeFormalTarget(t testing.TB) *contract.Contract {
	t.Helper()
	provider := manifest.Provider{
		Identity: "formal-placement",
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("formal-placement")
			define := func(name string, labels []effect.Label) {
				declaration.DefineFunctionSignature(name, signature.Function{
					Type:   typ.Func().Param("first", typ.Any).Param("second", typ.Any).Param("third", typ.Any).Returns(typ.Any).Build(),
					Effect: effect.Row{Labels: labels},
				})
				declaration.DefineFunctionOperation(name, manifestwire.Operation{
					Replace: true,
					Input: manifestwire.Values{
						Fixed: []typ.Type{typ.Any, typ.Any, typ.Any},
						Tail:  manifestwire.ValuesClosed,
					},
					Outcomes: []manifestwire.Outcome{{
						Kind:          manifestwire.OutcomeNormal,
						Values:        manifestwire.Values{Fixed: []typ.Type{typ.Any}, Tail: manifestwire.ValuesClosed},
						ResultAliases: []manifestwire.ResultAlias{{Result: 0, Source: manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0}}},
					}},
					Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
				})
			}
			define("owned", []effect.Label{
				ownership.Retain{Param: effect.ParamRef{Index: 0}},
				ownership.Store{Param: effect.ParamRef{Index: 1}, Into: effect.ParamRef{Index: 2}},
			})
			define("shared", []effect.Label{
				ownership.SendParam{Param: effect.ParamRef{Index: 0}},
				ownership.Export{Param: effect.ParamRef{Index: 1}},
				ownership.Opaque{Param: effect.ParamRef{Index: 2}},
			})
			return declaration
		},
	}
	requireProvider := manifest.Provider{
		Identity: "formal-placement-host",
		Mount:    manifest.MountGlobals,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("formal-placement-host")
			requireType := typ.Func().Param("module", typ.String).Returns(typ.Any).Build()
			declaration.DefineFunctionSignature("require", signature.Function{
				Type:   requireType,
				Effect: effect.Empty.With(dispatch.ModuleLoad{}),
			})
			declaration.DefineGlobalType("require", requireType)
			return declaration
		},
	}
	catalogue, err := manifest.Seal(append(stdlib.Providers(), provider, requireProvider)...)
	if err != nil {
		t.Fatal(err)
	}
	target, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

// mountFormalTargetRecord is the same cold mount transaction used by the
// materializer fixture, parameterized only by the manifest Target contract.
// Keeping this helper local makes the law's manifest provenance visible and
// avoids adding a second production mounting API for a test-only target.
func mountFormalTargetRecord(t testing.TB, target *contract.Contract, name, source string) (LinkInputs, artifactcompiler.CompileFailure, bool) {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: name + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	actor, instance, root := name+"-actor", name+"-cache", name+"-root"
	linked, err := proglink.Seal(&proglink.Spec{
		Target:  target,
		Modules: []linkproject.Module{{Name: name, Program: program}},
		Module: linkmodule.Spec{
			Actors:             []linkmodule.ActorSpec{{Name: actor}},
			ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{{Actor: actor, Instances: []string{instance}, Representative: instance}},
			AnalysisRoots:      []linkmodule.AnalysisRootSpec{{Name: root, Module: name, Actor: actor, Instance: instance}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := Build()
	grammar := receipt.ExecutionSchemaID()
	grammarOK := grammar.Available()
	issuance, issuanceOK := ArtifactIssuanceDirectory(receipt)
	if !receiptOK || !grammarOK || !issuanceOK {
		t.Fatal("program schema receipt is unavailable")
	}
	mounts := linked.Project().Mounts()
	artifacts := make([]programschema.Program, mounts.Count())
	rows := make([]programmount.MountedArtifact, mounts.Count())
	statics := make([]staticdomain.MountedProgram, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := mounts.ProgramID(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK || !programIDOK {
			t.Fatalf("mount %d has no artifact source", index)
		}
		artifact, failure := artifactcompiler.CompileDetailed(mounted, grammar, issuance)
		if failure.Available() || artifact == nil || !artifact.Available() {
			return LinkInputs{}, failure, false
		}
		artifacts[index] = artifact.Program()
		vocabularyTable, vocabularyOK := StructureVocabulary(receipt)
		snapshotValue, lowered := ingress.Lower(artifact, vocabularyTable)
		if !vocabularyOK || !lowered {
			t.Fatalf("lower artifact %d", index)
		}
		compiledProgram := artifact.Program()
		catalog, catalogOK := programcatalog.CatalogID(compiledProgram.SchemaID)
		if !compiledProgram.Available() || compiledProgram.ProgramID != programID || !catalogOK || !catalog.Available() {
			t.Fatalf("artifact %d publishes no cold program", index)
		}
		mountedProgram := programmount.Program{ModuleKey: module, Program: compiledProgram}
		if !mountedProgram.Available() {
			t.Fatalf("mount row %d unavailable", index)
		}
		rows[index] = programmount.MountedArtifact{Program: mountedProgram, Snapshot: snapshotValue}
		statics[index] = staticdomain.MountedProgram{Program: mountedProgram.Program, ModuleID: module, NamespaceID: module}
	}
	types, err := typeauthority.SealProgramRows(linked.ContentID(), artifacts)
	if err != nil || types == nil {
		t.Fatalf("seal type authority: %v", err)
	}
	inventory, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: target}, types, statics)
	if err != nil || inventory == nil {
		t.Fatalf("seal static authority: %v", err)
	}
	record, mountFailure := MountLink(receipt, LinkInputs{Source: linked, Artifacts: rows, StaticAuthority: inventory})
	if mountFailure.Available() {
		t.Fatalf("mount Link: %v", mountFailure)
	}
	return record, artifactcompiler.CompileFailure{}, true
}

package artifact

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func (compiler *compiler) finalizeFailure() CompileFailure {
	// Least-significant key first preserves canonical (From, To, ID) order.
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.environment); index++ {
		if compiler.environment[index-1].id == compiler.environment[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.localTransfers); index++ {
		if compiler.localTransfers[index-1].id == compiler.localTransfers[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.diagnosticObservations, func(row DiagnosticObservationRow) identity.ContentID { return row.id })
	for index := 1; index < len(compiler.diagnosticObservations); index++ {
		if compiler.diagnosticObservations[index-1].id == compiler.diagnosticObservations[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) sealArtifact() (*Artifact, CompileFailure) {
	pointIDs := make([]identity.ContentID, 0, len(compiler.points))
	for id := range compiler.points {
		pointIDs = append(pointIDs, id)
	}
	identity.SortContentIDs(pointIDs)
	points := make([]Point, len(pointIDs))
	for index, id := range pointIDs {
		point, ok := compiler.pointGeometry[id]
		if !ok || !point.Available() {
			return nil, compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		points[index] = point
	}
	occurrenceByID := make(map[occurrenceLookup]uint32, len(compiler.occurrences))
	occurrenceByKind := make(map[OccurrenceKind][]uint32)
	for index, row := range compiler.occurrences {
		if uint64(index) > uint64(^uint32(0)) {
			return nil, compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		occurrenceByID[occurrenceLookup{kind: row.kind, id: row.id}] = uint32(index)
		occurrenceByKind[row.kind] = append(occurrenceByKind[row.kind], uint32(index))
	}
	functionBoundaryByBody := make(map[identity.ContentID]uint32, len(compiler.functionBoundaries))
	for index, row := range compiler.functionBoundaries {
		functionBoundaryByBody[row.BodyID()] = uint32(index)
	}
	frozen, catalog, frozenOK := freezeColdPublication(compiler)
	if !frozenOK {
		return nil, compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	artifact := &Artifact{
		frozen: frozen, coldCatalog: catalog,
		key: compiler.key, counts: compiler.counts, pointAttachments: compiler.pointAttachments, points: points, environment: compiler.environment, localTransfers: compiler.localTransfers,
		regions: compiler.regions, events: compiler.events, values: compiler.values, calls: compiler.calls, callOperands: compiler.callOperands, callArguments: compiler.callArguments, callTypeArguments: compiler.callTypeArguments,
		bodies: compiler.bodies, functionBoundaries: compiler.functionBoundaries, outcomes: compiler.outcomes, returnValues: compiler.returnValues,
		boundaries:      compiler.boundaries,
		heapAllocations: compiler.heapAllocations, heapIndexes: compiler.heapIndexes,
		occurrences: compiler.occurrences, occurrenceByID: occurrenceByID, occurrenceByKind: occurrenceByKind, functionBoundaryByBody: functionBoundaryByBody, ruleOccurrences: compiler.ruleOccurrences,
		diagnosticObservations: compiler.diagnosticObservations, staticTypeArguments: compiler.staticTypeArguments, staticTypeValues: compiler.staticTypeValues, staticTypeNodes: compiler.staticTypeNodes, staticExpressions: compiler.staticExpressions, staticInputs: compiler.staticInputs,
	}
	artifact.id = artifactID(artifact)
	if failure := artifact.validUnsealedFailure(); failure.Available() {
		return nil, failure
	}
	artifact.sealed = artifact.id
	return artifact, CompileFailure{}
}

// coldStores issues the store identity of each compiled program's cold
// publication. A store identity is runtime-issued and never derived from
// content: two compilations of identical content are still two stores, and an
// address issued by one is not addressable in the other.
var coldStores atomic.Uint64

// freezeColdPublication seals the families that have moved onto the shared
// publication substrate. The compiler's own slices are transient build state
// and are not carried into the artifact beside it: the sealed publication is
// the single authority for every family it holds.
func freezeColdPublication(compiler *compiler) (snapshot.Frozen, identity.ContentID, bool) {
	if compiler == nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	catalog, derived := cold.CatalogID(compiler.key.SchemaDigest())
	if !derived {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	store := identity.StoreID(coldStores.Add(1))
	if !store.Available() {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	content, sealed := cold.CallTargetFamily().Content(compiler.callTargets, catalog)
	if !sealed {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	exactScalarContent, exactScalarSealed := cold.ExactScalarSummaryFamily().Content(compiler.exactScalarSummaries, catalog)
	if !exactScalarSealed {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	arithmeticContent, arithmeticSealed := cold.ArithmeticSummaryFamily().Content(compiler.arithmeticSummaries, catalog)
	if !arithmeticSealed {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	unaryContent, unarySealed := cold.UnarySummaryFamily().Content(compiler.unarySummaries, catalog)
	if !unarySealed {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	builder := snapshot.NewFrozen(catalog, store)
	if err := snapshot.PutFrozenColumn(&builder, cold.CallTargetFamily().Axis(catalog), content); err != nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	if err := snapshot.PutFrozenColumn(&builder, cold.ExactScalarSummaryFamily().Axis(catalog), exactScalarContent); err != nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	if err := snapshot.PutFrozenColumn(&builder, cold.ArithmeticSummaryFamily().Axis(catalog), arithmeticContent); err != nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	if err := snapshot.PutFrozenColumn(&builder, cold.UnarySummaryFamily().Axis(catalog), unaryContent); err != nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	frozen, err := builder.Seal()
	if err != nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	return frozen, catalog, true
}

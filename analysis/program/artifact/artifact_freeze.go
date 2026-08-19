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
	pointIDs := make([]identity.ContentID, 0, len(compiler.pointGeometry))
	for id := range compiler.pointGeometry {
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
		key: compiler.key, counts: compiler.counts, points: points, environment: compiler.environment, localTransfers: compiler.localTransfers,
		regions: compiler.regions, events: compiler.events, calls: compiler.calls, callOperands: compiler.callOperands, callArguments: compiler.callArguments, callTypeArguments: compiler.callTypeArguments,
		bodies: compiler.bodies, functionBoundaries: compiler.functionBoundaries, outcomes: compiler.outcomes, returnValues: compiler.returnValues,
		occurrences: compiler.occurrences, occurrenceByID: occurrenceByID, occurrenceByKind: occurrenceByKind, functionBoundaryByBody: functionBoundaryByBody, ruleOccurrences: compiler.ruleOccurrences,
		diagnosticObservations: compiler.diagnosticObservations, staticTypeValues: compiler.staticTypeValues, staticTypeNodes: compiler.staticTypeNodes, staticExpressions: compiler.staticExpressions, staticInputs: compiler.staticInputs,
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

// coldHeapPlanes flattens the compiler's nested allocation rows into the two
// dense planes the publication holds. Each allocation names its fields by the
// half-open span it occupies in the field plane, so the canonical field order
// the compiler emitted is the ordinal order of the plane and no allocation
// retains a slice header.
func coldHeapPlanes(rows []HeapAllocationRow) ([]cold.HeapAllocation, []cold.HeapField, bool) {
	allocations := make([]cold.HeapAllocation, 0, len(rows))
	fields := make([]cold.HeapField, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(fields)) || !fitsUint32(len(row.fields)) {
			return nil, nil, false
		}
		offset := uint32(len(fields))
		for _, field := range row.fields {
			converted, ok := cold.NewHeapField(
				field.id, uint8(field.kind), field.fieldSpan, field.selectorSpan, field.valuesSpan, field.valuesID,
				field.width, field.finalOpen, field.sharesFirstValueCell, uint64(field.normalized), field.normalizedOK,
			)
			if !ok {
				return nil, nil, false
			}
			fields = append(fields, converted)
		}
		allocation, ok := cold.NewHeapAllocation(row.id, uint8(row.role), uint8(row.form), row.rootSpan, offset, uint32(len(row.fields)))
		if !ok {
			return nil, nil, false
		}
		allocations = append(allocations, allocation)
	}
	return allocations, fields, true
}

// coldValuesPlanes flattens the compiler's nested Values rows the same way:
// members become one dense plane and each row names the span it owns.
func coldValuesPlanes(rows []ValuesRow) ([]cold.Values, []cold.ValuesMember, bool) {
	values := make([]cold.Values, 0, len(rows))
	members := make([]cold.ValuesMember, 0, len(rows))
	for _, row := range rows {
		if !row.Available() || !fitsUint32(len(members)) || !fitsUint32(len(row.members)) {
			return nil, nil, false
		}
		offset := uint32(len(members))
		for _, member := range row.members {
			converted, ok := cold.NewValuesMember(member.id)
			if !ok {
				return nil, nil, false
			}
			members = append(members, converted)
		}
		tail, tailOK := cold.NewValuesTail(row.tail.id, row.tail.span, cold.ValuesTailKind(row.tail.kind), row.tail.present)
		if !tailOK {
			return nil, nil, false
		}
		converted, ok := cold.NewValues(row.id, row.body, row.span, offset, uint32(len(row.members)), tail)
		if !ok {
			return nil, nil, false
		}
		values = append(values, converted)
	}
	return values, members, true
}

// coldHeapIndexes copies the compiler's index-access rows one for one; the
// row is already flat, so the conversion is a change of vocabulary only.
func coldHeapIndexes(rows []HeapIndexRow) ([]cold.HeapIndex, bool) {
	indexes := make([]cold.HeapIndex, 0, len(rows))
	for _, row := range rows {
		converted, ok := cold.NewHeapIndex(
			row.id, row.read, row.baseSpan, row.resultSpan, row.keySpan,
			row.lensKind, uint64(row.exactKey), row.valuesSpan, row.valuesID, row.position,
		)
		if !ok {
			return nil, false
		}
		indexes = append(indexes, converted)
	}
	return indexes, true
}

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
	allocations, allocationFields, heapOK := coldHeapPlanes(compiler.heapAllocations)
	values, valuesMembers, valuesOK := coldValuesPlanes(compiler.values)
	indexes, indexesOK := coldHeapIndexes(compiler.heapIndexes)
	if !heapOK || !valuesOK || !indexesOK {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	publication := cold.Publication{
		CallTargets:     compiler.callTargets,
		HeapAllocations: allocations, HeapFields: allocationFields,
		Values: values, ValuesMembers: valuesMembers,
		HeapIndexes:          indexes,
		ExactScalarSummaries: compiler.exactScalarSummaries,
		ArithmeticSummaries:  compiler.arithmeticSummaries,
		UnarySummaries:       compiler.unarySummaries,
	}
	frozen, sealed := publication.Seal(catalog, identity.StoreID(coldStores.Add(1)))
	if !sealed {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	return frozen, catalog, true
}

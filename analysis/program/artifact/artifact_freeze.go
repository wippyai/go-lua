package artifact

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
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
	identity.SortByContentID(compiler.localTransfers, func(row localTransferDraft) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.localTransfers, func(row localTransferDraft) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.localTransfers, func(row localTransferDraft) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.localTransfers); index++ {
		if compiler.localTransfers[index-1].id == compiler.localTransfers[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.diagnosticObservations, func(row programschema.DiagnosticObservation) identity.ContentID { return row.ID() })
	for index := 1; index < len(compiler.diagnosticObservations); index++ {
		if compiler.diagnosticObservations[index-1].ID() == compiler.diagnosticObservations[index].ID() {
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
	frozen, catalog, frozenOK := freezeColdPublication(compiler, points)
	if !frozenOK {
		return nil, compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	artifact := &Artifact{
		frozen: frozen, coldCatalog: catalog,
		key: compiler.key, counts: compiler.counts,
		staticTypeNodes: compiler.staticTypeNodes,
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
func coldHeapPlanes(rows []HeapAllocationRow) ([]programschema.HeapAllocation, []programschema.HeapField, bool) {
	allocations := make([]programschema.HeapAllocation, 0, len(rows))
	fields := make([]programschema.HeapField, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(fields)) || !fitsUint32(len(row.fields)) {
			return nil, nil, false
		}
		offset := uint32(len(fields))
		for _, field := range row.fields {
			converted, ok := programschema.NewHeapField(
				field.id, uint8(field.kind), field.fieldSpan, field.selectorSpan, field.valuesSpan, field.valuesID,
				field.width, field.finalOpen, field.sharesFirstValueCell, uint64(field.normalized), field.normalizedOK,
			)
			if !ok {
				return nil, nil, false
			}
			fields = append(fields, converted)
		}
		allocation, ok := programschema.NewHeapAllocation(row.id, uint8(row.role), uint8(row.form), row.rootSpan, offset, uint32(len(row.fields)))
		if !ok {
			return nil, nil, false
		}
		allocations = append(allocations, allocation)
	}
	return allocations, fields, true
}

// coldValuesPlanes flattens the compiler's nested Values rows the same way:
// members become one dense plane and each row names the span it owns.
func coldValuesPlanes(rows []ValuesRow) ([]programschema.Values, []programschema.ValuesMember, bool) {
	values := make([]programschema.Values, 0, len(rows))
	members := make([]programschema.ValuesMember, 0, len(rows))
	for _, row := range rows {
		if !row.Available() || !fitsUint32(len(members)) || !fitsUint32(len(row.members)) {
			return nil, nil, false
		}
		offset := uint32(len(members))
		for _, member := range row.members {
			converted, ok := programschema.NewValuesMember(member.id)
			if !ok {
				return nil, nil, false
			}
			members = append(members, converted)
		}
		tail, tailOK := programschema.NewValuesTail(row.tail.id, row.tail.span, programschema.ValuesTailKind(row.tail.kind), row.tail.present)
		if !tailOK {
			return nil, nil, false
		}
		converted, ok := programschema.NewValues(row.id, row.body, row.span, offset, uint32(len(row.members)), tail)
		if !ok {
			return nil, nil, false
		}
		values = append(values, converted)
	}
	return values, members, true
}

// coldPointPlanes flattens the sealed point sequence into the two dense
// planes the publication holds. Each point names its decisions by the
// half-open span it occupies in the decision plane, so the canonical decision
// order is the ordinal order of the plane and no point retains a slice header.
func coldPointPlanes(rows []Point) ([]programschema.Point, []programschema.PointDecision, bool) {
	points := make([]programschema.Point, 0, len(rows))
	decisions := make([]programschema.PointDecision, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(decisions)) || !fitsUint32(len(row.decisions)) {
			return nil, nil, false
		}
		offset := uint32(len(decisions))
		for _, decision := range row.decisions {
			converted, ok := programschema.NewPointDecision(decision)
			if !ok {
				return nil, nil, false
			}
			decisions = append(decisions, converted)
		}
		point, ok := programschema.NewPoint(row.id, row.initial, offset, uint32(len(row.decisions)))
		if !ok {
			return nil, nil, false
		}
		points = append(points, point)
	}
	return points, decisions, true
}

// coldEnvironmentPlanes flattens the compiler's nested environment rows into
// the two dense planes the publication holds. Each edge names its reset
// witnesses by the half-open span it occupies in the reset plane, so the
// canonical witness order is the ordinal order of the plane and no edge
// retains a slice header.
func coldEnvironmentPlanes(rows []EnvironmentEdge) ([]programschema.EnvironmentEdge, []programschema.EnvironmentReset, bool) {
	edges := make([]programschema.EnvironmentEdge, 0, len(rows))
	resets := make([]programschema.EnvironmentReset, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(resets)) || !fitsUint32(len(row.resets)) {
			return nil, nil, false
		}
		offset := uint32(len(resets))
		for _, reset := range row.resets {
			converted, ok := programschema.NewEnvironmentReset(reset)
			if !ok {
				return nil, nil, false
			}
			resets = append(resets, converted)
		}
		edge, ok := programschema.NewEnvironmentEdge(
			row.id, row.from, row.to, row.route, row.guard, row.decision, row.condition,
			row.reset, row.component, row.mu, offset, uint32(len(row.resets)), uint8(row.arm),
			row.guarded, row.truth, row.hasReset, row.hasMu,
		)
		if !ok {
			return nil, nil, false
		}
		edges = append(edges, edge)
	}
	return edges, resets, true
}

// coldLocalTransfers copies the compiler's transient transport drafts into
// the canonical Program family. The draft order is already the canonical
// (From, To, ID) order established by finalizeFailure; no resort or identity
// remint occurs at this boundary.
func coldLocalTransfers(rows []localTransferDraft) ([]programschema.LocalTransfer, []programschema.LocalTransferWrite, bool) {
	transfers := make([]programschema.LocalTransfer, 0, len(rows))
	writes := make([]programschema.LocalTransferWrite, 0)
	for _, row := range rows {
		if !fitsUint32(len(writes)) || !fitsUint32(len(row.writes)) {
			return nil, nil, false
		}
		offset := uint32(len(writes))
		for _, key := range row.writes {
			write, ok := programschema.NewLocalTransferWrite(key)
			if !ok {
				return nil, nil, false
			}
			writes = append(writes, write)
		}
		converted, ok := programschema.NewLocalTransfer(row.id, row.from, row.to, row.full, offset, uint32(len(row.writes)))
		if !ok {
			return nil, nil, false
		}
		transfers = append(transfers, converted)
	}
	return transfers, writes, true
}

// coldStaticPlanes copies the compiler's remaining local authored-static rows
// into their canonical families. StaticInput rows are already canonical and
// enter Publication directly, so they are deliberately absent here.
func coldStaticPlanes(values []StaticTypeValueRow, expressions []StaticExpressionRow) ([]programschema.StaticTypeValue, []programschema.StaticExpression, bool) {
	typeValues := make([]programschema.StaticTypeValue, 0, len(values))
	for _, row := range values {
		converted, ok := programschema.NewStaticTypeValue(row.id, row.body, row.reference, row.root, row.name)
		if !ok {
			return nil, nil, false
		}
		typeValues = append(typeValues, converted)
	}
	staticExpressions := make([]programschema.StaticExpression, 0, len(expressions))
	for _, row := range expressions {
		converted, ok := programschema.NewStaticExpression(row.id, row.reference, row.owner)
		if !ok {
			return nil, nil, false
		}
		staticExpressions = append(staticExpressions, converted)
	}
	return typeValues, staticExpressions, true
}

// coldRegionPlanes flattens the compiler's nested region rows into the two
// dense planes the publication holds, and copies the event bracket sequence
// one for one. Each region names its members by the half-open span it
// occupies in the member plane, so the canonical member order is the ordinal
// order of the plane and no region retains a slice header.
func coldRegionPlanes(rows []Region, events []WTOEvent) ([]programschema.Region, []programschema.RegionMember, []programschema.WTOEvent, bool) {
	regions := make([]programschema.Region, 0, len(rows))
	members := make([]programschema.RegionMember, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(members)) || !fitsUint32(len(row.members)) {
			return nil, nil, nil, false
		}
		offset := uint32(len(members))
		for _, member := range row.members {
			converted, ok := programschema.NewRegionMember(member)
			if !ok {
				return nil, nil, nil, false
			}
			members = append(members, converted)
		}
		region, ok := programschema.NewRegion(row.id, row.parent, offset, uint32(len(row.members)), row.cyclic)
		if !ok {
			return nil, nil, nil, false
		}
		regions = append(regions, region)
	}
	brackets := make([]programschema.WTOEvent, 0, len(events))
	for _, event := range events {
		converted, ok := programschema.NewWTOEvent(uint8(event.kind), event.region, event.point)
		if !ok {
			return nil, nil, nil, false
		}
		brackets = append(brackets, converted)
	}
	return regions, members, brackets, true
}

// coldHeapIndexes copies the compiler's index-access rows one for one; the
// row is already flat, so the conversion is a change of vocabulary only.
func coldHeapIndexes(rows []HeapIndexRow) ([]programschema.HeapIndex, bool) {
	indexes := make([]programschema.HeapIndex, 0, len(rows))
	for _, row := range rows {
		converted, ok := programschema.NewHeapIndex(
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
func freezeColdPublication(compiler *compiler, pointRows []Point) (snapshot.Frozen, identity.ContentID, bool) {
	if compiler == nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	catalog, derived := programschema.CatalogID(compiler.key.SchemaDigest())
	if !derived {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	allocations, allocationFields, heapOK := coldHeapPlanes(compiler.heapAllocations)
	values, valuesMembers, valuesOK := coldValuesPlanes(compiler.values)
	indexes, indexesOK := coldHeapIndexes(compiler.heapIndexes)
	points, pointDecisions, pointsOK := coldPointPlanes(pointRows)
	edges, resets, edgesOK := coldEnvironmentPlanes(compiler.environment)
	transfers, transferWrites, transfersOK := coldLocalTransfers(compiler.localTransfers)
	typeValues, staticExpressions, staticOK := coldStaticPlanes(compiler.staticTypeValues, compiler.staticExpressions)
	regions, regionMembers, events, regionsOK := coldRegionPlanes(compiler.regions, compiler.events)
	if !heapOK || !valuesOK || !indexesOK || !pointsOK || !edgesOK || !transfersOK || !staticOK || !regionsOK {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	for index, row := range compiler.occurrences {
		if !occurrenceDenseAvailable(row, compiler.occurrencePoints, compiler.occurrenceInputs) {
			return snapshot.Frozen{}, identity.ContentID{}, false
		}
		if uint64(index) > uint64(^uint32(0)) {
			return snapshot.Frozen{}, identity.ContentID{}, false
		}
	}
	for _, row := range compiler.ruleOccurrences {
		if !row.Available() {
			return snapshot.Frozen{}, identity.ContentID{}, false
		}
	}
	publication := programschema.Publication{
		CallTargets:     compiler.callTargets,
		HeapAllocations: allocations, HeapFields: allocationFields,
		Values: values, ValuesMembers: valuesMembers,
		HeapIndexes:            indexes,
		ExactScalarSummaries:   compiler.exactScalarSummaries,
		ArithmeticSummaries:    compiler.arithmeticSummaries,
		UnarySummaries:         compiler.unarySummaries,
		Points:                 points,
		PointDecisions:         pointDecisions,
		EnvironmentEdges:       edges,
		EnvironmentResets:      resets,
		LocalTransfers:         transfers,
		LocalTransferWrites:    transferWrites,
		DiagnosticObservations: compiler.diagnosticObservations,
		DiagnosticEvidence:     compiler.diagnosticEvidence,
		DiagnosticPaths:        compiler.diagnosticPaths,
		Occurrences:            compiler.occurrences,
		OccurrencePoints:       compiler.occurrencePoints,
		OccurrenceInputs:       compiler.occurrenceInputs,
		RuleOccurrences:        compiler.ruleOccurrences,
		StaticTypeValues:       typeValues,
		StaticExpressions:      staticExpressions,
		StaticInputs:           compiler.staticInputs,
		Regions:                regions,
		RegionMembers:          regionMembers,
		WTOEvents:              events,
		Calls:                  compiler.calls,
		CallOperands:           compiler.callOperands,
		CallArguments:          compiler.callArguments,
		CallTypeArguments:      compiler.callTypeArguments,
		Bodies:                 compiler.bodies,
		BodyEntries:            compiler.bodyEntries,
		BodyRoots:              compiler.bodyRoots,
		Outcomes:               compiler.outcomes,
		OutcomeReturnValues:    compiler.outcomeReturnValues,
		OutcomePoints:          compiler.outcomePoints,
		FunctionBoundaries:     compiler.functionBoundaries,
		FunctionFormals:        compiler.functionFormals,
		FunctionVarargs:        compiler.functionVarargs,
		FunctionCaptures:       compiler.functionCaptures,
	}
	frozen, sealed := publication.Seal(catalog, identity.StoreID(coldStores.Add(1)))
	if !sealed {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	return frozen, catalog, true
}

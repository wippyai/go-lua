// runtime_program.go declares the sealed row-model program: the dense member
// and factor tables the solve loop reads, and the one Seal that decides a
// program's validity. The value holds role-specific concrete tables and no
// pointer into a resizable slice: every cross-table reference is a dense index
// or a canonical key.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/population"
)

// memberRow is the sealed execution union. Exactly one arm is present: the
// established legacy typed member or the engine-private generated member.
// Geometry is projected through one shared accessor so fold/seal consumers do
// not duplicate metadata handling for either execution representation.
type memberRow struct {
	legacy    runtimeMember
	generated *generatedMember
}

func (row memberRow) geometry() (runtimeMemberGeometry, bool) {
	legacyPresent := row.legacy != nil
	generatedPresent := row.generated != nil
	if legacyPresent == generatedPresent {
		return nil, false
	}
	if legacyPresent {
		return row.legacy, true
	}
	return row.generated, true
}

func (row memberRow) valid() bool {
	geometry, ok := row.geometry()
	if !ok || geometry == nil || !geometry.member().Key().Available() {
		return false
	}
	slot, hasSlot := geometry.outputSlot()
	return !hasSlot || slot >= 0
}

// factorRecord is one sealed Factor coordinate: its canonical key, the carrier
// slot it owns, and the dense index of the retained typed owner. The owner
// keeps the route universe by reference; the record never copies it.
type factorRecord struct {
	key   composition.Key
	slot  shape.Slot
	owner int32
}

// queryExec is the typed projection compiled from one sealed Schema query
// row. runtimeProgram supplies the direct Factor handle and Unit; no graph or
// binding owner is captured by the closure.
type queryExec func(*carrier.Work, carrier.State, runtimeFactor, carrier.Unit) (frozenValue, solveBoundary, bool)

// heterogeneousQueryExec is the one combined execution seam for an ordered
// heterogeneous query projection vector. The runtime program supplies the
// sealed Factor owners; the graph still contributes exactly one query row and
// one point regardless of vector width.
type heterogeneousQueryExec func(*carrier.Work, carrier.State, *runtimeProgram) (frozenValue, solveBoundary, bool)

type queryProjectionRow struct {
	factorOrdinal uint64
	unit          carrier.Unit
}

// heterogeneousQueryRow retains the ordered Factor/Unit pairs of one query.
// It deliberately has no graph or topology multiplication: point and query
// identity remain on queryRow, while these pairs are the sole additional
// runtime coordinates needed by the typed fold.
type heterogeneousQueryRow struct {
	projections []queryProjectionRow
	exec        heterogeneousQueryExec
}

// queryRow is the whole live query plan. Schema ordinals select its declared
// family and factor, point is a store-local dense graph handle, and Unit is the
// Factor-issued read contract for the graph surface.
type queryRow struct {
	queryOrdinal  uint64
	factorOrdinal uint64
	point         int32
	// state is the compact executable-state address for this query's exact
	// (ContextID, Point) pair. It is authenticated at bind and checked again at
	// the program seal; point remains alongside it for the current materializer
	// until the executor's epoch storage is context-addressed.
	state         contextfiber.StateOrdinal
	unit          carrier.Unit
	exec          queryExec
	heterogeneous *heterogeneousQueryRow
}

func (row queryRow) valid() bool {
	_, unitOK := row.unit.Slot()
	if row.point < 0 || row.heterogeneous != nil && row.exec != nil {
		return false
	}
	if row.heterogeneous != nil {
		if row.heterogeneous.exec == nil || len(row.heterogeneous.projections) == 0 {
			return false
		}
		for _, projection := range row.heterogeneous.projections {
			if _, unitOK := projection.unit.Slot(); !unitOK {
				return false
			}
		}
		return true
	}
	return unitOK && row.exec != nil
}

// observationRow is a solve-local projection request over the same sealed
// query family. Its publication identity is explicit because observations do
// not occupy the graph's query table.
type observationRow struct {
	id            identity.ContentID
	queryOrdinal  uint64
	factorOrdinal uint64
	point         int32
	// state is the compact executable-state address for this observation's
	// exact (ContextID, member-output Point) pair. It is resolved while the
	// committed directory/index/layout are attached and is retained as the
	// only context coordinate needed by the later materialization seam.
	state contextfiber.StateOrdinal
	// contextID is the admission context that authenticated the observation.
	// StateOrdinal alone is insufficient at Seal: a same-point row from another
	// mounted context must not be interchangeable with this row. Link-global
	// state remains singular, but its observation admission is still checked
	// against this exact directory context.
	contextID     identity.ContentID
	unit          carrier.Unit
	exec          queryExec
	heterogeneous *heterogeneousQueryRow
}

func (row observationRow) valid() bool {
	_, unitOK := row.unit.Slot()
	if !row.id.Available() || row.point < 0 || row.heterogeneous != nil && row.exec != nil {
		return false
	}
	if row.heterogeneous != nil {
		if row.heterogeneous.exec == nil || len(row.heterogeneous.projections) == 0 {
			return false
		}
		for _, projection := range row.heterogeneous.projections {
			if _, unitOK := projection.unit.Slot(); !unitOK {
				return false
			}
		}
		return true
	}
	return unitOK && row.exec != nil
}

func (record factorRecord) valid() bool {
	return record.key.Available() && record.slot >= 0 && record.owner >= 0
}

// memberSpan is the half-open member range one graph Group owns in the dense
// member table. Groups address their members by span, never by a retained
// slice header into the table.
type memberSpan struct {
	start int32
	end   int32
}

func (span memberSpan) count() int { return int(span.end - span.start) }

// runtimeProgram is the sealed row-model program. It is produced by exactly one
// constructor, is never written after that constructor returns, and exposes its
// tables only through copying accessors.
type runtimeProgram struct {
	memberTable []memberRow
	groupSpans  []memberSpan
	// generatedPrograms borrows Schema's immutable Rule-ordinal descriptor
	// table. Every generated occurrence row refers to one descriptor here; no
	// occurrence owns an executable object or invocation storage.
	generatedPrograms  []generated.CompiledRule
	generatedPresent   bool
	generatedExecution *generatedExecutionProgram
	factorTable        []factorRecord
	factorOwners       []runtimeFactor
	queryTable         []queryRow
	observationTable   []observationRow
	programSealed      bool
}

// sealRuntimeProgram is the sole Seal and the sole writer of a runtimeProgram.
// It takes the one program-level validity decision: either every table is
// mutually consistent and the program is sealed, or no program exists. An
// artifact-backed row must carry the StateOrdinal resolved from its exact
// context plane; the graph-point StateOrdinal rule is reserved for the
// explicitly non-artifact construction.
func sealRuntimeProgram(schema *Schema, graph *equation.Graph, runtime *carrier.Composition, rows []memberRow, spans []memberSpan, factors []factorRecord, owners []runtimeFactor, queries []queryRow, observations []observationRow, contexts executioncontext.Directory, contextIndex contextfiber.Index, contextLayout contextfiber.Layout, pointOwners []contextfiber.PointOwner, artifactBacked bool) (*runtimeProgram, topologyConstructionRefusal, bool) {
	if schema == nil || !schema.Available() || graph == nil || runtime == nil || graph.CompositionID() != schema.coldID() || len(factors) != len(owners) || len(factors) != schemaFactorCount(schema) || len(queries) != graph.QueryCount() {
		return nil, refuseProgramSeal(topologyConstructionStepDeclarationShape), false
	}
	contextPlanePresent := contexts.Available() || contextIndex.Available() || contextLayout.Available() || len(pointOwners) != 0
	if artifactBacked {
		if !validQueryContextPlane(graph, contexts, contextIndex, contextLayout, pointOwners) {
			return nil, refuseProgramSeal(topologyConstructionStepDeclarationShape), false
		}
	} else if contextPlanePresent {
		return nil, refuseProgramSeal(topologyConstructionStepDeclarationShape), false
	}
	next := int32(0)
	for groupIndex, span := range spans {
		if span.start != next || span.end < span.start || int(span.end) > len(rows) {
			return nil, refuseProgramSeal(topologyConstructionStepMemberGroup), false
		}
		group, groupOK := graph.HyperedgeAt(groupIndex)
		if !groupOK || span.count() != group.MemberCount() {
			return nil, refuseProgramSeal(topologyConstructionStepMemberGroup), false
		}
		// A Group's rows are exactly its graph members. The transient expected set
		// proves that permutation once; no identity/index mirror survives Seal.
		expected := make(map[composition.Key]struct{}, group.MemberCount())
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !member.Key().Available() {
				return nil, refuseProgramSeal(topologyConstructionStepMemberGroup), false
			}
			expected[member.Key()] = struct{}{}
		}
		if len(expected) != group.MemberCount() {
			return nil, refuseProgramSeal(topologyConstructionStepMemberGroup), false
		}
		for position := span.start; position < span.end; position++ {
			row := rows[position]
			if !row.valid() {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
			geometry, geometryOK := row.geometry()
			if !geometryOK {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
			key := geometry.member().Key()
			if _, present := expected[key]; !present {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
			delete(expected, key)
		}
		if len(expected) != 0 {
			return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
		}
		next = span.end
	}
	if int(next) != len(rows) {
		return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
	}
	for index, record := range factors {
		owner := owners[index]
		slot, slotOK := shape.Slot(0), false
		if owner != nil {
			slot, slotOK = owner.runtimeSlot()
		}
		if !record.valid() || int(record.owner) != index || owner == nil || !slotOK || slot != record.slot || compositionKeyOf(owner.semantic()) != record.key || schema.factorSemanticAt(uint64(index)) != record.key || index > 0 && !lessRuntimeKey(factors[index-1].key, record.key) {
			return nil, refuseProgramSeal(topologyConstructionStepBinding), false
		}
	}
	for index, row := range queries {
		query, queryOK := graph.QueryAt(index)
		point, pointOK := graph.PointIndex(query.Point())
		state, stateOK := contextfiber.StateOrdinal(point), pointOK
		if artifactBacked {
			state, stateOK = queryStateOrdinalOwned(graph, query, contextIndex, contextLayout)
		}
		if !queryOK || !row.valid() || !pointOK || int(row.point) != point || !stateOK || row.state != state || row.queryOrdinal >= schema.queryCount() || schema.querySemanticAt(row.queryOrdinal) != query.Family() {
			return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
		}
		shape, shapeOK := schema.queryShapeAt(row.queryOrdinal)
		if !shapeOK || shape.Population != population.SelectedPoint {
			return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
		}
		if row.heterogeneous != nil {
			if shape.ProjectionCount == 0 || shape.ProjectionCount != uint64(len(row.heterogeneous.projections)) || len(query.Surfaces()) != len(row.heterogeneous.projections) || row.heterogeneous.exec == nil {
				return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
			}
			surfaces := query.Surfaces()
			for projectionIndex, pair := range row.heterogeneous.projections {
				projection, projectionOK := schema.queryProjectionShapeAt(row.queryOrdinal, uint64(projectionIndex))
				if !projectionOK || !validRuntimeQueryProjection(schema, factors, runtime, row.queryOrdinal, uint64(projectionIndex), pair) || !validProgramQuerySurface(surfaces[projectionIndex], projection) {
					return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
				}
			}
			continue
		}
		projection, projectionOK := schema.queryProjectionShapeAt(row.queryOrdinal, 0)
		unitSlot, unitOK := row.unit.Slot()
		if !projectionOK || shape.ProjectionCount != 1 || row.factorOrdinal >= uint64(len(factors)) || projection.Factor != factors[row.factorOrdinal].key || !unitOK || unitSlot != factors[row.factorOrdinal].slot || !runtime.OwnsUnit(unitSlot, row.unit) || projection.Kind == composition.QueryFactorExact && row.unit.Kind() != carrier.ExactUnit || projection.Kind == composition.QueryFactorSummary && row.unit.Kind() != carrier.SummaryUnit {
			return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
		}
	}
	for _, row := range observations {
		if !row.valid() || int(row.point) >= graph.PointCount() || row.queryOrdinal >= schema.queryCount() {
			return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
		}
		stateOK := row.state == contextfiber.StateOrdinal(row.point)
		if artifactBacked {
			contextOrdinal, contextOK := contextIndex.ContextOrdinal(row.contextID)
			canonicalContext, canonicalContextOK := contextLayout.ContextID(contextOrdinal)
			stateCell, cellOK := contextLayout.StateAt(row.state)
			statePoint, pointStateOK := stateCell.PointOrdinal()
			stateContext, stateContextOK := stateCell.ContextOrdinal()
			stateOK = row.contextID.Available() && contextOK && canonicalContextOK && canonicalContext == row.contextID && cellOK && pointStateOK && uint64(statePoint) == uint64(row.point) && stateContextOK && stateContext == contextOrdinal
		}
		if !stateOK {
			return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
		}
		shape, shapeOK := schema.queryShapeAt(row.queryOrdinal)
		if !shapeOK {
			return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
		}
		if row.heterogeneous != nil {
			if shape.ProjectionCount == 0 || shape.ProjectionCount != uint64(len(row.heterogeneous.projections)) || row.heterogeneous.exec == nil {
				return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
			}
			for projectionIndex, pair := range row.heterogeneous.projections {
				if !validRuntimeQueryProjection(schema, factors, runtime, row.queryOrdinal, uint64(projectionIndex), pair) {
					return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
				}
			}
			continue
		}
		projection, projectionOK := schema.queryProjectionShapeAt(row.queryOrdinal, 0)
		unitSlot, unitOK := row.unit.Slot()
		if !projectionOK || shape.ProjectionCount != 1 || row.factorOrdinal >= uint64(len(factors)) || projection.Factor != factors[row.factorOrdinal].key || !unitOK || unitSlot != factors[row.factorOrdinal].slot || !runtime.OwnsUnit(unitSlot, row.unit) || projection.Kind == composition.QueryFactorExact && row.unit.Kind() != carrier.ExactUnit || projection.Kind == composition.QueryFactorSummary && row.unit.Kind() != carrier.SummaryUnit {
			return nil, refuseProgramSeal(topologyConstructionStepQueryRow), false
		}
	}
	generatedPrograms, generatedPresent, generatedOK := sealedGeneratedPrograms(schema)
	if !generatedOK {
		return nil, refuseProgramSeal(topologyConstructionStepDeclarationShape), false
	}
	if generatedPresent {
		for _, row := range rows {
			if row.generated == nil {
				continue
			}
			if _, descriptorOK := generatedDescriptorAt(generatedPrograms, row.generated.rule); !descriptorOK {
				return nil, refuseProgramSeal(topologyConstructionStepMemberRow), false
			}
		}
	}
	program := &runtimeProgram{
		memberTable:       rows,
		groupSpans:        spans,
		generatedPrograms: generatedPrograms,
		generatedPresent:  generatedPresent,
		factorTable:       factors,
		factorOwners:      owners,
		queryTable:        queries,
		observationTable:  observations,
		programSealed:     true,
	}
	if generatedPresent {
		executionProgram, executionRefusal, executionOK := buildGeneratedExecutionProgram(program)
		if !executionOK {
			return nil, executionRefusal, false
		}
		program.generatedExecution = executionProgram
	}
	return program, topologyConstructionRefusal{}, true
}

// sealedGeneratedPrograms borrows the descriptor table validated during
// SchemaBuilder.Seal. Runtime links share that immutable owner; no occurrence
// row or runtime seal re-compiles or copies a descriptor.
func sealedGeneratedPrograms(schema *Schema) ([]generated.CompiledRule, bool, bool) {
	programs, present := schema.generatedProgramTable()
	if programs == nil && present {
		return nil, false, false
	}
	return programs, present, true
}

func generatedDescriptorAt(programs []generated.CompiledRule, ordinal uint32) (generated.CompiledRule, bool) {
	if ordinal >= uint32(len(programs)) {
		return generated.CompiledRule{}, false
	}
	descriptor := programs[ordinal]
	return descriptor, descriptor.Available()
}

func validRuntimeQueryProjection(schema *Schema, factors []factorRecord, runtime *carrier.Composition, queryOrdinal, projectionOrdinal uint64, pair queryProjectionRow) bool {
	if schema == nil || runtime == nil || pair.factorOrdinal >= uint64(len(factors)) {
		return false
	}
	projection, projectionOK := schema.queryProjectionShapeAt(queryOrdinal, projectionOrdinal)
	unitSlot, unitOK := pair.unit.Slot()
	if !projectionOK || projection.Factor != factors[pair.factorOrdinal].key || !unitOK || unitSlot != factors[pair.factorOrdinal].slot || !runtime.OwnsUnit(unitSlot, pair.unit) {
		return false
	}
	return projection.Kind == composition.QueryFactorExact && pair.unit.Kind() == carrier.ExactUnit || projection.Kind == composition.QueryFactorSummary && pair.unit.Kind() == carrier.SummaryUnit
}

// validQueryContextPlane authenticates the exact compact address plane a
// committed mounted program supplied. Query rows are allowed to retain a
// StateOrdinal only when the Index and Layout still belong to the same sealed
// directory, point shape, owner vector, and generation. A partially supplied
// plane is never completed with a default or an alias.
func validQueryContextPlane(graph *equation.Graph, contexts executioncontext.Directory, contextIndex contextfiber.Index, contextLayout contextfiber.Layout, pointOwners []contextfiber.PointOwner) bool {
	if graph == nil || !contexts.Available() || !contextIndex.Available() || !contextLayout.Available() || len(pointOwners) != graph.PointCount() {
		return false
	}
	generation := contextLayout.Generation()
	return generation != 0 && contextIndex.OwnedBy(contexts, graph.PointCount(), generation) && contextLayout.OwnedBy(contextIndex, contexts, pointOwners, generation)
}

// queryStateOrdinal resolves one retained equation Query through the exact
// context index and compact layout. Context identity is never inferred from a
// point owner, a module alias, or a default context. The caller must already
// have authenticated the plane with validQueryContextPlane when it is sealing
// a table; repeating the bounded coordinate checks here keeps direct binders
// fail-closed as well.
func queryStateOrdinal(graph *equation.Graph, query equation.Query, contexts executioncontext.Directory, contextIndex contextfiber.Index, contextLayout contextfiber.Layout, pointOwners []contextfiber.PointOwner) (contextfiber.StateOrdinal, bool) {
	if graph == nil || !validQueryContextPlane(graph, contexts, contextIndex, contextLayout, pointOwners) {
		return 0, false
	}
	return queryStateOrdinalOwned(graph, query, contextIndex, contextLayout)
}

// queryStateOrdinalOwned is the O(1) coordinate step after the context plane
// has been authenticated for this graph. Keeping plane authentication outside
// the query loop prevents a wide query family from multiplying the point-owner
// proof while retaining the same fail-closed identity checks.
func queryStateOrdinalOwned(graph *equation.Graph, query equation.Query, contextIndex contextfiber.Index, contextLayout contextfiber.Layout) (contextfiber.StateOrdinal, bool) {
	if graph == nil || !graph.OwnsQuery(query) || !query.ContextID().Available() {
		return 0, false
	}
	contextOrdinal, contextOK := contextIndex.ContextOrdinal(query.ContextID())
	pointIndex, pointOK := graph.PointIndex(query.Point())
	if !contextOK || !pointOK || pointIndex < 0 {
		return 0, false
	}
	return contextLayout.Lookup(contextOrdinal, contextfiber.PointOrdinal(pointIndex))
}

// valid reports the one decision sealRuntimeProgram already took. No later
// predicate re-derives a program's admissibility.
func (program *runtimeProgram) valid() bool {
	return program != nil && program.programSealed
}

func (program *runtimeProgram) memberCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.memberTable)
}

// memberRowAt returns the row by value.
func (program *runtimeProgram) memberRowAt(index int) (memberRow, bool) {
	if !program.valid() || index < 0 || index >= len(program.memberTable) {
		return memberRow{}, false
	}
	return program.memberTable[index], true
}

// memberRows is the hot fold's view of one Group's rows: a contiguous run of
// the sealed table, bounds-checked once per Group instead of once per member.
// The rows are immutable after the Seal, which the assignment law enforces.
func (program *runtimeProgram) memberRows(span memberSpan) []memberRow {
	if !program.valid() || span.start < 0 || span.end < span.start || int(span.end) > len(program.memberTable) {
		return nil
	}
	return program.memberTable[span.start:span.end]
}

func (program *runtimeProgram) generatedProgramAt(ordinal uint32) (generated.CompiledRule, bool) {
	if !program.valid() || !program.generatedPresent {
		return generated.CompiledRule{}, false
	}
	return generatedDescriptorAt(program.generatedPrograms, ordinal)
}

// memberRowIdentity returns the same canonical member used for execution after
// re-proving that the reporting Group contains it.
func memberRowIdentity(group equation.GroupNode, row memberRow) (equation.RuleMember, bool) {
	if !row.valid() {
		return equation.RuleMember{}, false
	}
	geometry, geometryOK := row.geometry()
	if !geometryOK {
		return equation.RuleMember{}, false
	}
	member := geometry.member()
	for index := 0; index < group.MemberCount(); index++ {
		candidate, ok := group.MemberAt(index)
		if ok && candidate.Key() == member.Key() {
			return member, true
		}
	}
	return equation.RuleMember{}, false
}

func (program *runtimeProgram) groupCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.groupSpans)
}

func (program *runtimeProgram) groupSpanAt(group int) (memberSpan, bool) {
	if !program.valid() || group < 0 || group >= len(program.groupSpans) {
		return memberSpan{}, false
	}
	return program.groupSpans[group], true
}

func (program *runtimeProgram) factorCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.factorTable)
}

func (program *runtimeProgram) factorRecordAt(index int) (factorRecord, bool) {
	if !program.valid() || index < 0 || index >= len(program.factorTable) {
		return factorRecord{}, false
	}
	return program.factorTable[index], true
}

// factorOwnerAt resolves the retained typed owner a record indexes. Route
// universes and typed observation stay owner-side exactly as they are today.
func (program *runtimeProgram) factorOwnerAt(owner int32) (runtimeFactor, bool) {
	if !program.valid() || owner < 0 || int(owner) >= len(program.factorOwners) {
		return nil, false
	}
	return program.factorOwners[owner], true
}

// factorOwnerByKey resolves a canonical Factor key through the sealed table.
// The table is ordered by key at bind time, so the lookup is a search over
// dense records rather than a hash of a 40-byte key into a retained map.
func (program *runtimeProgram) factorOwnerByKey(key composition.Key) (runtimeFactor, bool) {
	record, found := program.factorRecordByKey(key)
	if !found {
		return nil, false
	}
	return program.factorOwnerAt(record.owner)
}

func (program *runtimeProgram) factorRecordByKey(key composition.Key) (factorRecord, bool) {
	if !program.valid() || !key.Available() {
		return factorRecord{}, false
	}
	index := sort.Search(len(program.factorTable), func(position int) bool {
		return !lessRuntimeKey(program.factorTable[position].key, key)
	})
	if index >= len(program.factorTable) || program.factorTable[index].key != key {
		return factorRecord{}, false
	}
	return program.factorTable[index], true
}

func (program *runtimeProgram) queryCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.queryTable)
}

func (program *runtimeProgram) queryAt(index int) (queryRow, bool) {
	if !program.valid() || index < 0 || index >= len(program.queryTable) {
		return queryRow{}, false
	}
	return program.queryTable[index], true
}

func (program *runtimeProgram) materializeQuery(index int, work *carrier.Work, state carrier.State) (frozenValue, solveBoundary, bool) {
	row, ok := program.queryAt(index)
	if !ok {
		return nil, refused(SolveFailureFamilyObservation, "query-row"), false
	}
	if row.heterogeneous != nil {
		return row.heterogeneous.exec(work, state, program)
	}
	factor, ok := program.factorOwnerAt(int32(row.factorOrdinal))
	if !ok {
		return nil, refused(SolveFailureFamilyObservation, "factor-row"), false
	}
	return row.exec(work, state, factor, row.unit)
}

func (program *runtimeProgram) observationCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.observationTable)
}

func (program *runtimeProgram) observationAt(index int) (observationRow, bool) {
	if !program.valid() || index < 0 || index >= len(program.observationTable) {
		return observationRow{}, false
	}
	return program.observationTable[index], true
}

func (program *runtimeProgram) materializeObservation(index int, work *carrier.Work, state carrier.State) (frozenValue, solveBoundary, bool) {
	row, ok := program.observationAt(index)
	if !ok {
		return nil, refused(SolveFailureFamilyObservation, "observation-row"), false
	}
	if row.heterogeneous != nil {
		return row.heterogeneous.exec(work, state, program)
	}
	factor, ok := program.factorOwnerAt(int32(row.factorOrdinal))
	if !ok {
		return nil, refused(SolveFailureFamilyObservation, "factor-row"), false
	}
	return row.exec(work, state, factor, row.unit)
}

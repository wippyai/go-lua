package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// QueryProjectionSpec is one typed member of a heterogeneous query fold. The
// constructor closes the value type V at the binding boundary; no interface,
// reflection, or caller-selected Factor coordinate reaches the runtime row.
// Projection specs are intentionally values so callers can order unlike
// Factors in one HeterogeneousQuerySpec.
type QueryProjectionSpec[R any] struct {
	bind func(*schemaBindingState, uint64, uint64) (heterogeneousProjection[R], bool)
}

func (spec QueryProjectionSpec[R]) Available() bool { return spec.bind != nil }

// QueryProjectionFold is one typed projection reducer. BorrowIssued is an
// explicit synchronous-use receipt: when true, Accumulate must not retain the
// OrderedCells view or any value reachable only through it.
type QueryProjectionFold[V, R any] struct {
	Accumulate   func(R, OrderedCells[V]) (R, bool)
	BorrowIssued bool
}

func (fold QueryProjectionFold[V, R]) valid() bool { return fold.Accumulate != nil }

// HeterogeneousQuerySpec folds an ordered Factor projection vector into one
// result. Begin runs once, projection accumulators run in declaration order,
// and Result.Freeze runs once after the vector has completed.
type HeterogeneousQuerySpec[R any] struct {
	Begin       func() R
	Projections []QueryProjectionSpec[R]
	Result      FrozenResult[R]
	// TransferResult states that Begin creates fresh owner-controlled result
	// storage and projection reducers retain no aliases to the returned value.
	// Publication may therefore consume the final fold value without cloning.
	TransferResult bool
}

func (spec HeterogeneousQuerySpec[R]) valid() bool {
	if spec.Begin == nil || len(spec.Projections) == 0 || !validFrozenResult(spec.Result) {
		return false
	}
	for _, projection := range spec.Projections {
		if !projection.Available() {
			return false
		}
	}
	return true
}

// ExactQueryProjection constructs one exact-read projection. The FactorSlot
// and the typed accumulator are authenticated together when the query cell is
// bound, so a copied or foreign slot cannot cross a SchemaBinding fence.
func ExactQueryProjection[V, R any](factor *FactorSlot[V], fold QueryProjectionFold[V, R]) QueryProjectionSpec[R] {
	if factor == nil || !fold.valid() {
		return QueryProjectionSpec[R]{}
	}
	return QueryProjectionSpec[R]{bind: func(state *schemaBindingState, queryOrdinal, projectionOrdinal uint64) (heterogeneousProjection[R], bool) {
		if state == nil || state.schema == nil || factor.Schema() != state.schema {
			return heterogeneousProjection[R]{}, false
		}
		factorOrdinal, factorOK := factor.Ordinal()
		shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
		projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, projectionOrdinal)
		if !factorOK || factorOrdinal >= uint64(len(state.factors)) || factorOrdinal >= uint64(schemaFactorCount(state.schema)) || !shapeOK || !projectionOK || shape.ProjectionCount == 0 || projectionOrdinal >= shape.ProjectionCount || projection.Kind != composition.QueryFactorExact || projection.Factor != state.schema.factorSemanticAt(factorOrdinal) || projection.Normalizer.Available() {
			return heterogeneousProjection[R]{}, false
		}
		return heterogeneousProjection[R]{
			factor: factorSemanticKey(state.schema, factorOrdinal), factorOrdinal: factorOrdinal,
			kind: composition.QueryFactorExact, borrowIssued: fold.BorrowIssued,
			bindRuntime: func(runtime runtimeFactor) (heterogeneousProjectionRunner[R], bool) {
				typed, typedOK := runtime.(heterogeneousTypedFactor[V])
				if !typedOK || typed == nil {
					return nil, false
				}
				return func(work *carrier.Work, state carrier.State, value R, unit carrier.Unit) (R, solveBoundary, bool) {
					return runHeterogeneousProjection(work, state, value, typed, unit, fold.Accumulate, fold.BorrowIssued)
				}, true
			},
		}, true
	}}
}

// SummaryQueryProjection constructs one summary-read projection from the
// exact declared form token. The form supplies both the Factor and its
// normalizer semantic; callers cannot substitute an equal-looking identity.
func SummaryQueryProjection[V, R any](form SchemaReadForm[V], fold QueryProjectionFold[V, R]) QueryProjectionSpec[R] {
	if !fold.valid() {
		return QueryProjectionSpec[R]{}
	}
	return QueryProjectionSpec[R]{bind: func(state *schemaBindingState, queryOrdinal, projectionOrdinal uint64) (heterogeneousProjection[R], bool) {
		if state == nil || state.schema == nil || form.Schema() != state.schema || form.cell == nil || !summaryReadFormKind(form.Kind()) {
			return heterogeneousProjection[R]{}, false
		}
		factorOrdinal := form.cell.ordinal >> 32
		formOrdinal := uint64(uint32(form.cell.ordinal))
		if factorOrdinal >= uint64(len(state.factors)) || factorOrdinal >= uint64(schemaFactorCount(state.schema)) {
			return heterogeneousProjection[R]{}, false
		}
		formShape, formOK := state.schema.factorFormShapeAt(factorOrdinal, formOrdinal)
		shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
		projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, projectionOrdinal)
		factor := state.schema.factorSemanticAt(factorOrdinal)
		if !formOK || !shapeOK || !projectionOK || shape.ProjectionCount == 0 || projectionOrdinal >= shape.ProjectionCount || !factor.Available() || !summaryReadRowKind(formShape.Kind) || projection.Kind != composition.QueryFactorSummary || projection.Factor != factor || projection.Normalizer != formShape.Semantic || !projection.Normalizer.Available() {
			return heterogeneousProjection[R]{}, false
		}
		factorCell, factorBound := state.factors[factorOrdinal].(interface {
			schemaFactorSummaryKeys() ([]uint64, bool)
		})
		if !factorBound {
			return heterogeneousProjection[R]{}, false
		}
		keys, keysOK := factorCell.schemaFactorSummaryKeys()
		if !keysOK {
			return heterogeneousProjection[R]{}, false
		}
		return heterogeneousProjection[R]{
			factor: factor, factorOrdinal: factorOrdinal, kind: composition.QueryFactorSummary,
			normalizer: projection.Normalizer, summaryKeys: append([]uint64(nil), keys...), borrowIssued: fold.BorrowIssued,
			bindRuntime: func(runtime runtimeFactor) (heterogeneousProjectionRunner[R], bool) {
				typed, typedOK := runtime.(heterogeneousTypedFactor[V])
				if !typedOK || typed == nil {
					return nil, false
				}
				return func(work *carrier.Work, state carrier.State, value R, unit carrier.Unit) (R, solveBoundary, bool) {
					return runHeterogeneousProjection(work, state, value, typed, unit, fold.Accumulate, fold.BorrowIssued)
				}, true
			},
		}, true
	}}
}

// NewExactQueryProjection and NewSummaryQueryProjection are the constructor
// spellings used by code that prefers New-prefixed capability factories.
func NewExactQueryProjection[V, R any](factor *FactorSlot[V], fold QueryProjectionFold[V, R]) QueryProjectionSpec[R] {
	return ExactQueryProjection[V, R](factor, fold)
}

func NewSummaryQueryProjection[V, R any](form SchemaReadForm[V], fold QueryProjectionFold[V, R]) QueryProjectionSpec[R] {
	return SummaryQueryProjection[V, R](form, fold)
}

type heterogeneousProjection[R any] struct {
	factor        composition.Key
	factorOrdinal uint64
	kind          composition.QueryProjectionKind
	normalizer    composition.Key
	summaryKeys   []uint64
	borrowIssued  bool
	bindRuntime   func(runtimeFactor) (heterogeneousProjectionRunner[R], bool)
}

func (projection heterogeneousProjection[R]) valid(schema *Schema) bool {
	return schema != nil && projection.factor.Available() && projection.factorOrdinal < uint64(schemaFactorCount(schema)) && projection.kind != composition.QuerySupport && projection.bindRuntime != nil && schema.factorSemanticAt(projection.factorOrdinal) == projection.factor && (projection.kind == composition.QueryFactorExact && !projection.normalizer.Available() || projection.kind == composition.QueryFactorSummary && projection.normalizer.Available())
}

// heterogeneousProjectionRunner is prepared once while a committed program
// binds its sealed Factor owners. The runtime row still carries the
// authenticated Factor ordinal and Unit; this closure only removes the
// generic V type assertion from the solve hot path.
type heterogeneousProjectionRunner[R any] func(*carrier.Work, carrier.State, R, carrier.Unit) (R, solveBoundary, bool)

type schemaHeterogeneousQueryBindingCell[R any] struct {
	state          *schemaBindingState
	ordinal        uint64
	slot           *QuerySlot[R]
	projections    []heterogeneousProjection[R]
	result         FrozenResult[R]
	begin          func() R
	transferResult bool
}

func (cell *schemaHeterogeneousQueryBindingCell[R]) schemaBindingSchema() *Schema {
	if cell == nil || cell.state == nil {
		return nil
	}
	return cell.state.schema
}

func (cell *schemaHeterogeneousQueryBindingCell[R]) schemaQueryOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaHeterogeneousQueryBindingCell[R]) schemaQueryState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

func (cell *schemaHeterogeneousQueryBindingCell[R]) valid(phase schemaBindingPhase) bool {
	if cell == nil || cell.state == nil || cell.state.phase != phase || cell.state.schema == nil || !cell.state.schema.Available() || cell.slot == nil || cell.slot.Schema() != cell.state.schema || cell.ordinal >= uint64(len(cell.state.queries)) || cell.begin == nil || !validFrozenResult(cell.result) {
		return false
	}
	slotOrdinal, slotOK := cell.slot.Ordinal()
	shape, shapeOK := cell.state.schema.queryShapeAt(cell.ordinal)
	if !slotOK || slotOrdinal != cell.ordinal || !shapeOK || shape.ProjectionCount != uint64(len(cell.projections)) || shape.ProjectionCount == 0 || shape.Freezer != compositionKeyOf(cell.result.Semantic) {
		return false
	}
	for index, projection := range cell.projections {
		if !projection.valid(cell.state.schema) {
			return false
		}
		shape, shapeOK := cell.state.schema.queryProjectionShapeAt(cell.ordinal, uint64(index))
		if !shapeOK || shape.Kind != projection.kind || shape.Factor != projection.factor || shape.Normalizer != projection.normalizer {
			return false
		}
	}
	return true
}

func (cell *schemaHeterogeneousQueryBindingCell[R]) complete() bool {
	return cell.valid(schemaBindingOpen)
}

func (cell *schemaHeterogeneousQueryBindingCell[R]) sealed() bool {
	return cell.valid(schemaBindingSealed) && cell.state.authority != nil && cell.state.queries[cell.ordinal] == cell
}

// HeterogeneousQueryImplementation is one sealed ordered projection row.
type HeterogeneousQueryImplementation[R any] struct {
	row *schemaHeterogeneousQueryBindingCell[R]
}

func (implementation *HeterogeneousQueryImplementation[R]) sealedRow() (*schemaHeterogeneousQueryBindingCell[R], bool) {
	if implementation == nil || implementation.row == nil || !implementation.row.sealed() {
		return nil, false
	}
	return implementation.row, true
}

// BindHeterogeneousQuery installs one typed, ordered heterogeneous query.
// Every projection shape is checked before the row enters the binding; a
// mismatch poisons this binding and cannot leave a partially authenticated
// query cell behind.
func BindHeterogeneousQuery[R any](binding *SchemaBinding, query *QuerySlot[R], spec HeterogeneousQuerySpec[R]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.schema == nil || query == nil || query.Schema() != state.schema || !spec.valid() {
		state.poisonLocked()
		return false
	}
	queryOrdinal, queryOK := query.Ordinal()
	if !queryOK || queryOrdinal >= uint64(len(state.queries)) || state.queries[queryOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	projections := make([]heterogeneousProjection[R], len(spec.Projections))
	for index, projectionSpec := range spec.Projections {
		projection, projectionOK := projectionSpec.bind(state, queryOrdinal, uint64(index))
		if !projectionOK {
			state.poisonLocked()
			return false
		}
		projections[index] = projection
	}
	cell := &schemaHeterogeneousQueryBindingCell[R]{state: state, ordinal: queryOrdinal, slot: query, projections: projections, result: spec.Result, begin: spec.Begin, transferResult: spec.TransferResult}
	if !cell.complete() {
		state.poisonLocked()
		return false
	}
	state.queries[queryOrdinal] = cell
	return true
}

func HeterogeneousQueryImplementationAt[R any](binding *SchemaBinding, slot *QuerySlot[R]) (*HeterogeneousQueryImplementation[R], bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil || slot == nil || slot.Schema() != state.schema {
		return nil, false
	}
	ordinal, ok := slot.Ordinal()
	if !ok || ordinal >= uint64(len(state.queries)) {
		return nil, false
	}
	row, ok := state.queries[ordinal].(*schemaHeterogeneousQueryBindingCell[R])
	if !ok || !row.sealed() || row.slot != slot {
		return nil, false
	}
	return &HeterogeneousQueryImplementation[R]{row: row}, true
}

func factorSemanticKey(schema *Schema, ordinal uint64) composition.Key {
	if schema == nil || ordinal >= uint64(schemaFactorCount(schema)) {
		return composition.Key{}
	}
	return schema.factorSemanticAt(ordinal)
}

type heterogeneousTypedFactor[V any] interface {
	stagedObserveWithFailure(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[V], support.Mask) bool) (stagedObservationFailure, bool)
}

func runHeterogeneousProjection[V, R any](work *carrier.Work, input carrier.State, value R, factor heterogeneousTypedFactor[V], unit carrier.Unit, accumulate func(R, OrderedCells[V]) (R, bool), borrowIssued bool) (R, solveBoundary, bool) {
	if factor == nil || accumulate == nil || work == nil || !work.Checkpoint() {
		return value, refused(SolveFailureFamilyObservation, "preflight"), false
	}
	failureBoundary := boundaryNone
	visit := func(observation factbinding.Observation[V], _ support.Mask) bool {
		cells, cellsOK := orderedCellsFromObservation(observation, borrowIssued)
		if !cellsOK {
			failureBoundary = refused(SolveFailureFamilyObservation, "shape")
			return false
		}
		next, accepted := accumulate(value, cells)
		if !accepted {
			failureBoundary = refused(SolveFailureFamilyObservation, "projection")
			return false
		}
		value = next
		return work.Checkpoint()
	}
	failure, valid := factor.stagedObserveWithFailure(work, input, unit, input.Support(), visit)
	if !valid || !work.Checkpoint() {
		if failureBoundary != boundaryNone {
			return value, failureBoundary, false
		}
		return value, heterogeneousObservationBoundary(failure), false
	}
	return value, boundaryNone, true
}

func heterogeneousObservationBoundary(failure stagedObservationFailure) solveBoundary {
	switch failure {
	case stagedObservationFailureArguments:
		return refused(SolveFailureFamilyObservation, "preflight")
	case stagedObservationFailureCheckpoint, stagedObservationFailureSlot, stagedObservationFailureWork, stagedObservationFailureVisitor:
		return refused(SolveFailureFamilyObservation, "work")
	case stagedObservationFailureUnit:
		return refused(SolveFailureFamilyObservation, "unit")
	case stagedObservationFailureSupport:
		return refused(SolveFailureFamilyObservation, "support")
	case stagedObservationFailureRoot:
		return refused(SolveFailureFamilyObservation, "root")
	case stagedObservationFailureCarrier:
		return refused(SolveFailureFamilyObservation, "carrier")
	case stagedObservationFailureDecode:
		return refused(SolveFailureFamilyObservation, "decode")
	default:
		return refused(SolveFailureFamilyObservation, "projection")
	}
}

func materializeHeterogeneousQuery[R any](work *carrier.Work, state carrier.State, runners []heterogeneousProjectionRunner[R], result FrozenResult[R], begin func() R, transferResult bool, rows []queryProjectionRow) (frozenValue, solveBoundary, bool) {
	if begin == nil || !validFrozenResult(result) || work == nil || !work.Checkpoint() {
		return nil, refused(SolveFailureFamilyObservation, "preflight"), false
	}
	if len(runners) == 0 || len(runners) != len(rows) {
		return nil, refused(SolveFailureFamilyObservation, "factor-row"), false
	}
	value := begin()
	for index, run := range runners {
		if run == nil {
			return nil, refused(SolveFailureFamilyObservation, "factor-row"), false
		}
		var boundary solveBoundary
		var ok bool
		value, boundary, ok = run(work, state, value, rows[index].unit)
		if !ok {
			return nil, boundary, false
		}
		if !work.Checkpoint() {
			return nil, refused(SolveFailureFamilyObservation, "work"), false
		}
	}
	frozen := value
	if !transferResult {
		frozen = result.Freeze(value)
	}
	if !work.Checkpoint() {
		return nil, refused(SolveFailureFamilyObservation, "freeze"), false
	}
	return &typedFrozenValue[R]{value: frozen, freeze: result}, boundaryNone, true
}

func (implementation *HeterogeneousQueryImplementation[R]) declareMountedQuery(state *schemaBindingState, authority *schemaBindingAuthority, context executioncontext.Context, id, mount, point identity.ContentID) (declaredQueryRow, []*ruleSummaryMapping, bool) {
	row, ok := implementation.sealedRow()
	if !ok || row.state != state || state.authority != authority || !context.Available() || !id.Available() || !mount.Available() || !point.Available() {
		return declaredQueryRow{}, nil, false
	}
	family := state.schema.querySemanticAt(row.ordinal)
	if !family.Available() {
		return declaredQueryRow{}, nil, false
	}
	surfaces := make([]equation.Surface, len(row.projections))
	mappings := make([]*ruleSummaryMapping, 0, len(row.projections))
	for index, projection := range row.projections {
		if !projection.valid(state.schema) {
			return declaredQueryRow{}, nil, false
		}
		surface := equation.Surface{Factor: projection.factor, Form: equation.SurfaceReadExact, Local: 1}
		if projection.kind == composition.QueryFactorSummary {
			surface.Form = equation.SurfaceReadSummary
			surface.Semantic = projection.normalizer
			surface.Normalizer = projection.normalizer
			mappings = append(mappings, &ruleSummaryMapping{state: state, authority: authority, factor: projection.factor, normalizer: projection.normalizer, surface: surface, keys: newSummaryKeyVector(projection.summaryKeys)})
		}
		if !surface.Available() {
			return declaredQueryRow{}, nil, false
		}
		surfaces[index] = surface
	}
	return declaredQueryRow{Context: context, ID: id, Mount: mount, Point: point, Row: equation.QueryInstance{Context: context.ID(), Family: family, Surfaces: surfaces}}, mappings, true
}

func (implementation *HeterogeneousQueryImplementation[R]) bindProgramQuery(plane *programPlane, query equation.Query) (queryRow, bool) {
	row, ok := implementation.sealedRow()
	if !ok {
		return queryRow{}, false
	}
	return bindHeterogeneousProgramQueryRow(plane, query, row.state, row.ordinal, row.projections, row.result, row.begin, row.transferResult)
}

func bindHeterogeneousProgramQueryRow[R any](plane *programPlane, query equation.Query, state *schemaBindingState, queryOrdinal uint64, projections []heterogeneousProjection[R], result FrozenResult[R], begin func() R, transferResult bool) (queryRow, bool) {
	if plane == nil || !plane.frozen || plane.runtime == nil || plane.runtime.graph == nil || state == nil || state != plane.runtime.state || state.authority != plane.runtime.authority || !plane.runtime.graph.OwnsQuery(query) || !query.Key().Available() || queryOrdinal >= state.schema.queryCount() || query.Family() != state.schema.querySemanticAt(queryOrdinal) || len(query.Surfaces()) != len(projections) || len(projections) == 0 {
		return queryRow{}, false
	}
	shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
	if !shapeOK || shape.ProjectionCount != uint64(len(projections)) || shape.ProjectionCount == 0 {
		return queryRow{}, false
	}
	surfaces := query.Surfaces()
	pairs := make([]queryProjectionRow, len(projections))
	runners := make([]heterogeneousProjectionRunner[R], len(projections))
	for index, projection := range projections {
		shape, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, uint64(index))
		if !projectionOK || !projection.valid(state.schema) || !validProgramQuerySurface(surfaces[index], shape) || projection.factor != shape.Factor || projection.kind != shape.Kind || projection.normalizer != shape.Normalizer || projection.factorOrdinal >= uint64(len(plane.factors)) || projection.factor != state.schema.factorSemanticAt(projection.factorOrdinal) {
			return queryRow{}, false
		}
		factor := plane.factors[projection.factorOrdinal]
		unit, unitOK := factor.readUnit(surfaces[index])
		if !unitOK {
			return queryRow{}, false
		}
		runner, runnerOK := projection.bindRuntime(factor)
		if !runnerOK || runner == nil {
			return queryRow{}, false
		}
		pairs[index] = queryProjectionRow{factorOrdinal: projection.factorOrdinal, unit: unit}
		runners[index] = runner
	}
	point, pointOK := plane.runtime.graph.PointIndex(query.Point())
	if !pointOK {
		return queryRow{}, false
	}
	stateOrdinal, stateOK := plane.queryState(query)
	if !stateOK {
		return queryRow{}, false
	}
	exec := func(work *carrier.Work, state carrier.State, program *runtimeProgram) (frozenValue, solveBoundary, bool) {
		_ = program
		return materializeHeterogeneousQuery(work, state, runners, result, begin, transferResult, pairs)
	}
	row := queryRow{queryOrdinal: queryOrdinal, point: int32(point), state: stateOrdinal, heterogeneous: &heterogeneousQueryRow{projections: pairs, exec: exec}}
	return row, row.valid()
}

func (implementation *HeterogeneousQueryImplementation[R]) bindProgramObservation(plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point, context executioncontext.Context, _ RuleReadSurface, _ bool) (observationRow, bool) {
	row, ok := implementation.sealedRow()
	if !ok || plane == nil || plane.runtime == nil || !plane.runtime.graph.OwnsMember(member) {
		return observationRow{}, false
	}
	return bindHeterogeneousObservationRow(plane, id, member, point, context, row.state, row.ordinal, row.projections, row.result, row.begin, row.transferResult)
}

func bindHeterogeneousObservationRow[R any](plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point, context executioncontext.Context, state *schemaBindingState, queryOrdinal uint64, projections []heterogeneousProjection[R], result FrozenResult[R], begin func() R, transferResult bool) (observationRow, bool) {
	if plane == nil || !plane.frozen || plane.runtime == nil || plane.runtime.graph == nil || state == nil || state != plane.runtime.state || state.authority != plane.runtime.authority || !id.Available() || !plane.runtime.graph.OwnsPoint(point) || queryOrdinal >= state.schema.queryCount() {
		return observationRow{}, false
	}
	shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
	if !shapeOK || shape.ProjectionCount != uint64(len(projections)) || shape.ProjectionCount == 0 {
		return observationRow{}, false
	}
	pairs := make([]queryProjectionRow, len(projections))
	runners := make([]heterogeneousProjectionRunner[R], len(projections))
	for index, projection := range projections {
		projectionShape, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, uint64(index))
		if !projectionOK || !projection.valid(state.schema) || projectionShape.Kind != projection.kind || projectionShape.Factor != projection.factor || projectionShape.Normalizer != projection.normalizer || projection.factorOrdinal >= uint64(len(plane.factors)) {
			return observationRow{}, false
		}
		var surface equation.Surface
		if projection.kind == composition.QueryFactorExact {
			var surfaceOK bool
			surface, surfaceOK = exactObservationReadSurfaceForFactor(member, projection.factor)
			if !surfaceOK {
				return observationRow{}, false
			}
		} else {
			surface = equation.Surface{Factor: projection.factor, Form: equation.SurfaceReadSummary, Semantic: projection.normalizer, Normalizer: projection.normalizer, Local: 1}
			if !surface.Available() {
				return observationRow{}, false
			}
			keyRange, ranged := plane.runtime.graph.SummaryKeyRange(surface)
			if !ranged || keyRange.Count() != len(projection.summaryKeys) {
				return observationRow{}, false
			}
			for keyIndex, expected := range projection.summaryKeys {
				actual, present := keyRange.At(keyIndex)
				if !present || actual != expected {
					return observationRow{}, false
				}
			}
		}
		if !validProgramQuerySurface(surface, projectionShape) {
			return observationRow{}, false
		}
		factor := plane.factors[projection.factorOrdinal]
		unit, unitOK := factor.readUnit(surface)
		if !unitOK {
			return observationRow{}, false
		}
		runner, runnerOK := projection.bindRuntime(factor)
		if !runnerOK || runner == nil {
			return observationRow{}, false
		}
		pairs[index] = queryProjectionRow{factorOrdinal: projection.factorOrdinal, unit: unit}
		runners[index] = runner
	}
	pointIndex, pointOK := plane.runtime.graph.PointIndex(point)
	if !pointOK {
		return observationRow{}, false
	}
	stateOrdinal, stateOK := plane.observationState(point, context)
	if !stateOK {
		return observationRow{}, false
	}
	exec := func(work *carrier.Work, state carrier.State, program *runtimeProgram) (frozenValue, solveBoundary, bool) {
		_ = program
		return materializeHeterogeneousQuery(work, state, runners, result, begin, transferResult, pairs)
	}
	row := observationRow{id: id, queryOrdinal: queryOrdinal, point: int32(pointIndex), state: stateOrdinal, contextID: context.ID(), heterogeneous: &heterogeneousQueryRow{projections: pairs, exec: exec}}
	return row, row.valid()
}

// Program query and observation admissions retain only this interface, so the
// concrete projection value types and the binding cell never cross the sealed
// program boundary.
func NewHeterogeneousQueryAdmission[R any](implementation *HeterogeneousQueryImplementation[R], id, mount, point identity.ContentID, context executioncontext.Context) (ProgramQueryAdmission, bool) {
	if implementation == nil || !context.Available() || !id.Available() || !mount.Available() || !point.Available() {
		return ProgramQueryAdmission{}, false
	}
	return ProgramQueryAdmission{admit: implementation, Context: context, ID: id, Mount: mount, Point: point}, true
}

func NewHeterogeneousObservationAdmission[R any](implementation *HeterogeneousQueryImplementation[R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	admission := ProgramObservationAdmission{admit: implementation, memberPoint: point, ID: id, Role: role, Mount: mount, Point: point, Occurrence: occurrence, Context: context}
	return admission, admission.Available()
}

// NewHeterogeneousCallInputObservationAdmission admits a heterogeneous query
// over the authenticated input state of one committed Call stage. The stage's
// output point authenticates its attached rule member; its input point selects
// the state read by the query. Keeping those coordinates distinct prevents a
// pre-effect consumer from fabricating a member at the predecessor point.
func NewHeterogeneousCallInputObservationAdmission[R any](implementation *HeterogeneousQueryImplementation[R], id identity.ContentID, stage ProgramCallStage, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if implementation == nil || !stage.Available() {
		return ProgramObservationAdmission{}, false
	}
	return newCallInputObservationAdmission(implementation, id, stage, context)
}

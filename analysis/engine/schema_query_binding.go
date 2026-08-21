package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// QueryFold is the ordered projection reducer used by a query row. A fold is
// configured only when both callbacks are present; otherwise Project is used.
type QueryFold[I, R any] struct {
	Begin      func() R
	Accumulate func(R, I) (R, bool)
	// BorrowIssued declares that Accumulate consumes I synchronously and never
	// retains it in R or elsewhere. The runtime may therefore present the
	// issuer's generation-fenced view instead of reconstructing owned cells.
	BorrowIssued bool
	// TransferResult declares that Begin creates fresh owner-controlled fold
	// storage and Accumulate retains no alias to its returned R. Publication
	// may consume that final R instead of cloning it through Freeze.
	TransferResult bool
}

func (fold QueryFold[I, R]) valid() bool {
	return (fold.Begin == nil) == (fold.Accumulate == nil)
}

// HotExactQuerySpec supplies the typed implementation of one schema-declared
// exact query row. Exactly one of Project and Fold must be configured.
type HotExactQuerySpec[V, R any] struct {
	Project func(OrderedCells[V]) R
	Fold    QueryFold[OrderedCells[V], R]
	Result  FrozenResult[R]
}

// HotSummaryQuerySpec supplies the typed implementation of one
// schema-declared summary query row. The summary form itself remains in the
// sealed Schema.
type HotSummaryQuerySpec[V, R any] struct {
	Project func(OrderedCells[V]) R
	Fold    QueryFold[OrderedCells[V], R]
	Result  FrozenResult[R]
}

// factorProjection is the hot, typed part of a schema query row. It has no
// graph identity, factor owner, or publication authority; those are resolved
// once when runtimeProgram is sealed.
type factorProjection[V, R any] struct {
	project        func(OrderedCells[V]) R
	begin          func() R
	accumulate     func(R, OrderedCells[V]) (R, bool)
	borrowIssued   bool
	transferResult bool
	result         FrozenResult[R]
}

func (projection factorProjection[V, R]) valid() bool {
	fold := projection.begin != nil || projection.accumulate != nil
	return (projection.project != nil) != fold && (!fold || projection.begin != nil && projection.accumulate != nil) && validFrozenResult(projection.result)
}

func exactFactorProjection[V, R any](spec HotExactQuerySpec[V, R]) factorProjection[V, R] {
	return factorProjection[V, R]{
		project: spec.Project, begin: spec.Fold.Begin, accumulate: spec.Fold.Accumulate,
		borrowIssued: spec.Fold.BorrowIssued, transferResult: spec.Fold.TransferResult,
		result: spec.Result,
	}
}

func summaryFactorProjection[V, R any](spec HotSummaryQuerySpec[V, R]) factorProjection[V, R] {
	return factorProjection[V, R]{
		project: spec.Project, begin: spec.Fold.Begin, accumulate: spec.Fold.Accumulate,
		borrowIssued: spec.Fold.BorrowIssued, transferResult: spec.Fold.TransferResult,
		result: spec.Result,
	}
}

func (schema *Schema) queryCount() uint64 {
	if schema == nil || !schema.Available() {
		return 0
	}
	_, _, queries, _, ok := schema.shapeCount()
	if !ok {
		return 0
	}
	return uint64(queries)
}

func validBindingQueryInstance(schema *Schema, ordinal uint64, query equation.QueryInstance) bool {
	if schema == nil || !schema.Available() || ordinal >= schema.queryCount() || query.Point == 0 {
		return false
	}
	shape, ok := schema.queryShapeAt(ordinal)
	if !ok || shape.ProjectionCount != 1 || len(query.Surfaces) != 1 {
		return false
	}
	projection, ok := schema.queryProjectionShapeAt(ordinal, 0)
	if !ok {
		return false
	}
	surface := query.Surfaces[0]
	if !surface.Available() || surface.Factor != projection.Factor || surface.Mode != equation.TargetModeNone {
		return false
	}
	switch projection.Kind {
	case composition.QueryFactorExact:
		return surface.Form == equation.SurfaceReadExact && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.QueryFactorSummary:
		return surface.Form == equation.SurfaceReadSummary && surface.Semantic.Available() && surface.Semantic == projection.Normalizer && surface.Normalizer == projection.Normalizer
	default:
		return false
	}
}

func duplicateBindingQuery(rows []equation.QueryInstance, query equation.QueryInstance) bool {
	for _, prior := range rows {
		if prior.Family != query.Family || prior.Point != query.Point || len(prior.Surfaces) != len(query.Surfaces) {
			continue
		}
		match := true
		for index := range prior.Surfaces {
			if prior.Surfaces[index] != query.Surfaces[index] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

type schemaQueryBindingCell interface {
	complete() bool
	schemaQueryOrdinal() uint64
	schemaQueryState() *schemaBindingState
}

// schemaExactQueryBindingCell is one sealed schema row plus its typed
// projection contract. The state and slot pointers are the binding-generation
// and slot-identity fences; runtime execution does not carry either one.
type schemaExactQueryBindingCell[V, R any] struct {
	state         *schemaBindingState
	ordinal       uint64
	factorOrdinal uint64
	slot          *QuerySlot[R]
	projection    factorProjection[V, R]
}

func (cell *schemaExactQueryBindingCell[V, R]) schemaBindingSchema() *Schema {
	if cell == nil || cell.state == nil {
		return nil
	}
	return cell.state.schema
}

func (cell *schemaExactQueryBindingCell[V, R]) schemaQueryOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaExactQueryBindingCell[V, R]) schemaQueryState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

func (cell *schemaExactQueryBindingCell[V, R]) valid(phase schemaBindingPhase) bool {
	if cell == nil || cell.state == nil || cell.state.phase != phase || cell.state.schema == nil || !cell.state.schema.Available() || cell.slot == nil || cell.slot.Schema() != cell.state.schema || !cell.projection.valid() || cell.ordinal >= uint64(len(cell.state.queries)) || cell.factorOrdinal >= uint64(len(cell.state.factors)) {
		return false
	}
	slotOrdinal, slotOK := cell.slot.Ordinal()
	shape, shapeOK := cell.state.schema.queryShapeAt(cell.ordinal)
	projection, projectionOK := cell.state.schema.queryProjectionShapeAt(cell.ordinal, 0)
	return slotOK && slotOrdinal == cell.ordinal && shapeOK && projectionOK && shape.ProjectionCount == 1 && shape.Freezer == compositionKeyOf(cell.projection.result.Semantic) && projection.Kind == composition.QueryFactorExact && projection.Factor.Available() && !projection.Normalizer.Available() && cell.state.schema.factorSemanticAt(cell.factorOrdinal) == projection.Factor
}

func (cell *schemaExactQueryBindingCell[V, R]) complete() bool {
	return cell.valid(schemaBindingOpen)
}

func (cell *schemaExactQueryBindingCell[V, R]) sealed() bool {
	return cell.valid(schemaBindingSealed) && cell.state.authority != nil && cell.state.queries[cell.ordinal] == cell
}

// schemaSummaryQueryBindingCell is the corresponding summary row. Its closed
// key plane is issued by the bound factor while the SchemaBinding is open.
type schemaSummaryQueryBindingCell[V, R any] struct {
	state         *schemaBindingState
	ordinal       uint64
	factorOrdinal uint64
	slot          *QuerySlot[R]
	keys          []uint64
	projection    factorProjection[V, R]
}

func (cell *schemaSummaryQueryBindingCell[V, R]) schemaBindingSchema() *Schema {
	if cell == nil || cell.state == nil {
		return nil
	}
	return cell.state.schema
}

func (cell *schemaSummaryQueryBindingCell[V, R]) schemaQueryOrdinal() uint64 {
	if cell == nil {
		return 0
	}
	return cell.ordinal
}

func (cell *schemaSummaryQueryBindingCell[V, R]) schemaQueryState() *schemaBindingState {
	if cell == nil {
		return nil
	}
	return cell.state
}

func (cell *schemaSummaryQueryBindingCell[V, R]) valid(phase schemaBindingPhase) bool {
	if cell == nil || cell.state == nil || cell.state.phase != phase || cell.state.schema == nil || !cell.state.schema.Available() || cell.slot == nil || cell.slot.Schema() != cell.state.schema || len(cell.keys) == 0 || !cell.projection.valid() || cell.ordinal >= uint64(len(cell.state.queries)) || cell.factorOrdinal >= uint64(len(cell.state.factors)) {
		return false
	}
	slotOrdinal, slotOK := cell.slot.Ordinal()
	shape, shapeOK := cell.state.schema.queryShapeAt(cell.ordinal)
	projection, projectionOK := cell.state.schema.queryProjectionShapeAt(cell.ordinal, 0)
	return slotOK && slotOrdinal == cell.ordinal && shapeOK && projectionOK && shape.ProjectionCount == 1 && shape.Freezer == compositionKeyOf(cell.projection.result.Semantic) && projection.Kind == composition.QueryFactorSummary && projection.Factor.Available() && projection.Normalizer.Available() && cell.state.schema.factorSemanticAt(cell.factorOrdinal) == projection.Factor
}

func (cell *schemaSummaryQueryBindingCell[V, R]) complete() bool {
	return cell.valid(schemaBindingOpen)
}

func (cell *schemaSummaryQueryBindingCell[V, R]) sealed() bool {
	return cell.valid(schemaBindingSealed) && cell.state.authority != nil && cell.state.queries[cell.ordinal] == cell
}

// ExactQueryImplementation is an opaque handle to one sealed exact query row.
type ExactQueryImplementation[V, R any] struct {
	row *schemaExactQueryBindingCell[V, R]
}

func (implementation *ExactQueryImplementation[V, R]) sealedRow() (*schemaExactQueryBindingCell[V, R], bool) {
	if implementation == nil || implementation.row == nil || !implementation.row.sealed() {
		return nil, false
	}
	return implementation.row, true
}

// SummaryQueryImplementation is an opaque handle to one sealed summary row.
type SummaryQueryImplementation[V, R any] struct {
	row *schemaSummaryQueryBindingCell[V, R]
}

func (implementation *SummaryQueryImplementation[V, R]) sealedRow() (*schemaSummaryQueryBindingCell[V, R], bool) {
	if implementation == nil || implementation.row == nil || !implementation.row.sealed() {
		return nil, false
	}
	return implementation.row, true
}

// BindExactQuery installs the typed implementation of one schema-declared
// exact query row.
func BindExactQuery[V, R any](binding *SchemaBinding, query *QuerySlot[R], factor *FactorSlot[V], spec HotExactQuerySpec[V, R]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	projectionContract := exactFactorProjection(spec)
	if state.phase != schemaBindingOpen || state.schema == nil || query == nil || factor == nil || query.Schema() != state.schema || factor.Schema() != state.schema || !projectionContract.valid() {
		state.poisonLocked()
		return false
	}
	queryOrdinal, queryOK := query.Ordinal()
	factorOrdinal, factorOK := factor.Ordinal()
	if !queryOK || !factorOK || queryOrdinal >= uint64(len(state.queries)) || factorOrdinal >= uint64(len(state.factors)) || state.queries[queryOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	cell := &schemaExactQueryBindingCell[V, R]{state: state, ordinal: queryOrdinal, factorOrdinal: factorOrdinal, slot: query, projection: projectionContract}
	if !cell.complete() {
		state.poisonLocked()
		return false
	}
	state.queries[queryOrdinal] = cell
	return true
}

// BindSummaryQuery installs the typed implementation of one schema-declared
// summary query row and validates its exact schema form.
func BindSummaryQuery[V, R any](binding *SchemaBinding, query *QuerySlot[R], factor *FactorSlot[V], form SchemaReadForm[V], spec HotSummaryQuerySpec[V, R]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	projectionContract := summaryFactorProjection(spec)
	if state.phase != schemaBindingOpen || state.schema == nil || query == nil || factor == nil || query.Schema() != state.schema || factor.Schema() != state.schema || form.Schema() != state.schema || !summaryReadFormKind(form.Kind()) || !projectionContract.valid() {
		state.poisonLocked()
		return false
	}
	queryOrdinal, queryOK := query.Ordinal()
	factorOrdinal, factorOK := factor.Ordinal()
	if !queryOK || !factorOK || queryOrdinal >= uint64(len(state.queries)) || factorOrdinal >= uint64(len(state.factors)) || form.cell == nil || state.queries[queryOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	formFactor := form.cell.ordinal >> 32
	formOrdinal := uint64(uint32(form.cell.ordinal))
	shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
	queryProjection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	formShape, formShapeOK := state.schema.factorFormShapeAt(factorOrdinal, formOrdinal)
	if !shapeOK || !projectionOK || !formShapeOK || shape.ProjectionCount != 1 || queryProjection.Kind != composition.QueryFactorSummary || queryProjection.Factor != state.schema.factorSemanticAt(factorOrdinal) || queryProjection.Normalizer != formShape.Semantic || formFactor != factorOrdinal || !summaryReadRowKind(formShape.Kind) {
		state.poisonLocked()
		return false
	}
	factorCell, factorBound := state.factors[factorOrdinal].(interface {
		schemaFactorSummaryKeys() ([]uint64, bool)
	})
	if !factorBound {
		state.poisonLocked()
		return false
	}
	keys, keysIssued := factorCell.schemaFactorSummaryKeys()
	if !keysIssued {
		state.poisonLocked()
		return false
	}
	cell := &schemaSummaryQueryBindingCell[V, R]{state: state, ordinal: queryOrdinal, factorOrdinal: factorOrdinal, slot: query, keys: keys, projection: projectionContract}
	if !cell.complete() {
		state.poisonLocked()
		return false
	}
	state.queries[queryOrdinal] = cell
	return true
}

func ExactQueryImplementationAt[V, R any](binding *SchemaBinding, slot *QuerySlot[R]) (*ExactQueryImplementation[V, R], bool) {
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
	row, ok := state.queries[ordinal].(*schemaExactQueryBindingCell[V, R])
	if !ok || !row.sealed() || row.slot != slot {
		return nil, false
	}
	return &ExactQueryImplementation[V, R]{row: row}, true
}

func SummaryQueryImplementationAt[V, R any](binding *SchemaBinding, slot *QuerySlot[R]) (*SummaryQueryImplementation[V, R], bool) {
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
	row, ok := state.queries[ordinal].(*schemaSummaryQueryBindingCell[V, R])
	if !ok || !row.sealed() || row.slot != slot {
		return nil, false
	}
	return &SummaryQueryImplementation[V, R]{row: row}, true
}

func (implementation *SummaryQueryImplementation[V, R]) topologySummaryMapping(surface equation.Surface) (equation.SummaryMapping, bool) {
	row, ok := implementation.sealedRow()
	if !ok {
		return equation.SummaryMapping{}, false
	}
	projection, ok := row.state.schema.queryProjectionShapeAt(row.ordinal, 0)
	if !ok || !surface.Available() || surface.Factor != projection.Factor || surface.Form != equation.SurfaceReadSummary || surface.Semantic != projection.Normalizer || surface.Normalizer != projection.Normalizer || surface.Mode != equation.TargetModeNone || len(row.keys) == 0 {
		return equation.SummaryMapping{}, false
	}
	return equation.SummaryMapping{Surface: surface, Keys: row.keys}, true
}

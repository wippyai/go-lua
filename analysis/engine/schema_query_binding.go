package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// Solver-side query binding cells and sealed implementations; the compile-side
// receipt constructors live in schema_query_receipt.go.

// QueryFold is the receipt-native ordered projection reducer. A fold is
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

// HotExactQuerySpec is the Link-local half of an exact Factor Query.  The
// cold QuerySlot carries only the semantic family and its one projection;
// this spec supplies exactly one typed Project or ordered-row Fold and its
// result persistence law. Exact read shape does not imply one observation.
// No callback is retained by Schema or by a QuerySlot.
type HotExactQuerySpec[V, R any] struct {
	Project func(OrderedCells[V]) R
	Fold    QueryFold[OrderedCells[V], R]
	Result  FrozenResult[R]
}

// HotSummaryQuerySpec is the Link-local half of a summary Factor Query. The
// exact summary form is supplied separately to BindSummaryQuery; keeping it
// as a typed token prevents a caller from replacing the normalizer named by
// the cold Query projection.
type HotSummaryQuerySpec[V, R any] struct {
	Project func(OrderedCells[V]) R
	Fold    QueryFold[OrderedCells[V], R]
	Result  FrozenResult[R]
}

// bindingQueryReceipt is the topology builder's narrow query authority. It
// exposes only the shared lifecycle, authority, Schema, and canonical query
// family; the typed projector and factor form stay behind the implementation
// receipt.
type bindingQueryReceipt interface {
	boundTopologyQueryReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, uint64, bool)
}

// receiptQueryOwner is the common runtime proof for SchemaBinding queries.
// It carries no callback or cold row and validates the graph-owned query
// family against the exact sealed binding at execution time.
type receiptQueryOwner struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	ordinal   uint64
}

func (owner *receiptQueryOwner) validQueryOwner(runtime *solverRuntime, identity equation.Query) bool {
	return owner != nil && runtime != nil && runtime.graph != nil && owner.state != nil && owner.authority != nil && owner.schema != nil && owner.state.schema == owner.schema && owner.state.phase == schemaBindingSealed && owner.state.authority == owner.authority &&
		runtime.bindingState == owner.state && runtime.bindingAuthority == owner.authority &&
		runtime.graph.OwnsQuery(identity) && owner.ordinal < owner.schema.queryCount() && owner.schema.querySemanticAt(owner.ordinal) == identity.Family()
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

func (implementation *ExactQueryImplementation[V, R]) boundTopologyQueryReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, uint64, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return nil, nil, composition.Key{}, 0, false
	}
	return implementation.binding.state, implementation.binding.authority, implementation.binding.state.schema.querySemanticAt(implementation.binding.queryOrdinal), implementation.binding.queryOrdinal, true
}

func (implementation *SummaryQueryImplementation[V, R]) boundTopologyQueryReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, uint64, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return nil, nil, composition.Key{}, 0, false
	}
	return implementation.binding.state, implementation.binding.authority, implementation.binding.state.schema.querySemanticAt(implementation.binding.queryOrdinal), implementation.binding.queryOrdinal, true
}

// schemaExactQueryBindingCell is the sole owner of one hot exact-query
// projector.  It is deliberately schema-bound: equal cold Schemas cannot
// exchange cells or receipts because state and authority are checked by
// pointer at every boundary.
type schemaExactQueryBindingCell[V, R any] struct {
	state          *schemaBindingState
	schema         *Schema
	ordinal        uint64
	factorOrdinal  uint64
	factor         composition.Key
	project        func(OrderedCells[V]) R
	begin          func() R
	accumulate     func(R, OrderedCells[V]) (R, bool)
	borrowIssued   bool
	transferResult bool
	result         FrozenResult[R]
	query          *QuerySlot[R]
}

type schemaQueryBindingCell interface {
	complete() bool
	schemaQueryOrdinal() uint64
	schemaQueryState() *schemaBindingState
}

// schemaSummaryQueryBindingCell mirrors the exact cell but retains the
// summary form's canonical normalizer key. It is intentionally a separate
// concrete cell: the query projection kind is part of the sealed receipt
// contract and must not be inferred from a callback.
type schemaSummaryQueryBindingCell[V, R any] struct {
	state          *schemaBindingState
	schema         *Schema
	ordinal        uint64
	factorOrdinal  uint64
	factor         composition.Key
	normalizer     composition.Key
	project        func(OrderedCells[V]) R
	begin          func() R
	accumulate     func(R, OrderedCells[V]) (R, bool)
	borrowIssued   bool
	transferResult bool
	keys           []uint64
	result         FrozenResult[R]
	query          *QuerySlot[R]
}

func (cell *schemaSummaryQueryBindingCell[V, R]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
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

func (cell *schemaSummaryQueryBindingCell[V, R]) complete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || !cell.schema.Available() || cell.state.schema != cell.schema || cell.state.phase != schemaBindingOpen || cell.query == nil || cell.query.Schema() != cell.schema || (cell.project == nil && (cell.begin == nil || cell.accumulate == nil)) || !validFrozenResult(cell.result) || !cell.factor.Available() || !cell.normalizer.Available() || len(cell.keys) == 0 || cell.factorOrdinal >= uint64(len(cell.state.factors)) {
		return false
	}
	shape, ok := cell.schema.queryShapeAt(cell.ordinal)
	if !ok || shape.ProjectionCount != 1 || shape.Freezer != compositionKeyOf(cell.result.Semantic) {
		return false
	}
	projection, ok := cell.schema.queryProjectionShapeAt(cell.ordinal, 0)
	return ok && projection.Kind == composition.QueryFactorSummary && projection.Factor == cell.factor && projection.Normalizer == cell.normalizer && cell.schema.factorSemanticAt(cell.factorOrdinal) == cell.factor
}

type summaryQueryRuntimeBinding[V, R any] struct {
	state         *schemaBindingState
	authority     *schemaBindingAuthority
	cell          *schemaSummaryQueryBindingCell[V, R]
	queryOrdinal  uint64
	factorOrdinal uint64
	factor        composition.Key
	normalizer    composition.Key
	issued        bool
}

func (receipt summaryQueryRuntimeBinding[V, R]) valid() bool {
	if !receipt.issued || receipt.state == nil || receipt.authority == nil || receipt.cell == nil || receipt.state.phase != schemaBindingSealed || receipt.state.authority != receipt.authority || receipt.cell.state != receipt.state || receipt.cell.schema != receipt.state.schema || receipt.cell.ordinal != receipt.queryOrdinal || receipt.cell.factorOrdinal != receipt.factorOrdinal || receipt.cell.factor != receipt.factor || receipt.cell.normalizer != receipt.normalizer || (receipt.cell.project == nil && (receipt.cell.begin == nil || receipt.cell.accumulate == nil)) || !validFrozenResult(receipt.cell.result) || receipt.queryOrdinal >= uint64(len(receipt.state.queries)) || receipt.state.queries[receipt.queryOrdinal] != receipt.cell {
		return false
	}
	shape, ok := receipt.state.schema.queryShapeAt(receipt.queryOrdinal)
	if !ok || shape.ProjectionCount != 1 || shape.Freezer != compositionKeyOf(receipt.cell.result.Semantic) {
		return false
	}
	projection, ok := receipt.state.schema.queryProjectionShapeAt(receipt.queryOrdinal, 0)
	return ok && projection.Kind == composition.QueryFactorSummary && projection.Factor == receipt.factor && projection.Normalizer == receipt.normalizer && receipt.state.schema.factorSemanticAt(receipt.factorOrdinal) == receipt.factor
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

func (cell *schemaExactQueryBindingCell[V, R]) schemaBindingSchema() *Schema {
	if cell == nil {
		return nil
	}
	return cell.schema
}

func (cell *schemaExactQueryBindingCell[V, R]) complete() bool {
	if cell == nil || cell.state == nil || cell.schema == nil || !cell.schema.Available() || cell.state.schema != cell.schema || cell.state.phase != schemaBindingOpen || cell.query == nil || cell.query.Schema() != cell.schema || (cell.project == nil && (cell.begin == nil || cell.accumulate == nil)) || !validFrozenResult(cell.result) || cell.factor == (composition.Key{}) || cell.factorOrdinal >= uint64(len(cell.state.factors)) {
		return false
	}
	shape, ok := cell.schema.queryShapeAt(cell.ordinal)
	if !ok || shape.ProjectionCount != 1 || shape.Freezer != compositionKeyOf(cell.result.Semantic) {
		return false
	}
	projection, ok := cell.schema.queryProjectionShapeAt(cell.ordinal, 0)
	return ok && projection.Kind == composition.QueryFactorExact && projection.Factor == cell.factor && cell.schema.factorSemanticAt(cell.factorOrdinal) == cell.factor
}

// ExactQueryImplementation is an opaque, sealed receipt for one exact
// Factor Query.  It retains only the exact SchemaBinding state, authority,
// owning cell, and canonical ordinals; no cold row, callback copy, or key
// domain is duplicated here.
type ExactQueryImplementation[V, R any] struct {
	binding exactQueryRuntimeBinding[V, R]
}

type exactQueryRuntimeBinding[V, R any] struct {
	state         *schemaBindingState
	authority     *schemaBindingAuthority
	cell          *schemaExactQueryBindingCell[V, R]
	queryOrdinal  uint64
	factorOrdinal uint64
	factor        composition.Key
	issued        bool
}

func (receipt exactQueryRuntimeBinding[V, R]) valid() bool {
	if !receipt.issued || receipt.state == nil || receipt.authority == nil || receipt.cell == nil || receipt.state.phase != schemaBindingSealed || receipt.state.authority != receipt.authority || receipt.cell.state != receipt.state || receipt.cell.schema != receipt.state.schema || receipt.cell.ordinal != receipt.queryOrdinal || receipt.cell.factorOrdinal != receipt.factorOrdinal || receipt.cell.factor != receipt.factor || (receipt.cell.project == nil && (receipt.cell.begin == nil || receipt.cell.accumulate == nil)) || !validFrozenResult(receipt.cell.result) || receipt.queryOrdinal >= uint64(len(receipt.state.queries)) || receipt.state.queries[receipt.queryOrdinal] != receipt.cell {
		return false
	}
	shape, ok := receipt.state.schema.queryShapeAt(receipt.queryOrdinal)
	if !ok || shape.ProjectionCount != 1 || shape.Freezer != compositionKeyOf(receipt.cell.result.Semantic) {
		return false
	}
	projection, ok := receipt.state.schema.queryProjectionShapeAt(receipt.queryOrdinal, 0)
	return ok && projection.Kind == composition.QueryFactorExact && projection.Factor == receipt.factor && receipt.state.schema.factorSemanticAt(receipt.factorOrdinal) == receipt.factor
}

// BindExactQuery installs the one currently supported Query lane.  It is an
// open-phase operation and poisons the binding on any failed declaration;
// support, summary, and multi-projection rows are rejected before a cell is
// published.
func BindExactQuery[V, R any](binding *SchemaBinding, query *QuerySlot[R], factor *FactorSlot[V], spec HotExactQuerySpec[V, R]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	foldConfigured := spec.Fold.Begin != nil || spec.Fold.Accumulate != nil
	if (spec.Project != nil) == foldConfigured || (foldConfigured && !spec.Fold.valid()) || state.phase != schemaBindingOpen || state.schema == nil || query == nil || factor == nil || query.Schema() != state.schema || factor.Schema() != state.schema || !validFrozenResult(spec.Result) {
		state.poisonLocked()
		return false
	}
	queryOrdinal, queryOK := query.Ordinal()
	factorOrdinal, factorOK := factor.Ordinal()
	if !queryOK || !factorOK || queryOrdinal >= uint64(len(state.queries)) || factorOrdinal >= uint64(len(state.factors)) {
		state.poisonLocked()
		return false
	}
	shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	factorKey := state.schema.factorSemanticAt(factorOrdinal)
	if !shapeOK || !projectionOK || shape.ProjectionCount != 1 || projection.Kind != composition.QueryFactorExact || projection.Factor != factorKey || shape.Freezer != compositionKeyOf(spec.Result.Semantic) || !factorKey.Available() || state.queries[queryOrdinal] != nil {
		state.poisonLocked()
		return false
	}
	cell := &schemaExactQueryBindingCell[V, R]{state: state, schema: state.schema, ordinal: queryOrdinal, factorOrdinal: factorOrdinal, factor: factorKey, project: spec.Project, begin: spec.Fold.Begin, accumulate: spec.Fold.Accumulate, borrowIssued: spec.Fold.BorrowIssued, transferResult: spec.Fold.TransferResult, result: spec.Result, query: query}
	if !cell.complete() {
		state.poisonLocked()
		return false
	}
	state.queries[queryOrdinal] = cell
	return true
}

// BindSummaryQuery installs the typed summary Query lane. The form token is
// mandatory because a Factor may expose several summary normalizers; the
// canonical Query projection and this exact form must agree before a cell is
// published.
func BindSummaryQuery[V, R any](binding *SchemaBinding, query *QuerySlot[R], factor *FactorSlot[V], form SchemaReadForm[V], spec HotSummaryQuerySpec[V, R]) bool {
	state := bindingState(binding)
	if state == nil {
		return false
	}
	foldConfigured := spec.Fold.Begin != nil || spec.Fold.Accumulate != nil
	if foldConfigured && !spec.Fold.valid() {
		state.mu.Lock()
		state.poisonLocked()
		state.mu.Unlock()
		return false
	}
	foldSelected := foldConfigured
	if (spec.Project != nil) == foldSelected {
		state.mu.Lock()
		state.poisonLocked()
		state.mu.Unlock()
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingOpen || state.schema == nil || query == nil || factor == nil || query.Schema() != state.schema || factor.Schema() != state.schema || form.Schema() != state.schema || !summaryReadFormKind(form.Kind()) || (spec.Project == nil && !foldSelected) || !validFrozenResult(spec.Result) {
		state.poisonLocked()
		return false
	}
	queryOrdinal, queryOK := query.Ordinal()
	factorOrdinal, factorOK := factor.Ordinal()
	if !queryOK || !factorOK || queryOrdinal >= uint64(len(state.queries)) || factorOrdinal >= uint64(len(state.factors)) || form.cell == nil {
		state.poisonLocked()
		return false
	}
	formFactor := form.cell.ordinal >> 32
	formOrdinal := uint64(uint32(form.cell.ordinal))
	factorKey := state.schema.factorSemanticAt(factorOrdinal)
	shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	formShape, formShapeOK := state.schema.factorFormShapeAt(factorOrdinal, formOrdinal)
	if !shapeOK || !projectionOK || !formShapeOK || shape.ProjectionCount != 1 || projection.Kind != composition.QueryFactorSummary || projection.Factor != factorKey || projection.Normalizer != formShape.Semantic || formFactor != factorOrdinal || !summaryReadRowKind(formShape.Kind) || !projection.Normalizer.Available() || !factorKey.Available() || state.queries[queryOrdinal] != nil {
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
	cell := &schemaSummaryQueryBindingCell[V, R]{state: state, schema: state.schema, ordinal: queryOrdinal, factorOrdinal: factorOrdinal, factor: factorKey, normalizer: formShape.Semantic, project: spec.Project, begin: spec.Fold.Begin, accumulate: spec.Fold.Accumulate, borrowIssued: spec.Fold.BorrowIssued, transferResult: spec.Fold.TransferResult, keys: keys, result: spec.Result, query: query}
	if !cell.complete() {
		state.poisonLocked()
		return false
	}
	state.queries[queryOrdinal] = cell
	return true
}

// ExactQueryImplementationAt issues a fresh opaque receipt after Binding.Seal.
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
	cell, ok := state.queries[ordinal].(*schemaExactQueryBindingCell[V, R])
	if !ok || !cell.completeSealed(state) {
		return nil, false
	}
	receipt := exactQueryRuntimeBinding[V, R]{state: state, authority: state.authority, cell: cell, queryOrdinal: ordinal, factorOrdinal: cell.factorOrdinal, factor: cell.factor, issued: true}
	if !receipt.valid() {
		return nil, false
	}
	return &ExactQueryImplementation[V, R]{binding: receipt}, true
}

// SummaryQueryImplementation is the sealed receipt for one summary Factor
// Query. It carries no cold row or mutable callback registry.
type SummaryQueryImplementation[V, R any] struct {
	binding summaryQueryRuntimeBinding[V, R]
}

// boundTopologySummarySurfaceReceipt exposes only the sealed Factor/form
// authority needed to admit a graph summary mapping. The query carries no
// caller-owned coordinate vector. The Factor owner issues its closed key plane
// once during binding; topologySummaryMapping only borrows that sealed plane.
func (implementation *SummaryQueryImplementation[V, R]) boundTopologySummarySurfaceReceipt() (*schemaBindingState, *schemaBindingAuthority, composition.Key, composition.Key, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return nil, nil, composition.Key{}, composition.Key{}, false
	}
	return implementation.binding.state, implementation.binding.authority, implementation.binding.factor, implementation.binding.normalizer, true
}

func (implementation *SummaryQueryImplementation[V, R]) topologySummaryMapping(surface equation.Surface) (equation.SummaryMapping, bool) {
	if implementation == nil || !implementation.binding.valid() || !surface.Available() || surface.Factor != implementation.binding.factor || surface.Form != equation.SurfaceReadSummary || surface.Semantic != implementation.binding.normalizer || surface.Normalizer != implementation.binding.normalizer || surface.Mode != equation.TargetModeNone {
		return equation.SummaryMapping{}, false
	}
	if len(implementation.binding.cell.keys) == 0 {
		return equation.SummaryMapping{}, false
	}
	return equation.SummaryMapping{Surface: surface, Keys: implementation.binding.cell.keys}, true
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
	cell, ok := state.queries[ordinal].(*schemaSummaryQueryBindingCell[V, R])
	if !ok || cell == nil || cell.state != state || cell.schema != state.schema || cell.query != slot || (cell.project == nil && (cell.begin == nil || cell.accumulate == nil)) || !validFrozenResult(cell.result) {
		return nil, false
	}
	receipt := summaryQueryRuntimeBinding[V, R]{state: state, authority: state.authority, cell: cell, queryOrdinal: ordinal, factorOrdinal: cell.factorOrdinal, factor: cell.factor, normalizer: cell.normalizer, issued: true}
	if !receipt.valid() {
		return nil, false
	}
	return &SummaryQueryImplementation[V, R]{binding: receipt}, true
}

func (implementation *SummaryQueryImplementation[V, R]) projector() (func(OrderedCells[V]) R, bool) {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell.project == nil {
		return nil, false
	}
	return implementation.binding.cell.project, true
}

func (implementation *SummaryQueryImplementation[V, R]) accumulator() (func() R, func(R, OrderedCells[V]) (R, bool), bool, bool, bool) {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell.begin == nil || implementation.binding.cell.accumulate == nil {
		return nil, nil, false, false, false
	}
	return implementation.binding.cell.begin, implementation.binding.cell.accumulate, implementation.binding.cell.borrowIssued, implementation.binding.cell.transferResult, true
}

// frozenResult returns the result contract behind a valid receipt: the
// identity this family's answers are frozen under, and the freeze, clone,
// equality and fingerprint the owning domain declared for them. A materializer
// writes the frozen form and checks it against that contract, so what reaches a
// published column is what the domain says an answer is.
func (implementation *SummaryQueryImplementation[V, R]) frozenResult() (FrozenResult[R], bool) {
	if implementation == nil || !implementation.binding.valid() {
		return FrozenResult[R]{}, false
	}
	return implementation.binding.cell.result, true
}

func (implementation *ExactQueryImplementation[V, R]) frozenResult() (FrozenResult[R], bool) {
	if implementation == nil || !implementation.binding.valid() {
		return FrozenResult[R]{}, false
	}
	return implementation.binding.cell.result, true
}

func (cell *schemaExactQueryBindingCell[V, R]) completeSealed(state *schemaBindingState) bool {
	return cell != nil && state != nil && cell.state == state && state.phase == schemaBindingSealed && cell.schema == state.schema && cell.query != nil && cell.query.Schema() == state.schema && (cell.project != nil || cell.begin != nil && cell.accumulate != nil) && validFrozenResult(cell.result)
}

// projector returns the typed hot callback behind a valid receipt. It is
// compiler-owned; callers receive no mutable binding cell or cold query row.
func (implementation *ExactQueryImplementation[V, R]) projector() (func(OrderedCells[V]) R, bool) {
	if implementation == nil || !implementation.binding.valid() {
		return nil, false
	}
	if implementation.binding.cell.project == nil {
		return nil, false
	}
	return implementation.binding.cell.project, true
}

func (implementation *ExactQueryImplementation[V, R]) accumulator() (func() R, func(R, OrderedCells[V]) (R, bool), bool, bool, bool) {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell.begin == nil || implementation.binding.cell.accumulate == nil {
		return nil, nil, false, false, false
	}
	return implementation.binding.cell.begin, implementation.binding.cell.accumulate, implementation.binding.cell.borrowIssued, implementation.binding.cell.transferResult, true
}

// receiptExactQueryRuntime is the compiler-side query evidence. It is built
// only from an issued Query receipt, the graph-owned equation Query, and the
// already-bound Factor runtime. In particular it never consults a declaration schema or
// reconstructs a declaration row.
type receiptExactQueryRuntime[V, R any] struct {
	identity equation.Query
	receipt  exactQueryRuntimeBinding[V, R]
	factor   receiptQueryFactor[V]
	surface  equation.Surface
	unit     carrier.Unit
}

func (runtime *receiptExactQueryRuntime[V, R]) query() equation.Query {
	if runtime == nil {
		return equation.Query{}
	}
	return runtime.identity
}

type receiptSummaryQueryRuntime[V, R any] struct {
	identity equation.Query
	receipt  summaryQueryRuntimeBinding[V, R]
	factor   receiptQueryFactor[V]
	surface  equation.Surface
	unit     carrier.Unit
}

func (runtime *receiptSummaryQueryRuntime[V, R]) query() equation.Query {
	if runtime == nil {
		return equation.Query{}
	}
	return runtime.identity
}

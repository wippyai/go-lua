package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/product"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

type frozenValue interface {
	clone() frozenValue
	equal(frozenValue) bool
	fingerprint() uint64
}

type typedFrozenValue[R any] struct {
	value  R
	freeze FrozenResult[R]
}

func (value *typedFrozenValue[R]) clone() frozenValue {
	if value == nil {
		return nil
	}
	return &typedFrozenValue[R]{value: value.freeze.Clone(value.value), freeze: value.freeze}
}
func (value *typedFrozenValue[R]) equal(other frozenValue) bool {
	right, ok := other.(*typedFrozenValue[R])
	return ok && value != nil && right != nil && value.freeze.Equal(value.value, right.value)
}
func (value *typedFrozenValue[R]) fingerprint() uint64 {
	if value == nil {
		return 0
	}
	return value.freeze.Fingerprint(value.value)
}

type queryResult struct {
	owner *queryAuthority
	key   composition.Key
	value frozenValue
}

func (result *queryResult) clone() *queryResult {
	if result == nil || result.owner == nil || result.value == nil {
		return nil
	}
	return &queryResult{owner: result.owner, key: result.key, value: result.value.clone()}
}

type runtimeQuery interface {
	query() equation.Query
	queryAuthority() *queryAuthority
	materialize(*carrier.Work, carrier.State) (*queryResult, bool)
}

// queryReadRuntime is a closed, type-erased execution closure built from a
// typed QueryRead token. It never exposes a payload or a carrier key: only the
// retained typed closure can decode its own session at QueryValue time.
type queryReadRuntime interface {
	ordinal() int
	form() *formSchema
	surface() equation.Surface
	refine(*queryExecution, int) bool
}

type queryReadSession interface{ close() }

type queryProductRow = provenanceRow

type queryExecution struct {
	schema   *querySchema
	work     *carrier.Work
	state    carrier.State
	reads    []queryReadRuntime
	sessions []queryReadSession
	rows     product.Rows
	values   []queryProductRow
	columns  [][]uint64
	epoch    uint64
	active   atomic.Uint64
	started  atomic.Bool
	failed   atomic.Bool
	ready    bool
}

func (execution *queryExecution) checkpoint() bool {
	return execution != nil && execution.work != nil && execution.epoch != 0 && execution.active.Load() == execution.epoch && execution.work.Checkpoint()
}

func (execution *queryExecution) requireCheckpoint() bool {
	if !execution.checkpoint() {
		if execution != nil {
			execution.failed.Store(true)
		}
		return false
	}
	return true
}

func (execution *queryExecution) close() {
	if execution == nil {
		return
	}
	for index := range execution.values {
		execution.values[index].prefix = nil
	}
	execution.values = nil
	for index := range execution.columns {
		execution.columns[index] = nil
	}
	execution.columns = nil
	execution.rows = product.Rows{}
	for index := range execution.sessions {
		if execution.sessions[index] != nil {
			execution.sessions[index].close()
		}
	}
	execution.sessions = nil
	execution.reads = nil
	execution.ready = false
}

func newQueryExecution(work *carrier.Work, state carrier.State, schema *querySchema, reads []queryReadRuntime, epoch uint64) (*queryExecution, bool) {
	if work == nil || !work.Checkpoint() || !work.OwnsState(state) || schema == nil || schema.support || len(reads) == 0 || epoch == 0 || len(reads) != len(schema.reads) {
		return nil, false
	}
	execution := &queryExecution{
		schema: schema, work: work, state: state, reads: append([]queryReadRuntime(nil), reads...), sessions: make([]queryReadSession, len(reads)), epoch: epoch,
	}
	execution.active.Store(epoch)
	if support.Empty(state.Support()) {
		execution.ready = true
		return execution, true
	}
	rows, ok := product.NewRows(state.Support())
	if !ok {
		execution.active.CompareAndSwap(epoch, 0)
		return nil, false
	}
	execution.rows, execution.values = rows, []queryProductRow{{}}
	for index, read := range execution.reads {
		if !execution.requireCheckpoint() || read == nil || read.form() != schema.reads[index].form || !read.refine(execution, index) || execution.rows.Count() != len(execution.values) {
			execution.close()
			execution.active.CompareAndSwap(epoch, 0)
			return nil, false
		}
	}
	if !execution.requireCheckpoint() || !execution.freezeProvenance() || execution.rows.Count() == 0 {
		execution.close()
		execution.active.CompareAndSwap(epoch, 0)
		return nil, false
	}
	execution.ready = true
	return execution, true
}

func (execution *queryExecution) readID(row, read int) (uint64, bool) {
	if execution == nil {
		return 0, false
	}
	return provenanceID(execution.values, execution.columns, row, read, len(execution.reads))
}

func (execution *queryExecution) freezeProvenance() bool {
	if execution == nil || len(execution.values) != execution.rows.Count() {
		return false
	}
	columns, ok := freezeProvenanceColumns(execution.checkpoint, execution.values, len(execution.reads))
	if !ok {
		return false
	}
	execution.columns = columns
	return true
}

type typedQueryReadRuntime[K ~uint32 | ~uint64, V, S any] struct {
	index       int
	declared    *formSchema
	resolved    equation.Surface
	binding     *factbinding.Binding[K, V]
	unit        carrier.Unit
	normalize   func(OrderedCells[V]) S
	equal       func(S, S) bool
	fingerprint func(S) uint64
}

type typedQueryReadSession[V, S any] struct {
	values  []S
	records []*orderedCellsRecord[V]
}

func (runtime *typedQueryReadRuntime[K, V, S]) ordinal() int      { return runtime.index }
func (runtime *typedQueryReadRuntime[K, V, S]) form() *formSchema { return runtime.declared }
func (runtime *typedQueryReadRuntime[K, V, S]) surface() equation.Surface {
	return runtime.resolved
}

func (runtime *typedQueryReadRuntime[K, V, S]) refine(execution *queryExecution, index int) bool {
	if runtime == nil || runtime.binding == nil {
		return false
	}
	return materializeTypedQueryRead(execution, index, runtime.unit, runtime.binding.ResolveObservation, runtime.normalize, runtime.equal, runtime.fingerprint)
}

func materializeTypedQueryRead[V, S any](execution *queryExecution, index int, unit carrier.Unit, resolve func(carrier.SlotWork, carrier.ObservationRow) (factbinding.Observation[V], bool), normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) bool {
	if execution == nil || execution.work == nil || index < 0 || index >= len(execution.reads) || index >= len(execution.sessions) || execution.sessions[index] != nil || unit == (carrier.Unit{}) || resolve == nil || normalize == nil || equal == nil || fingerprint == nil || execution.rows.Count() != len(execution.values) {
		return false
	}
	slot, ok := unit.Slot()
	if !ok {
		return false
	}
	work, ok := execution.work.SlotWork(slot)
	if !ok || !work.BeginObservation() {
		return false
	}
	defer work.EndObservation()
	root, ok := execution.state.HandleAt(slot)
	if !ok {
		return false
	}
	values := &typedQueryReadSession[V, S]{}
	refinement := execution.rows.BeginRefineWithCheckpoint(execution.checkpoint)
	if refinement == nil {
		return false
	}
	for source := 0; source < execution.rows.Count(); source++ {
		if !execution.requireCheckpoint() {
			values.close()
			return false
		}
		within, ok := execution.rows.At(source)
		if !ok || !work.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
			if !execution.requireCheckpoint() {
				return false
			}
			observation, ok := resolve(work, row)
			if !ok || !observation.Valid() {
				return false
			}
			cells := make([]summaryCell[V], observation.Count())
			for cellIndex := range cells {
				if !execution.requireCheckpoint() {
					return false
				}
				entry, ok := observation.At(cellIndex)
				if !ok {
					return false
				}
				value, present := entry.Read()
				cells[cellIndex] = summaryCell[V]{value: value, present: present}
			}
			record := newOrderedCellsRecord(cells)
			values.records = append(values.records, record)
			values.values = append(values.values, normalize(OrderedCells[V]{record: record}))
			return refinement.Add(source, row.Region())
		}) {
			values.close()
			return false
		}
	}
	nextRows, sources, ok := refinement.Seal()
	if !ok || sources.SourceCount() != len(execution.values) || sources.Count() != len(values.values) || len(values.records) != len(values.values) {
		values.close()
		return false
	}
	// One output per source has no within-source pair to quotient. Preserve the
	// exact sealed rows and extend the existing provenance prefixes in place.
	if sources.Count() == sources.SourceCount() {
		for source := range execution.values {
			start, end, mapped := sources.Range(source)
			if !execution.requireCheckpoint() || !mapped || start != source || end != source+1 {
				values.close()
				return false
			}
		}
		for source := range execution.values {
			execution.values[source] = extendProvenance(execution.values[source], index, uint64(source+1))
		}
		execution.rows, execution.sessions[index] = nextRows, values
		return true
	}
	next := make([]queryProductRow, sources.Count())
	for source := 0; source < len(execution.values); source++ {
		if !execution.requireCheckpoint() {
			values.close()
			return false
		}
		start, end, ok := sources.Range(source)
		if !ok || end <= start {
			values.close()
			return false
		}
		for output := start; output < end; output++ {
			if !execution.requireCheckpoint() {
				values.close()
				return false
			}
			next[output] = extendProvenance(execution.values[source], index, uint64(output+1))
		}
	}
	rows, representatives, ok := nextRows.PrefixQuotientWithCheckpoint(sources, execution.requireCheckpoint, func(value int) (uint64, bool) {
		if !execution.requireCheckpoint() || value < 0 || value >= len(values.values) {
			return 0, false
		}
		return fingerprint(values.values[value]), execution.requireCheckpoint()
	}, func(left, right int) bool {
		if !execution.requireCheckpoint() || left < 0 || right < 0 || left >= len(values.values) || right >= len(values.values) || left >= len(values.records) || right >= len(values.records) {
			return false
		}
		matched := equal(values.values[left], values.values[right])
		return matched && execution.requireCheckpoint()
	})
	if !ok || len(representatives) == 0 {
		values.close()
		return false
	}
	compactValues := make([]S, len(representatives))
	compactRecords := make([]*orderedCellsRecord[V], len(representatives))
	compactRows := make([]queryProductRow, len(representatives))
	retained := make([]bool, len(values.records))
	for compactID, representative := range representatives {
		if !execution.requireCheckpoint() || representative < 0 || representative >= len(next) || representative >= len(values.values) || representative >= len(values.records) || values.records[representative] == nil {
			values.close()
			return false
		}
		prefix := next[representative].prefix
		if prefix == nil || prefix.read != index || prefix.id != uint64(representative+1) {
			values.close()
			return false
		}
		compactValues[compactID] = values.values[representative]
		compactRecords[compactID] = values.records[representative]
		compactRows[compactID] = extendProvenance(queryProductRow{prefix: prefix.previous}, index, uint64(compactID+1))
		retained[representative] = true
	}
	for _, record := range values.records {
		if record == nil {
			values.close()
			return false
		}
	}
	if !execution.requireCheckpoint() {
		values.close()
		return false
	}
	// Commit only after all compacted rows and identities are complete.  The
	// discarded records are revoked here; retained records move to the compact
	// session and the first representative preserves the preceding prefix.
	for recordID, record := range values.records {
		if !retained[recordID] {
			record.revoke()
		}
	}
	values.values, values.records = nil, nil
	execution.rows, execution.values = rows, compactRows
	execution.sessions[index] = &typedQueryReadSession[V, S]{values: compactValues, records: compactRecords}
	return true
}

func resolveTypedQueryValue[V, S any](execution *queryExecution, row, index int) (S, bool) {
	var zero S
	if execution == nil || row < 0 || row >= len(execution.values) || index < 0 || index >= len(execution.sessions) {
		return zero, false
	}
	id, found := execution.readID(row, index)
	if !found {
		return zero, false
	}
	values, ok := execution.sessions[index].(*typedQueryReadSession[V, S])
	if !ok || id > uint64(len(values.values)) {
		return zero, false
	}
	return values.values[id-1], true
}

func (session *typedQueryReadSession[V, S]) close() {
	if session == nil {
		return
	}
	var zero S
	for index := range session.values {
		session.values[index] = zero
	}
	session.values = nil
	for _, record := range session.records {
		if record != nil {
			record.revoke()
		}
	}
	session.records = nil
}

type supportQueryExecution struct {
	work      *carrier.Work
	epoch     uint64
	reachable bool
	active    atomic.Uint64
}

func (execution *supportQueryExecution) checkpoint() bool {
	return execution != nil && execution.work != nil && execution.epoch != 0 && execution.active.Load() == execution.epoch && execution.work.Checkpoint()
}

type boundQuery[R any] struct {
	identity  equation.Query
	authority *queryAuthority
	queryCap  *Query[R]
	reads     []queryReadRuntime
	nextEpoch atomic.Uint64
}

func (bound *boundQuery[R]) query() equation.Query { return bound.identity }
func (bound *boundQuery[R]) queryAuthority() *queryAuthority {
	if bound == nil {
		return nil
	}
	return bound.authority
}

// bindQueryRead binds one typed token to its same-position equation surface.
// It is the only E-side route that joins a cold query read with a concrete
// Factor unit; callers cannot substitute a token from another Query or a
// selector surface.
func bindQueryRead[K ~uint32 | ~uint64, V, S any](schema *querySchema, read QueryRead[S], factor *boundFactor[K, V], surface equation.Surface, normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) (queryReadRuntime, bool) {
	if schema == nil || schema.support || read.schema != schema || read.index < 0 || read.index >= len(schema.reads) || read.resolve == nil || factor == nil || normalize == nil || equal == nil || fingerprint == nil {
		return nil, false
	}
	declared := schema.reads[read.index].form
	if declared == nil || declared.factor != factor.factor.schema || (declared.readKind != exactReadForm && declared.readKind != summaryReadForm) {
		return nil, false
	}
	if declared.readKind == exactReadForm && surface.Form != equation.SurfaceReadExact || declared.readKind == summaryReadForm && !matchesFactorReadForm(factor.factor.schema, surface, summaryReadForm) {
		return nil, false
	}
	unit, ok := factor.readUnit(surface)
	if !ok {
		return nil, false
	}
	return &typedQueryReadRuntime[K, V, S]{index: read.index, declared: declared, resolved: surface, binding: factor.binding, unit: unit, normalize: normalize, equal: equal, fingerprint: fingerprint}, true
}

// bindQuery turns one equation-issued ordered surface vector into a normal
// runtime query. There is no singular exact-query fast path.
func bindQuery[R any](identity equation.Query, query *Query[R], reads []queryReadRuntime) (*boundQuery[R], bool) {
	authority := sealedQueryAuthority(query)
	if !identity.Key().Available() || authority == nil || query.schema.support || query.project == nil || !validFrozenResult(query.result) || query.schema.semantic.compositionKey() != identity.Family() || len(reads) == 0 || len(reads) != len(query.schema.reads) || len(identity.Surfaces()) != len(reads) {
		return nil, false
	}
	surfaces := identity.Surfaces()
	for index, read := range reads {
		if read == nil || read.ordinal() != index || read.form() != query.schema.reads[index].form || read.surface() != surfaces[index] {
			return nil, false
		}
	}
	return &boundQuery[R]{identity: identity, authority: authority, queryCap: query, reads: append([]queryReadRuntime(nil), reads...)}, true
}

func (bound *boundQuery[R]) materialize(work *carrier.Work, state carrier.State) (*queryResult, bool) {
	if bound == nil || work == nil || !work.Checkpoint() || !work.OwnsState(state) || bound.queryCap == nil || bound.queryCap.schema == nil || bound.authority == nil || sealedQueryAuthority(bound.queryCap) != bound.authority {
		return nil, false
	}
	epoch := bound.nextEpoch.Add(1)
	if epoch == 0 {
		return nil, false
	}
	execution, ok := newQueryExecution(work, state, bound.queryCap.schema, bound.reads, epoch)
	if !ok {
		return nil, false
	}
	var result R
	failed := false
	func() {
		defer func() {
			failed = execution.failed.Load() || !execution.checkpoint()
			execution.active.CompareAndSwap(epoch, 0)
			execution.close()
		}()
		result = bound.queryCap.project(Observation{execution: execution, epoch: epoch})
	}()
	if failed || !work.Checkpoint() {
		return nil, false
	}
	frozen := bound.queryCap.result.Freeze(result)
	if !work.Checkpoint() {
		return nil, false
	}
	return &queryResult{owner: bound.authority, key: bound.identity.Key(), value: &typedFrozenValue[R]{value: frozen, freeze: bound.queryCap.result}}, true
}

type boundSupportQuery[R any] struct {
	identity  equation.Query
	authority *queryAuthority
	queryCap  *Query[R]
	nextEpoch atomic.Uint64
}

func (bound *boundSupportQuery[R]) query() equation.Query { return bound.identity }
func (bound *boundSupportQuery[R]) queryAuthority() *queryAuthority {
	if bound == nil {
		return nil
	}
	return bound.authority
}

func bindSupportQuery[R any](identity equation.Query, query *Query[R]) (*boundSupportQuery[R], bool) {
	authority := sealedQueryAuthority(query)
	if !identity.Key().Available() || authority == nil || len(identity.Surfaces()) != 0 || query.schema.support == false || query.supportProject == nil || !validFrozenResult(query.result) || query.schema.semantic.compositionKey() != identity.Family() {
		return nil, false
	}
	return &boundSupportQuery[R]{identity: identity, authority: authority, queryCap: query}, true
}

func (bound *boundSupportQuery[R]) materialize(work *carrier.Work, state carrier.State) (*queryResult, bool) {
	if bound == nil || work == nil || !work.Checkpoint() || !work.OwnsState(state) || bound.queryCap == nil || bound.authority == nil || sealedQueryAuthority(bound.queryCap) != bound.authority {
		return nil, false
	}
	epoch := bound.nextEpoch.Add(1)
	if epoch == 0 {
		return nil, false
	}
	execution := &supportQueryExecution{work: work, epoch: epoch, reachable: !support.Empty(state.Support())}
	execution.active.Store(epoch)
	var value R
	func() {
		defer execution.active.CompareAndSwap(epoch, 0)
		value = bound.queryCap.supportProject(SupportObservation{execution: execution, epoch: epoch})
	}()
	if !work.Checkpoint() {
		return nil, false
	}
	frozen := bound.queryCap.result.Freeze(value)
	if !work.Checkpoint() {
		return nil, false
	}
	return &queryResult{owner: bound.authority, key: bound.identity.Key(), value: &typedFrozenValue[R]{value: frozen, freeze: bound.queryCap.result}}, true
}

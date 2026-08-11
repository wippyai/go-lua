package engine

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/product"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// Row is an opaque, synchronous product row. Its support and typed values
// remain inside the private product session; it expires when Transfer returns.
type Row struct {
	session *productSession
	epoch   uint64
	index   int
}

type readRuntime interface {
	inputPort() int
	refine(*productSession, int) bool
	observations() []demand.Observation
	dynamicReads() []demand.DynamicRead
	exactProof() ruleReadProof
	summaryProof() ruleSummaryReadProof
}

type productRow = provenanceRow

type productSession struct {
	execution *ruleExecution
	work      *carrier.Work
	inputs    []carrier.State
	reads     []readRuntime
	sessions  []readSession
	rows      product.Rows
	values    []productRow
	columns   [][]uint64
	live      bool
	started   atomic.Bool
	ready     bool
	// current is the one Product row whose callback owns the synchronous row
	// capability. It is -1 outside that callback; a Row never grants access to
	// a prior or future region while a transfer is still running.
	current int
}

// readSession is payload-free at the generic executor boundary. Concrete
// typed stores are reached only by the declaration-time Read resolver. A
// staged session additionally exposes its epoch-local exact selected routes;
// static declaration reads remain on readRuntime.observations.
type readSession interface {
	close()
	dynamicObservations() []demand.Observation
}

func (session *productSession) valid(execution *ruleExecution, epoch uint64) bool {
	return session != nil && session.live && session.execution == execution && execution != nil && epoch != 0 && execution.active.Load() == epoch && session.work != nil && session.work.Checkpoint()
}

// checkpoint samples the one carrier-owned epoch probe. Product owns no
// second cancellation authority; it merely stops its private rows before a
// Rule callback, patch, or evidence can escape.
func (session *productSession) checkpoint() bool {
	return session != nil && session.execution != nil && session.work != nil && session.execution.active.Load() == session.execution.epoch && session.work.Checkpoint()
}

func (session *productSession) requireCheckpoint() bool {
	if !session.checkpoint() {
		if session != nil && session.execution != nil {
			session.execution.failed.Store(true)
		}
		return false
	}
	return true
}

func (session *productSession) close() {
	if session == nil {
		return
	}
	session.rows = product.Rows{}
	session.values = nil
	for index := range session.columns {
		session.columns[index] = nil
	}
	session.columns = nil
	for index := range session.sessions {
		if session.sessions[index] != nil {
			session.sessions[index].close()
		}
	}
	session.sessions = nil
	session.reads = nil
	session.inputs = nil
	session.ready = false
	session.current = -1
	session.live = false
}

func newProductSession(execution *ruleExecution, reads []readRuntime, work *carrier.Work, inputs []carrier.State, within support.Mask) (*productSession, bool) {
	if execution == nil || work == nil || !work.Checkpoint() || !work.OwnsContributionStates(execution.base, inputs) || !within.Valid() {
		return nil, false
	}
	// BeginContribution computed the exact input intersection under the sealed
	// group premise and retained that immutable handle in the shared
	// predecessor. Product members only consume the established region;
	// rebuilding the identical meet per member is unnecessary.
	if !execution.base.State().Support().SameHandle(within) {
		return nil, false
	}
	session := &productSession{execution: execution, work: work, inputs: append([]carrier.State(nil), inputs...), reads: append([]readRuntime(nil), reads...), sessions: make([]readSession, len(reads)), live: true, current: -1}
	if support.Empty(within) {
		// An unreachable group is a valid zero-row product. It retains no
		// observation frame and cannot stage a patch, but still lets the
		// monomorphic Transfer confirm its own no-row behavior.
		session.ready = true
		return session, true
	}
	rows, ok := product.NewRows(within)
	if !ok {
		session.close()
		return nil, false
	}
	session.rows, session.values = rows, []productRow{{}}
	for index, read := range reads {
		if !session.requireCheckpoint() || read == nil || read.inputPort() < 0 || read.inputPort() >= len(inputs) || !read.refine(session, index) || session.rows.Count() != len(session.values) {
			session.close()
			return nil, false
		}
	}
	// Every typed refinement has already compacted its own output against its
	// source prefix. Freeze the completed tuple once so hot ReadValue lookups
	// use read-major provenance columns rather than walking that prefix.
	if !session.requireCheckpoint() || !session.freezeProvenance() {
		session.close()
		return nil, false
	}
	if session.rows.Count() == 0 {
		session.close()
		return nil, false
	}
	session.ready = true
	return session, true
}

// Product invokes a bound Rule's declared product once. There is no ambient
// State path: a stale Access, duplicate Product, or escaped Row fails closed.
func Product[V, O any](access Access[V, O], visit func(Row) bool) bool {
	session := access.execution
	if session == nil || access.owner == nil || session.owner != access.owner || access.epoch == 0 || session.active.Load() != access.epoch || session.product == nil || visit == nil {
		return false
	}
	product := session.product
	if !product.valid(session, access.epoch) || !product.ready || !product.started.CompareAndSwap(false, true) {
		session.failed.Store(true)
		return false
	}
	for index := 0; index < product.rows.Count(); index++ {
		product.current = index
		visited := visit(Row{session: product, epoch: access.epoch, index: index})
		settled := session.output == nil || session.output.settled(index)
		product.current = -1
		if !product.requireCheckpoint() || !visited || !settled || !product.requireCheckpoint() {
			session.failed.Store(true)
			return false
		}
	}
	return true
}

// ReadValue reaches only the type witness installed at cold declaration. The
// matching E runtime is checked first, so a typed Read from another rule or a
// stale row cannot be used as an erased observation channel.
func ReadValue[V, O, S any](access Access[V, O], row Row, read Read[S]) (S, bool) {
	var zero S
	execution := access.execution
	if execution == nil || execution.owner == nil || access.owner != execution.owner || execution.active.Load() != access.epoch || execution.product == nil || !execution.product.requireCheckpoint() || read.rule == nil || read.rule != execution.owner.ruleSchema() || read.index < 0 || read.resolve == nil || row.session == nil || row.session != execution.product || row.epoch != access.epoch || row.index != execution.product.current || row.index < 0 || row.index >= len(row.session.values) || read.index >= len(row.session.reads) || row.session.reads[read.index] == nil {
		if execution != nil {
			execution.failed.Store(true)
		}
		return zero, false
	}
	id, found := row.session.readID(row.index, read.index)
	if !found {
		execution.failed.Store(true)
		return zero, false
	}
	value, ok := read.resolve(row.session, read.index, id)
	if !ok || !row.session.requireCheckpoint() {
		execution.failed.Store(true)
		return zero, false
	}
	return value, true
}

type typedReadRuntime[K ~uint32 | ~uint64, V, S any] struct {
	input       int
	binding     *factbinding.Binding[K, V]
	unit        carrier.Unit
	proof       ruleReadProof
	summary     ruleSummaryReadProof
	normalize   func(OrderedCells[V]) S
	equal       func(S, S) bool
	fingerprint func(S) uint64
}

type typedReadSession[V, S any] struct {
	values  []S
	records []*orderedCellsRecord[V]
}

// stagedFactor is the narrow typed bridge from a row-local staged read to its
// target Factor. It deliberately exposes neither K nor a carrier root: the
// owner resolves a sealed Ref, observes its own typed Unit, and reports only
// a typed observation plus its exact support piece.
type stagedFactor[V any] interface {
	stagedUnit(exactRef) (carrier.Unit, bool)
	stagedObserve(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[V], support.Mask) bool) bool
	stagedSlot() (shape.Slot, bool)
}

// stagedReadRuntime is one dynamic exact-read node. It has no candidate
// vector: the locator sees only declared predecessor observations and emits
// owner-issued Refs through SelectRoute. Physical Unit work is deduplicated
// per row while canonical numeric tags retain every semantic route.
type stagedReadRuntime[V, S any, Tag selectionTag] struct {
	input     int
	selector  *coldReadSelector
	target    stagedFactor[V]
	locate    func(SelectorContext) bool
	normalize func(OrderedCells[V]) S
}

type stagedRoute[Tag selectionTag] struct {
	ref  exactRef
	unit carrier.Unit
	tag  Tag
}

// stagedRouteSink is installed only while one locator is running. The typed
// emission assertion makes a mismatched tag type fail closed. Duplicate
// (Unit,Tag) routes are rejected during canonicalization, after every locator
// emission is collected but before any Product state is retained.
type stagedRouteSink[V any, Tag selectionTag] struct {
	target stagedFactor[V]
	routes []stagedRoute[Tag]
}

func (sink *stagedRouteSink[V, Tag]) accept(emission selectorEmission) bool {
	if sink == nil || sink.target == nil {
		return false
	}
	route, ok := emission.(emittedRoute[Tag])
	if !ok || route.ref == nil {
		return false
	}
	unit, resolved := sink.target.stagedUnit(route.ref)
	if !resolved || unit == (carrier.Unit{}) {
		return false
	}
	sink.routes = append(sink.routes, stagedRoute[Tag]{ref: route.ref, unit: unit, tag: route.tag})
	return true
}

type stagedSelectionValue[Tag selectionTag, S any] struct {
	ref   exactRef
	tag   Tag
	value S
}

// typedStagedSelectionSession owns the only retained route payload for one
// Product execution. Routes are semantic and canonically ordered by exact
// Unit then numeric tag; `dynamic` is only the physical dependency union for
// demand replacement.
type typedStagedSelectionSession[V, S any, Tag selectionTag] struct {
	values       [][]stagedSelectionValue[Tag, S]
	records      []*orderedCellsRecord[V]
	dynamic      []demand.Observation
	dynamicSeen  map[stagedDemandKey]struct{}
	routeScratch []stagedRoute[Tag]
	routeUnits   []carrier.Unit
	routeIndices []int
	frame        selectorFrame
}

type stagedPartial[S any] struct {
	mask   support.Mask
	values []S
}

type stagedDemandKey struct {
	input int
	unit  carrier.Unit
}

func (runtime *stagedReadRuntime[V, S, Tag]) inputPort() int {
	if runtime == nil {
		return -1
	}
	return runtime.input
}

func (*stagedReadRuntime[V, S, Tag]) observations() []demand.Observation { return nil }

func (runtime *stagedReadRuntime[V, S, Tag]) dynamicReads() []demand.DynamicRead {
	if runtime == nil || runtime.input < 0 || runtime.target == nil {
		return nil
	}
	slot, ok := runtime.target.stagedSlot()
	if !ok {
		return nil
	}
	return []demand.DynamicRead{{Input: uint64(runtime.input), Slot: slot}}
}

func (*stagedReadRuntime[V, S, Tag]) exactProof() ruleReadProof { return ruleReadProof{} }

func (*stagedReadRuntime[V, S, Tag]) summaryProof() ruleSummaryReadProof {
	return ruleSummaryReadProof{}
}

func (runtime *stagedReadRuntime[V, S, Tag]) refine(session *productSession, index int) bool {
	if runtime == nil || session == nil || session.execution == nil || session.work == nil || index < 0 || index >= len(session.reads) ||
		runtime.input < 0 || runtime.input >= len(session.inputs) || runtime.selector == nil || runtime.selector.read != index ||
		runtime.target == nil || runtime.locate == nil || runtime.normalize == nil || len(runtime.selector.depends) == 0 ||
		len(session.values) != session.rows.Count() || index >= len(session.sessions) || session.sessions[index] != nil {
		return false
	}
	for _, dependency := range runtime.selector.depends {
		if !session.requireCheckpoint() || dependency.kind != readDependency || dependency.index < 0 || dependency.index >= index || dependency.index >= len(session.reads) || session.reads[dependency.index] == nil {
			return false
		}
	}
	selected := &typedStagedSelectionSession[V, S, Tag]{
		values: make([][]stagedSelectionValue[Tag, S], 0, len(session.values)),
		frame:  selectorFrame{execution: session.execution, epoch: session.execution.epoch, read: runtime.selector, product: session, row: -1, current: -1},
	}
	refinement := session.rows.BeginRefineWithCheckpoint(session.checkpoint)
	if refinement == nil {
		selected.close()
		return false
	}
	for source := 0; source < session.rows.Count(); source++ {
		if !session.requireCheckpoint() {
			selected.close()
			return false
		}
		within, ok := session.rows.At(source)
		if !ok || support.Empty(within) {
			selected.close()
			return false
		}
		sink := stagedRouteSink[V, Tag]{target: runtime.target, routes: selected.routeScratch[:0]}
		frame := &selected.frame
		frame.row, frame.current, frame.routes = source, -1, &sink
		located := runReadSelector(frame, runtime.locate)
		frame.routes = nil
		selected.routeScratch = sink.routes
		if !located || session.execution.failed.Load() || !session.requireCheckpoint() {
			selected.close()
			return false
		}
		if len(sink.routes) == 0 {
			if !refinement.Add(source, within) {
				selected.close()
				return false
			}
			selected.values = append(selected.values, nil)
			continue
		}
		units, routeUnits, indexed := selected.indexRoutes(sink.routes)
		if !indexed || len(units) == 0 || len(routeUnits) != len(sink.routes) {
			selected.close()
			return false
		}
		for _, unit := range units {
			selected.addDynamic(runtime.input, unit)
		}
		partials, refined := runtime.refineStagedSource(session, within, units, selected)
		if !refined || len(partials) == 0 {
			selected.close()
			return false
		}
		for _, partial := range partials {
			if !session.requireCheckpoint() || !refinement.Add(source, partial.mask) || len(partial.values) != len(units) {
				selected.close()
				return false
			}
			values := make([]stagedSelectionValue[Tag, S], len(sink.routes))
			for routeIndex, route := range sink.routes {
				unitIndex := routeUnits[routeIndex]
				if unitIndex < 0 || unitIndex >= len(partial.values) {
					selected.close()
					return false
				}
				values[routeIndex] = stagedSelectionValue[Tag, S]{ref: route.ref, tag: route.tag, value: partial.values[unitIndex]}
			}
			selected.values = append(selected.values, values)
		}
	}
	nextRows, sources, sealed := refinement.Seal()
	if !sealed || sources.SourceCount() != len(session.values) || sources.Count() != len(selected.values) || nextRows.Count() != len(selected.values) {
		selected.close()
		return false
	}
	next := make([]productRow, sources.Count())
	for source := range session.values {
		start, end, mapped := sources.Range(source)
		if !session.requireCheckpoint() || !mapped || start < 0 || end <= start || end > len(next) {
			selected.close()
			return false
		}
		for output := start; output < end; output++ {
			next[output] = extendProvenance(session.values[source], index, uint64(output+1))
		}
	}
	if !session.requireCheckpoint() {
		selected.close()
		return false
	}
	session.rows, session.values, session.sessions[index] = nextRows, next, selected
	return true
}

// refineStagedSource composes selected exact Unit partitions in canonical
// Unit order. This is the exact guarded cross-product required for
// correlation; it intentionally performs no quotient because canonical route
// tags and their order are semantic observations of a Selection.
func (runtime *stagedReadRuntime[V, S, Tag]) refineStagedSource(session *productSession, within support.Mask, units []carrier.Unit, selected *typedStagedSelectionSession[V, S, Tag]) ([]stagedPartial[S], bool) {
	if runtime == nil || session == nil || session.work == nil || runtime.target == nil || runtime.normalize == nil || selected == nil || !within.Valid() || support.Empty(within) || len(units) == 0 {
		return nil, false
	}
	partials := []stagedPartial[S]{{mask: within, values: make([]S, len(units))}}
	for unitIndex, unit := range units {
		if !session.requireCheckpoint() || unit == (carrier.Unit{}) {
			return nil, false
		}
		next := make([]stagedPartial[S], 0, len(partials))
		for _, partial := range partials {
			if !session.requireCheckpoint() || !partial.mask.Valid() || support.Empty(partial.mask) || len(partial.values) != len(units) {
				return nil, false
			}
			before := len(next)
			observed := runtime.target.stagedObserve(session.work, session.inputs[runtime.input], unit, partial.mask, func(observation factbinding.Observation[V], region support.Mask) bool {
				if !session.requireCheckpoint() || !observation.Valid() || !region.Valid() || support.Empty(region) {
					return false
				}
				cells := make([]summaryCell[V], observation.Count())
				for cellIndex := range cells {
					if !session.requireCheckpoint() {
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
				values := append([]S(nil), partial.values...)
				values[unitIndex] = runtime.normalize(OrderedCells[V]{record: record})
				selected.records = append(selected.records, record)
				next = append(next, stagedPartial[S]{mask: region, values: values})
				return true
			})
			if !observed || len(next) == before {
				return nil, false
			}
		}
		partials = next
	}
	return partials, true
}

// indexRoutes canonicalizes the observable semantic route order by (Unit,
// numeric tag), rejects duplicate semantic routes, and derives its physical
// Unit partition in one O(K log K) sort plus O(K) scan. Its scratch belongs
// to the execution-local selection session, so it neither allocates a map nor
// reintroduces a quadratic route path.
func (session *typedStagedSelectionSession[V, S, Tag]) indexRoutes(routes []stagedRoute[Tag]) ([]carrier.Unit, []int, bool) {
	if session == nil || len(routes) == 0 {
		return nil, nil, false
	}
	if cap(session.routeIndices) < len(routes) {
		session.routeIndices = make([]int, len(routes))
	} else {
		session.routeIndices = session.routeIndices[:len(routes)]
	}
	for _, route := range routes {
		if route.unit == (carrier.Unit{}) {
			return nil, nil, false
		}
	}
	sort.Slice(routes, func(left, right int) bool {
		leftUnit, rightUnit := routes[left].unit, routes[right].unit
		if leftUnit.Same(rightUnit) {
			return routes[left].tag < routes[right].tag
		}
		return leftUnit.Less(rightUnit)
	})
	session.routeUnits = session.routeUnits[:0]
	var previous carrier.Unit
	havePrevious := false
	var previousTag Tag
	for routeIndex, route := range routes {
		unit := route.unit
		if havePrevious {
			if previous.Same(unit) {
				if previousTag >= route.tag {
					return nil, nil, false
				}
			} else if !previous.Less(unit) {
				return nil, nil, false
			}
		}
		unitIndex := len(session.routeUnits) - 1
		if unitIndex < 0 || !session.routeUnits[unitIndex].Same(unit) {
			session.routeUnits = append(session.routeUnits, unit)
			unitIndex++
		}
		session.routeIndices[routeIndex] = unitIndex
		previous, previousTag, havePrevious = unit, route.tag, true
	}
	return session.routeUnits, session.routeIndices, len(session.routeUnits) != 0
}

func (session *typedStagedSelectionSession[V, S, Tag]) addDynamic(input int, unit carrier.Unit) {
	if session == nil || input < 0 || unit == (carrier.Unit{}) {
		return
	}
	key := stagedDemandKey{input: input, unit: unit}
	if session.dynamicSeen == nil {
		session.dynamicSeen = make(map[stagedDemandKey]struct{})
	}
	if _, known := session.dynamicSeen[key]; known {
		return
	}
	session.dynamicSeen[key] = struct{}{}
	session.dynamic = append(session.dynamic, demand.Observation{Input: uint64(input), Unit: unit})
}

func (session *typedStagedSelectionSession[V, S, Tag]) dynamicObservations() []demand.Observation {
	if session == nil || len(session.dynamic) == 0 {
		return nil
	}
	result := append([]demand.Observation(nil), session.dynamic...)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Input != result[right].Input {
			return result[left].Input < result[right].Input
		}
		return result[left].Unit.Less(result[right].Unit)
	})
	return result
}

func (session *typedStagedSelectionSession[V, S, Tag]) close() {
	if session == nil {
		return
	}
	for index := range session.values {
		clear(session.values[index])
		session.values[index] = nil
	}
	session.values = nil
	for _, record := range session.records {
		record.revoke()
	}
	session.records = nil
	session.dynamic = nil
	session.dynamicSeen = nil
	session.routeScratch = nil
	session.routeUnits = nil
	session.routeIndices = nil
	session.frame.routes = nil
}

func (runtime *typedReadRuntime[K, V, S]) inputPort() int { return runtime.input }

func (runtime *typedReadRuntime[K, V, S]) exactProof() ruleReadProof {
	if runtime == nil {
		return ruleReadProof{}
	}
	return runtime.proof
}

func (runtime *typedReadRuntime[K, V, S]) summaryProof() ruleSummaryReadProof {
	if runtime == nil {
		return ruleSummaryReadProof{}
	}
	return runtime.summary
}

func (runtime *typedReadRuntime[K, V, S]) refine(session *productSession, index int) bool {
	return runtime != nil && runtime.equal != nil && runtime.fingerprint != nil && materializeTypedRead(session, index, runtime.input, runtime.unit, runtime.binding.ResolveObservation, runtime.normalize, runtime.equal, runtime.fingerprint)
}

func (runtime *typedReadRuntime[K, V, S]) observations() []demand.Observation {
	if runtime == nil || runtime.unit == (carrier.Unit{}) || runtime.input < 0 {
		return nil
	}
	return []demand.Observation{{Input: uint64(runtime.input), Unit: runtime.unit}}
}

func (*typedReadRuntime[K, V, S]) dynamicReads() []demand.DynamicRead { return nil }

// readID resolves a row's exact identity for one declared read. Before the
// final freeze it follows only the persistent prefix; afterwards it reads the
// read-major column directly. The latter is the path exposed to Product and
// RuleAdmission callbacks.
func (session *productSession) readID(row, read int) (uint64, bool) {
	if session == nil || row < 0 || row >= len(session.values) || read < 0 || read >= len(session.reads) {
		return 0, false
	}
	return provenanceID(session.values, session.columns, row, read, len(session.reads))
}

// freezeProvenance converts the retained prefix forest into compact columns
// exactly once, after coalescing has selected the final rows. This is the only
// tuple copy: rows × reads, rather than one copy for every prior read at every
// refinement and another copy during coalescing.
func (session *productSession) freezeProvenance() bool {
	if session == nil || len(session.values) != session.rows.Count() {
		return false
	}
	columns, ok := freezeProvenanceColumns(session.checkpoint, session.values, len(session.reads))
	if !ok {
		return false
	}
	session.columns = columns
	return true
}

// observations is the precise typed surface used by this completed Product.
// It is copied before the session is revoked, so demand owns only carrier
// Units and ordered input positions rather than any transient row payload.
func (session *productSession) observations() []demand.Observation {
	if session == nil {
		return nil
	}
	result := make([]demand.Observation, 0, len(session.reads))
	for index, read := range session.reads {
		if !session.requireCheckpoint() {
			return nil
		}
		if read != nil {
			result = append(result, read.observations()...)
		}
		if index < len(session.sessions) && session.sessions[index] != nil {
			result = append(result, session.sessions[index].dynamicObservations()...)
		}
	}
	if !session.requireCheckpoint() {
		return nil
	}
	return result
}

func materializeTypedRead[V, S any](session *productSession, index, input int, unit carrier.Unit, resolve func(carrier.SlotWork, carrier.ObservationRow) (factbinding.Observation[V], bool), normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) bool {
	if session == nil || session.work == nil || index < 0 || index >= len(session.reads) || input < 0 || input >= len(session.inputs) || unit == (carrier.Unit{}) || resolve == nil || normalize == nil || equal == nil || fingerprint == nil || session.rows.Count() != len(session.values) {
		return false
	}
	slot, ok := unit.Slot()
	if !ok {
		return false
	}
	work, ok := session.work.SlotWork(slot)
	if !ok || !work.BeginObservation() {
		return false
	}
	defer work.EndObservation()
	root, ok := session.inputs[input].HandleAt(slot)
	if !ok {
		return false
	}
	values := &typedReadSession[V, S]{}
	refinement := session.rows.BeginRefineWithCheckpoint(session.checkpoint)
	if refinement == nil {
		return false
	}
	for source := 0; source < session.rows.Count(); source++ {
		if !session.requireCheckpoint() {
			values.close()
			return false
		}
		within, ok := session.rows.At(source)
		if !ok || !work.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
			if !session.requireCheckpoint() {
				return false
			}
			observation, ok := resolve(work, row)
			if !ok || !observation.Valid() {
				return false
			}
			cells := make([]summaryCell[V], observation.Count())
			for cellIndex := range cells {
				if !session.requireCheckpoint() {
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
	if !ok || sources.SourceCount() != len(session.values) || sources.Count() != len(values.values) {
		values.close()
		return false
	}
	// One output per source cannot contain an equal pair to quotient. Commit
	// that exact refinement directly: retain the sealed rows and append only
	// the new typed observation identity to each existing provenance prefix.
	if sources.Count() == sources.SourceCount() {
		for source := range session.values {
			start, end, mapped := sources.Range(source)
			if !session.requireCheckpoint() || !mapped || start != source || end != source+1 {
				values.close()
				return false
			}
		}
		for source := range session.values {
			session.values[source] = extendProvenance(session.values[source], index, uint64(source+1))
		}
		session.rows, session.sessions[index] = nextRows, values
		return true
	}
	next := make([]productRow, sources.Count())
	for source := 0; source < len(session.values); source++ {
		if !session.requireCheckpoint() {
			values.close()
			return false
		}
		start, end, ok := sources.Range(source)
		if !ok || end <= start {
			values.close()
			return false
		}
		for output := start; output < end; output++ {
			if !session.requireCheckpoint() {
				values.close()
				return false
			}
			next[output] = extendProvenance(session.values[source], index, uint64(output+1))
		}
	}
	rows, representatives, ok := nextRows.PrefixQuotientWithCheckpoint(sources, session.checkpoint, func(value int) (uint64, bool) {
		if !session.requireCheckpoint() || value < 0 || value >= len(values.values) {
			return 0, false
		}
		return fingerprint(values.values[value]), session.requireCheckpoint()
	}, func(left, right int) bool {
		if !session.requireCheckpoint() || left < 0 || right < 0 || left >= len(values.values) || right >= len(values.values) {
			return false
		}
		return equal(values.values[left], values.values[right]) && session.requireCheckpoint()
	})
	if !ok || len(representatives) == 0 {
		values.close()
		return false
	}
	compacted, compact := compactTypedRead(session, index, next, values, representatives)
	if !compact || rows.Count() != len(compacted) {
		values.close()
		return false
	}
	session.rows, session.values, session.sessions[index] = rows, compacted, values
	return true
}

// compactTypedRead retains the first PrefixQuotient representative for every
// output class. Its old prefix survives unchanged; only the just-materialized
// read identity is rewritten to the compact typed-session-local ordinal.
// Nothing reaches productSession until all validation and cancellation checks
// complete, so a failed quotient cannot publish a partial tuple.
func compactTypedRead[V, S any](session *productSession, index int, rows []productRow, values *typedReadSession[V, S], representatives []int) ([]productRow, bool) {
	if session == nil || index < 0 || values == nil || len(rows) == 0 || len(representatives) == 0 || len(values.values) != len(rows) || len(values.records) != len(rows) {
		return nil, false
	}
	compactedRows := make([]productRow, len(representatives))
	compactedValues := make([]S, len(representatives))
	compactedRecords := make([]*orderedCellsRecord[V], len(representatives))
	retained := make([]bool, len(rows))
	for output, representative := range representatives {
		if !session.requireCheckpoint() || representative < 0 || representative >= len(rows) || retained[representative] || values.records[representative] == nil {
			return nil, false
		}
		prefix := rows[representative].prefix
		if prefix == nil || prefix.read != index || prefix.id != uint64(representative+1) {
			return nil, false
		}
		retained[representative] = true
		compactedRows[output] = extendProvenance(provenanceRow{prefix: prefix.previous}, index, uint64(output+1))
		compactedValues[output] = values.values[representative]
		compactedRecords[output] = values.records[representative]
	}
	if !session.requireCheckpoint() {
		return nil, false
	}
	for output, record := range values.records {
		if !retained[output] {
			record.revoke()
		}
	}
	values.values, values.records = compactedValues, compactedRecords
	return compactedRows, true
}

func resolveTypedRead[V, S any](session *productSession, index int, id uint64) (S, bool) {
	var zero S
	if session == nil || index < 0 || index >= len(session.sessions) || id == 0 {
		return zero, false
	}
	values, ok := session.sessions[index].(*typedReadSession[V, S])
	if !ok || id > uint64(len(values.values)) {
		return zero, false
	}
	return values.values[id-1], true
}

func (session *typedReadSession[V, S]) close() {
	if session == nil {
		return
	}
	var zero S
	for index := range session.values {
		session.values[index] = zero
	}
	session.values = nil
	for _, record := range session.records {
		record.revoke()
	}
	session.records = nil
}

func (*typedReadSession[V, S]) dynamicObservations() []demand.Observation { return nil }

func resolveTypedSelection[V, S any, Tag selectionTag](session *productSession, index int, id uint64) (Selection[Tag, S], bool) {
	if session == nil || session.execution == nil || index < 0 || index >= len(session.sessions) || id == 0 {
		return Selection[Tag, S]{}, false
	}
	values, ok := session.sessions[index].(*typedStagedSelectionSession[V, S, Tag])
	if !ok || id > uint64(len(values.values)) {
		return Selection[Tag, S]{}, false
	}
	// A staged read may split its source row by each selected exact Unit. Its
	// provenance id therefore names the source-major materialization output,
	// not a final Product row. Both public and locator-scoped accessors retain
	// the caller's row fence before exposing this payload.
	selectionID := int(id) - 1
	if selectionID < 0 || selectionID >= len(values.values) {
		return Selection[Tag, S]{}, false
	}
	epoch := session.execution.epoch
	if epoch == 0 || session.execution.active.Load() != epoch {
		return Selection[Tag, S]{}, false
	}
	return Selection[Tag, S]{
		session:     session,
		epoch:       epoch,
		read:        index,
		selectionID: id,
		count: func(row int) (int, bool) {
			if row < 0 || row >= len(session.values) {
				return 0, false
			}
			return len(values.values[selectionID]), true
		},
		at: func(row, ordinal int) (Tag, S, bool) {
			var tag Tag
			var zero S
			if row < 0 || row >= len(session.values) || ordinal < 0 || ordinal >= len(values.values[selectionID]) {
				return tag, zero, false
			}
			entry := values.values[selectionID][ordinal]
			return entry.tag, entry.value, true
		},
		route: func(row, ordinal int) (exactRef, bool) {
			if row < 0 || row >= len(session.values) || ordinal < 0 || ordinal >= len(values.values[selectionID]) {
				return nil, false
			}
			ref := values.values[selectionID][ordinal].ref
			return ref, ref != nil
		},
	}, true
}

type outputAccess[V any] struct {
	begin          func(*ruleExecution) outputSession
	stage          func(*ruleExecution, uint64, int, V) bool
	stageTransform func(*ruleExecution, uint64, int) bool
	noCandidate    func(*ruleExecution, uint64, int) bool
	stageSelection func(*ruleExecution, uint64, int, routeOutputBatch[V]) bool
	noSelection    func(*ruleExecution, uint64, int, routeOutputBatch[V]) bool
	derivation     func(outputSession) ([]RuleDisposition[V], bool)
}

// routeOutputBatch is assembled synchronously by StageSelection. It retains
// the canonical selected ordinal and its owner-issued exact Ref together with
// the route-local value, so output application never has an unpaired V path.
type routeOutputBatch[V any] struct {
	read        int
	selectionID uint64
	count       int
	// at is consumed in canonical Selection order by the Factor-owned route
	// sink. It keeps Ref/tag-derived output values paired without allocating
	// a per-row Ref×value staging plane.
	at func(int) (exactRef, V, bool)
}

type outputSession interface {
	accept(*RuleEvidence) (carrier.Patch, bool)
	discard()
	complete() bool
	hasStaged() bool
	settled(int) bool
}

// outputRuntime is the private per-row staged-target projection. Direct
// targets and selector targets share one ordered write vector so a selector
// may consult already computed target bits but never future output state.
type outputRuntime struct{ writes []outputWriteRuntime }

func (runtime *outputRuntime) routeRead() (uint64, bool) {
	if runtime == nil {
		return 0, false
	}
	var found uint64
	for _, write := range runtime.writes {
		if write.routeRead == 0 {
			continue
		}
		if found != 0 || write.selector != nil || write.direct != (carrier.Target{}) {
			return 0, false
		}
		found = write.routeRead
	}
	return found, found != 0
}

type outputWriteRuntime struct {
	// routeRead is the one-based staged read ordinal consumed by a route batch.
	// Zero is the ordinary direct/static target form.
	routeRead  uint64
	direct     carrier.Target
	directID   ruleTargetProof
	selector   *coldWriteSelector
	candidates []int
	targets    []carrier.Target
	targetIDs  []ruleTargetProof
	relations  map[int][][]int
}

type resolvedRuleTarget struct {
	target carrier.Target
	proof  ruleTargetProof
}

func (runtime *outputRuntime) targets(execution *ruleExecution, row int) ([]resolvedRuleTarget, bool) {
	if runtime == nil || execution == nil || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || len(runtime.writes) == 0 {
		return nil, false
	}
	selected := make([][]bool, len(runtime.writes))
	result := make([]resolvedRuleTarget, 0, len(runtime.writes))
	for writeIndex, write := range runtime.writes {
		if !execution.product.requireCheckpoint() {
			return nil, false
		}
		if write.routeRead != 0 {
			return nil, false
		}
		if write.selector == nil {
			if write.direct == (carrier.Target{}) {
				return nil, false
			}
			selected[writeIndex] = []bool{true}
			if write.directID.sealAuthority == 0 {
				return nil, false
			}
			result = append(result, resolvedRuleTarget{target: write.direct, proof: write.directID})
			continue
		}
		if len(write.candidates) == 0 || len(write.candidates) != len(write.targets) || len(write.candidates) != len(write.targetIDs) || len(write.candidates) != len(write.selector.candidates) {
			return nil, false
		}
		bits := make([]bool, len(write.candidates))
		frame := &selectorFrame{execution: execution, epoch: execution.epoch, write: write.selector, product: execution.product, row: row, current: -1, requireCurrent: true}
		frame.selected = func(prior, current int) (bool, bool) {
			if prior < 0 || prior >= writeIndex || current < 0 || current >= len(bits) || prior >= len(selected) || len(selected[prior]) == 0 {
				return false, false
			}
			matches, ok := write.relations[prior]
			if !ok || current >= len(matches) {
				return false, false
			}
			for _, ordinal := range matches[current] {
				if !execution.product.requireCheckpoint() {
					return false, false
				}
				if ordinal < 0 || ordinal >= len(selected[prior]) {
					return false, false
				}
				if selected[prior][ordinal] {
					return true, true
				}
			}
			return false, true
		}
		for ordinal := range write.candidates {
			if !execution.product.requireCheckpoint() {
				return nil, false
			}
			frame.current = ordinal
			choose := runWriteSelector(frame, write.selector.decide)
			if execution.failed.Load() {
				return nil, false
			}
			if choose {
				bits[ordinal] = true
				if write.targetIDs[ordinal].sealAuthority == 0 {
					return nil, false
				}
				result = append(result, resolvedRuleTarget{target: write.targets[ordinal], proof: write.targetIDs[ordinal]})
			}
		}
		selected[writeIndex] = bits
	}
	return result, true
}

type typedOutput[K ~uint32 | ~uint64, V any] struct {
	execution    *ruleExecution
	binding      *factbinding.Binding[K, V]
	targets      func(*ruleExecution, int) ([]resolvedRuleTarget, bool)
	routeRead    uint64
	routeTarget  func(exactRef) (carrier.Target, ruleTargetProof, bool)
	patch        *factbinding.Patch[K, V]
	transform    typedCarryTransform[K, V]
	routeScratch []V
	// disposition is one execution-local byte per materialized Product row.
	// It is allocated only when a transfer settles its first row, and records
	// the sole legal outcome for that row: a staged value or an explicit empty
	// successor. It is not a fact plane and never escapes the execution.
	disposition  []outputDisposition
	staged       bool
	dispositions []RuleDisposition[V]
	proofCount   int
	closed       bool
}

// typedCarryTransform is a Factor-owned map plus the immutable, precompiled
// carried target closure on which it may act.  It has no domain vocabulary;
// only the owner-specific V callback is retained here.
type typedCarryTransform[K ~uint32 | ~uint64, V any] struct {
	semantic SemanticKey
	closures []factbinding.TransformClosure[K, V]
	apply    func(V) (V, bool)
}

func (transform typedCarryTransform[K, V]) active() bool {
	return transform.semantic.Available() && transform.apply != nil
}

type transformedCarryOwner[V any] interface {
	transformedCarry() (SemanticKey, []carrier.Target, func(V) (V, bool), bool)
}

type transformedCarryRouteOwner interface {
	transformedCarryRoute() bool
}

type outputDisposition uint8

const (
	outputUnset outputDisposition = iota
	outputStaged
	outputNoCandidate
)

// newTypedOutputAccess closes the Factor owner's private K at cold binding
// time. Rule compilation sees only runtimeFactor plus this V-specialized
// access object, so a named K never needs reflection, an offset conversion,
// or a type-switch arm in the hot runtime.
func newTypedOutputAccess[K ~uint32 | ~uint64, V any](output *boundFactor[K, V], owner anyRule, projection *outputRuntime) (outputAccess[V], bool) {
	if output == nil || output.binding == nil || owner == nil || projection == nil {
		return outputAccess[V]{}, false
	}
	routeRead, routeOutput := projection.routeRead()
	if routeOutput && !output.hasRouteUniverse() {
		return outputAccess[V]{}, false
	}
	var transform typedCarryTransform[K, V]
	if transformed, present := owner.(transformedCarryOwner[V]); present {
		semantic, targets, apply, active := transformed.transformedCarry()
		if active {
			defaultValue, defaultOK := output.factor.algebra.Default()
			mappedDefault, mappedOK := apply(defaultValue)
			if !defaultOK || !mappedOK || !output.factor.algebra.Equal(defaultValue, mappedDefault) {
				return outputAccess[V]{}, false
			}
			closure, closed := output.binding.TransformClosure(targets)
			if !closed || !semantic.Available() || apply == nil {
				return outputAccess[V]{}, false
			}
			closures := []factbinding.TransformClosure[K, V]{closure}
			if routeOwner, routeRequired := owner.(transformedCarryRouteOwner); routeRequired && routeOwner.transformedCarryRoute() {
				if !output.hasRouteUniverse() {
					return outputAccess[V]{}, false
				}
				routeClosure, routeOK := output.routeTransformClosure()
				if !routeOK {
					return outputAccess[V]{}, false
				}
				closures = append(closures, routeClosure)
			}
			transform = typedCarryTransform[K, V]{semantic: semantic, closures: closures, apply: apply}
		}
	}
	return outputAccess[V]{
		begin: func(execution *ruleExecution) outputSession {
			if execution == nil || execution.owner != owner || execution.work == nil {
				return nil
			}
			// A no-candidate Product must not create an empty Factor patch. Delay
			// the binding scratch until the first actual StageValue.
			typed := &typedOutput[K, V]{execution: execution, binding: output.binding, targets: projection.targets, transform: transform}
			if routeOutput {
				typed.routeRead = routeRead
				typed.routeTarget = output.stagedTarget
			}
			return typed
		},
		stage: func(execution *ruleExecution, epoch uint64, row int, value V) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			if !ok || !typed.stage(execution, epoch, row, value) {
				return false
			}
			return true
		},
		stageTransform: func(execution *ruleExecution, epoch uint64, row int) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.stageTransform(execution, epoch, row)
		},
		noCandidate: func(execution *ruleExecution, epoch uint64, row int) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.noCandidate(execution, epoch, row)
		},
		stageSelection: func(execution *ruleExecution, epoch uint64, row int, batch routeOutputBatch[V]) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.stageSelection(execution, epoch, row, batch)
		},
		noSelection: func(execution *ruleExecution, epoch uint64, row int, batch routeOutputBatch[V]) bool {
			if execution == nil || execution.owner != owner {
				return false
			}
			typed, ok := execution.output.(*typedOutput[K, V])
			return ok && typed.noSelection(execution, epoch, row, batch)
		},
		derivation: func(session outputSession) ([]RuleDisposition[V], bool) {
			typed, ok := session.(*typedOutput[K, V])
			if !ok || typed.execution == nil || typed.execution.product == nil || !typed.execution.product.requireCheckpoint() {
				return nil, false
			}
			if typed.proofCount != len(typed.dispositions) {
				return nil, false
			}
			// Targets came from sealed cold assembly and RuleTarget exposes only
			// equality. The derivation can share their immutable backing rather
			// than copying every target vector per executed row.
			return typed.dispositions, true
		},
	}, true
}

func (output *typedOutput[K, V]) beginPatch(execution *ruleExecution) bool {
	if output == nil || execution == nil || output.patch != nil {
		return output != nil && output.patch != nil
	}
	output.patch = output.binding.Begin(execution.work, execution.base.State())
	return output.patch != nil
}

// applyCarryTransform applies the one declared carry map before any ordinary
// exact writes for this Product row.  A no-candidate row never reaches this
// method.  The Patch remains unpublished until the enclosing Group finishes,
// so a later write failure discards both operations together.
func (output *typedOutput[K, V]) applyCarryTransform(execution *ruleExecution, when support.Mask) bool {
	if output == nil || !output.transform.active() {
		return true
	}
	return output.beginPatch(execution) && output.patch.TransformClosures(output.transform.closures, when, output.transform.apply)
}

func (output *typedOutput[K, V]) stage(execution *ruleExecution, epoch uint64, row int, value V) bool {
	if output == nil || output.closed || output.execution != execution || execution == nil || execution.active.Load() != epoch || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset {
		return false
	}
	if output.routeRead != 0 || output.targets == nil || output.disposition != nil && output.disposition[row] != outputUnset || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
		return false
	}
	targets, ok := output.targets(execution, row)
	if !ok {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if len(targets) == 0 {
		// A Factor write must name at least one sealed target. Selector
		// emptiness is an explicit NoCandidate outcome, never a no-op stage.
		return false
	}
	if !output.applyCarryTransform(execution, when) || !output.beginPatch(execution) {
		return false
	}
	for _, target := range targets {
		if !execution.product.requireCheckpoint() {
			return false
		}
		if !output.patch.Write(target.target, when, value) {
			return false
		}
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputStaged
	output.staged = true
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		resolved := make([]RuleTarget, len(targets))
		for index, target := range targets {
			resolved[index] = RuleTarget{target: target.target, proof: target.proof}
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionStaged, value: value, guard: RuleGuard{mask: when}, targets: resolved, carryTransform: output.transform.semantic, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

// stageTransform settles one row whose only semantic effect is its declared
// transformed carry.  It exists so a transform-only Rule has no sentinel
// write or parallel carry publication path.
func (output *typedOutput[K, V]) stageTransform(execution *ruleExecution, epoch uint64, row int) bool {
	if output == nil || output.closed || !output.transform.active() || output.execution != execution || execution == nil || execution.active.Load() != epoch || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() || !output.applyCarryTransform(execution, when) {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputStaged
	output.staged = true
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionStaged, guard: RuleGuard{mask: when}, carryTransform: output.transform.semantic, transformOnly: true, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

func (output *typedOutput[K, V]) noCandidate(execution *ruleExecution, epoch uint64, row int) bool {
	if output == nil || output.closed || output.execution != execution || execution == nil || execution.active.Load() != epoch || execution.product == nil || !execution.product.requireCheckpoint() || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
		return false
	}
	if output.routeRead != 0 {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputNoCandidate
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionNoCandidate, guard: RuleGuard{mask: when}, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

func (output *typedOutput[K, V]) validRouteBatch(execution *ruleExecution, epoch uint64, row int, batch routeOutputBatch[V]) bool {
	if output == nil || output.closed || output.execution != execution || execution == nil || execution.active.Load() != epoch || execution.product == nil || !execution.product.requireCheckpoint() ||
		output.routeRead == 0 || output.routeTarget == nil || row != execution.product.current || row < 0 || row >= len(execution.product.values) || output.disposition != nil && output.disposition[row] != outputUnset ||
		batch.read < 0 || uint64(batch.read)+1 != output.routeRead || batch.selectionID == 0 || batch.count < 0 || batch.count > 0 && batch.at == nil {
		return false
	}
	actual, ok := execution.product.readID(row, int(output.routeRead-1))
	return ok && actual == batch.selectionID
}

// stageSelection applies one complete selected-route batch. It authenticates
// every Ref against the output Factor, retains every ordinal pair as evidence,
// then groups equal exact targets and delegates their reduction to the
// Factor's admitted Join before a single strong Set per target.
func (output *typedOutput[K, V]) stageSelection(execution *ruleExecution, epoch uint64, row int, batch routeOutputBatch[V]) bool {
	if !output.validRouteBatch(execution, epoch, row, batch) || batch.count == 0 || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if !output.applyCarryTransform(execution, when) || !output.beginPatch(execution) {
		return false
	}
	var pairs []RuleOutput[V]
	if execution.owner != nil && execution.owner.requiresDerivation() {
		pairs = make([]RuleOutput[V], 0, batch.count)
	}
	var target carrier.Target
	begin := 0
	for ordinal := 0; ordinal < batch.count; ordinal++ {
		ref, value, available := batch.at(ordinal)
		current, proof, resolved := output.routeTarget(ref)
		if !available || !resolved || current == (carrier.Target{}) || proof.sealAuthority == 0 {
			return false
		}
		if pairs != nil {
			pairs = append(pairs, RuleOutput[V]{target: RuleTarget{target: current, proof: proof}, value: value, ordinal: ordinal})
		}
		if ordinal == 0 {
			target, begin = current, ordinal
			output.routeScratch = append(output.routeScratch[:0], value)
			continue
		}
		if current.Same(target) {
			output.routeScratch = append(output.routeScratch, value)
			continue
		}
		// SelectRoute is canonical Unit→tag order and this Factor's target
		// universe is declared in the same exact-key order. A decreasing target
		// therefore proves a broken route-to-target correspondence rather than
		// asking the hot path to repair it with a sort.
		if current.Less(target) || !execution.product.requireCheckpoint() || !output.patch.WriteJoined(target, when, output.routeScratch) {
			return false
		}
		target, begin = current, ordinal
		output.routeScratch = append(output.routeScratch[:0], value)
	}
	if begin < batch.count && (!execution.product.requireCheckpoint() || !output.patch.WriteJoined(target, when, output.routeScratch)) {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputStaged
	output.staged = true
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionStaged, guard: RuleGuard{mask: when}, outputs: pairs, carryTransform: output.transform.semantic, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

func (output *typedOutput[K, V]) noSelection(execution *ruleExecution, epoch uint64, row int, batch routeOutputBatch[V]) bool {
	if !output.validRouteBatch(execution, epoch, row, batch) || batch.count != 0 || execution.owner != nil && execution.owner.requiresDerivation() && row != output.proofCount {
		return false
	}
	when, ok := execution.product.rows.At(row)
	if !ok || !when.Valid() {
		return false
	}
	if output.disposition == nil {
		output.disposition = make([]outputDisposition, len(execution.product.values))
	}
	output.disposition[row] = outputNoCandidate
	if execution.owner != nil && execution.owner.requiresDerivation() {
		if output.dispositions == nil {
			output.dispositions = make([]RuleDisposition[V], len(execution.product.values))
		}
		output.dispositions[row] = RuleDisposition[V]{kind: RuleDispositionNoCandidate, guard: RuleGuard{mask: when}, row: ruleResultRow{index: row}, ordinal: row}
		output.proofCount++
	}
	return true
}

func (output *typedOutput[K, V]) complete() bool {
	if output == nil || output.closed || output.execution == nil || output.execution.product == nil || !output.execution.product.requireCheckpoint() {
		return false
	}
	rows := len(output.execution.product.values)
	if rows == 0 {
		return true
	}
	if len(output.disposition) != rows {
		return false
	}
	for _, disposition := range output.disposition {
		if disposition == outputUnset {
			return false
		}
	}
	if output.execution.owner != nil && output.execution.owner.requiresDerivation() && (len(output.dispositions) != rows || output.proofCount != rows) {
		return false
	}
	return true
}

func (output *typedOutput[K, V]) hasStaged() bool {
	return output != nil && output.staged
}

func (output *typedOutput[K, V]) settled(row int) bool {
	return output != nil && output.execution != nil && output.execution.product != nil && row == output.execution.product.current && row >= 0 && row < len(output.disposition) && output.disposition[row] != outputUnset
}

func (output *typedOutput[K, V]) accept(evidence *RuleEvidence) (carrier.Patch, bool) {
	if output == nil || output.closed || output.execution == nil || output.execution.failed.Load() || !evidence.consume() {
		return carrier.Patch{}, false
	}
	output.closed = true
	if output.patch == nil {
		return carrier.Patch{}, false
	}
	patch := output.patch
	accepted, ok := patch.Accept(output.execution.work)
	// Patch.Accept owns deterministic stage/pending-root cleanup on failure.
	// Retain the sole pointer here until that transaction has returned.
	output.patch = nil
	return accepted, ok
}

func (output *typedOutput[K, V]) discard() {
	if output == nil || output.closed {
		return
	}
	output.closed = true
	if output.patch != nil {
		output.patch.Discard()
		output.patch = nil
	}
}

type boundRule[V, O any] struct {
	rule           *ruleSchema
	admission      RuleAdmission[V, O]
	anchor         SemanticKey
	operandContent [32]byte
	coordinates    ActivationCoordinates
	transfer       func(Access[V, O]) bool
	operand        O
	reads          []readRuntime
	output         outputAccess[V]
	// routeScope retains only the owning Factor authority. Its O(R) target
	// vector and transformed-carry closure remain Factor/Binding-owned; this
	// member retains only authored carry surfaces plus the route bit.
	routeScope     runtimeFactor
	routeTransform bool
	carrySemantic  SemanticKey
	carryTargets   []carrier.Target
	carryApply     func(V) (V, bool)
	carryOnly      bool
	nextEpoch      atomic.Uint64
}

func (bound *boundRule[V, O]) transformedCarry() (SemanticKey, []carrier.Target, func(V) (V, bool), bool) {
	if bound == nil || !bound.carrySemantic.Available() || bound.carryApply == nil {
		return SemanticKey{}, nil, nil, false
	}
	return bound.carrySemantic, bound.carryTargets, bound.carryApply, true
}

func (bound *boundRule[V, O]) transformedCarryRoute() bool {
	return bound != nil && bound.routeTransform
}

func (bound *boundRule[V, O]) validCarryDisposition(disposition RuleDisposition[V]) bool {
	semantic, transformed := disposition.CarryTransform()
	if disposition.Kind() == RuleDispositionNoCandidate {
		return !transformed && !disposition.TransformOnly()
	}
	if disposition.Kind() != RuleDispositionStaged {
		return false
	}
	if bound.carrySemantic.Available() {
		return transformed && semantic == bound.carrySemantic
	}
	return !transformed && !disposition.TransformOnly()
}

func (bound *boundRule[V, O]) requiresDerivation() bool {
	return bound != nil && bound.admission.kind == ruleAdmissionDerivation
}

func (bound *boundRule[V, O]) initialReads() []demand.Observation {
	if bound == nil {
		return nil
	}
	result := make([]demand.Observation, 0, len(bound.reads))
	for _, read := range bound.reads {
		result = append(result, read.observations()...)
	}
	return result
}

func (bound *boundRule[V, O]) dynamicReads() []demand.DynamicRead {
	if bound == nil {
		return nil
	}
	result := make([]demand.DynamicRead, 0, len(bound.reads))
	for _, read := range bound.reads {
		if read != nil {
			result = append(result, read.dynamicReads()...)
		}
	}
	return result
}

type ruleExecution struct {
	owner   anyRule
	work    *carrier.Work
	base    carrier.ContributionBase
	inputs  []carrier.State
	epoch   uint64
	active  atomic.Uint64
	failed  atomic.Bool
	product *productSession
	output  outputSession
}

// anyRule contains no typed Fact payload. It exists only to ensure an Access
// cannot be replayed against another canonical Rule-instance row.
type anyRule interface {
	ruleSchema() *ruleSchema
	requiresDerivation() bool
}

func (bound *boundRule[V, O]) ruleSchema() *ruleSchema {
	if bound == nil {
		return nil
	}
	return bound.rule
}

// bindMemberRule turns graph-owned member metadata into the one private
// per-row target projection. The caller supplies no write surface, selector
// candidate, dependency, or relation: the member is the sole occurrence owner.
func bindMemberRule[V, O any](member equation.RuleMember, rule *Rule[V, O], operand O, output runtimeFactor) (*boundRule[V, O], []carrier.Target, bool) {
	if !member.Key().Available() || !member.Rule().Available() || rule == nil || !rule.available() || rule.schema == nil || !rule.admission.same(rule.schema.admission) || output == nil || rule.schema.output == nil ||
		output.semantic() != rule.schema.output.semantic || member.ReadCount() != len(rule.schema.reads) || member.WriteCount() != len(rule.schema.writes) || len(rule.schema.writes) == 0 && len(rule.schema.carries) == 0 {
		return nil, nil, false
	}
	coordinates := ActivationCoordinates{}
	if activation, dynamic := member.ActivationMember(); dynamic {
		locator, located := activation.Locator()
		if !located {
			return nil, nil, false
		}
		coordinates = ActivationCoordinates{
			binding:     semanticKeyFromComposition(activation.Binding()),
			application: semanticKeyFromComposition(locator.Application),
			target:      semanticKeyFromComposition(locator.Target),
			endpoint:    semanticKeyFromComposition(locator.Endpoint),
		}
		if !coordinates.Available() {
			return nil, nil, false
		}
	}
	projection := &outputRuntime{writes: make([]outputWriteRuntime, len(rule.schema.writes))}
	all := make([]carrier.Target, 0, len(rule.schema.writes))
	for writeIndex, cold := range rule.schema.writes {
		if cold.form == nil || cold.form.factor != rule.schema.output {
			return nil, nil, false
		}
		surface, ok := member.WriteAt(writeIndex)
		if !ok {
			return nil, nil, false
		}
		switch cold.form.writeKind {
		case exactWriteForm:
			candidateCount, candidatesOK := member.WriteCandidateCount(writeIndex)
			dependencyCount, dependenciesOK := member.WriteDependencyCount(writeIndex)
			relationCount, relationsOK := member.WriteRelationCount(writeIndex)
			if !candidatesOK || !dependenciesOK || !relationsOK || candidateCount != 0 || dependencyCount != 0 || relationCount != 0 {
				return nil, nil, false
			}
			if cold.route != 0 {
				route, routeOK := member.WriteRouteRead(writeIndex)
				if !routeOK || route != cold.route || cold.route-1 >= uint64(len(rule.schema.reads)) || !output.hasRouteUniverse() {
					return nil, nil, false
				}
				projection.writes[writeIndex] = outputWriteRuntime{routeRead: cold.route}
				continue
			}
			route, routeOK := member.WriteRouteRead(writeIndex)
			if !routeOK || route != 0 {
				return nil, nil, false
			}
			target, ok := output.writeTarget(surface)
			if !ok {
				return nil, nil, false
			}
			proof, proofOK := newRuleTargetProof(rule.schema.output, surface)
			if !proofOK {
				return nil, nil, false
			}
			projection.writes[writeIndex] = outputWriteRuntime{direct: target, directID: proof}
			all = appendUniqueTarget(all, target)
		case selectorWriteForm:
			if cold.route != 0 {
				return nil, nil, false
			}
			selector := coldSelectorForWrite(rule.schema, writeIndex)
			issued, ok := output.writeSelector(surface)
			candidateCount, candidatesOK := member.WriteCandidateCount(writeIndex)
			dependencyCount, dependenciesOK := member.WriteDependencyCount(writeIndex)
			if !ok || !candidatesOK || !dependenciesOK || issued.Kind() != carrier.TargetSelector || selector == nil || len(selector.candidates) == 0 || candidateCount != len(selector.candidates) || dependencyCount != len(selector.depends) {
				return nil, nil, false
			}
			targets, ok := output.writeSelectorCandidates(surface)
			if !ok || len(targets) != len(selector.candidates) {
				return nil, nil, false
			}
			targetIDs := make([]ruleTargetProof, len(selector.candidates))
			for candidate, read := range selector.candidates {
				bound, ok := member.WriteCandidateAt(writeIndex, candidate)
				targetID, targetOK := member.WriteTargetCandidateAt(writeIndex, candidate)
				if !ok || !targetOK || bound != uint64(read) {
					return nil, nil, false
				}
				proof, proofOK := newRuleTargetProof(rule.schema.output, targetID)
				if !proofOK {
					return nil, nil, false
				}
				targetIDs[candidate] = proof
			}
			relations := make(map[int][][]int)
			targetDependencies := 0
			for dependencyIndex, dependency := range selector.depends {
				bound, ok := member.WriteDependencyAt(writeIndex, dependencyIndex)
				if !ok || bound.Index != uint64(dependency.index) || bound.Target != (dependency.kind == writeDependency) {
					return nil, nil, false
				}
				if dependency.kind != writeDependency {
					continue
				}
				relation, ok := member.WriteRelationAt(writeIndex, targetDependencies)
				if !ok || relation.Prior != uint64(dependency.index) || len(relation.Matches) != len(selector.candidates) {
					return nil, nil, false
				}
				matches := make([][]int, len(relation.Matches))
				for current, row := range relation.Matches {
					matches[current] = make([]int, len(row))
					for ordinal, prior := range row {
						if prior > uint64(^uint(0)>>1) {
							return nil, nil, false
						}
						matches[current][ordinal] = int(prior)
					}
				}
				relations[dependency.index] = matches
				targetDependencies++
			}
			relationCount, relationsOK := member.WriteRelationCount(writeIndex)
			if !relationsOK || relationCount != targetDependencies {
				return nil, nil, false
			}
			projection.writes[writeIndex] = outputWriteRuntime{selector: selector, candidates: append([]int(nil), selector.candidates...), targets: targets, targetIDs: targetIDs, relations: relations}
			for _, target := range targets {
				all = appendUniqueTarget(all, target)
			}
		default:
			return nil, nil, false
		}
	}
	anchor := semanticKeyFromComposition(member.Key())
	if !anchor.Available() {
		return nil, nil, false
	}
	entity := OperandEntity{key: member.Operand().Entity()}
	if !entity.key.Available() || entity.key.Version != instanceOperandEntityVersion {
		return nil, nil, false
	}
	operandContent := [32]byte(entity.key.ID)
	if operandContent == [32]byte{} {
		return nil, nil, false
	}
	hasRouteWrite := false
	for _, write := range projection.writes {
		if write.routeRead != 0 {
			hasRouteWrite = true
			break
		}
	}
	carryRouteScope := output.carryRouteScopeFor(member)
	routeScope := hasRouteWrite || carryRouteScope
	if routeScope && !output.hasRouteUniverse() {
		return nil, nil, false
	}
	if len(all) == 0 && len(rule.schema.carries) == 0 && !routeScope {
		return nil, nil, false
	}
	bound := &boundRule[V, O]{rule: rule.schema, admission: rule.admission, anchor: anchor, operandContent: operandContent, coordinates: coordinates, transfer: rule.transfer, operand: operand}
	if len(rule.schema.carries) == 1 && rule.schema.carries[0].transform.Available() {
		carry := rule.schema.carries[0]
		if rule.carryTransform != carry.transform || rule.carryApply == nil {
			return nil, nil, false
		}
		carryTargets, targetsOK := output.carryTargetsFor(member)
		if !targetsOK {
			return nil, nil, false
		}
		bound.carrySemantic = carry.transform
		bound.carryTargets = carryTargets
		bound.routeTransform = carryRouteScope
		// The immutable, content-addressed instance operand closes the
		// Factor-owned transform exactly once. Allocation recency can therefore
		// select its own root without a dynamic registry or a global transform
		// callback, while the hot terminal map remains monomorphic V -> V.
		bound.carryApply = func(value V) (V, bool) {
			return rule.carryApply(operand, value)
		}
	}
	bound.carryOnly = len(rule.schema.writes) == 0 && !bound.carrySemantic.Available()
	if routeScope {
		bound.routeScope = output
	}
	if !bound.carryOnly {
		access, boundOutput := rule.output.bindOutput(output, bound, projection)
		if !boundOutput {
			return nil, nil, false
		}
		bound.output = access
	}
	return bound, all, true
}

func newRuleTargetProof(factor *factorSchema, surface equation.Surface) (ruleTargetProof, bool) {
	if factor == nil || factor.composition == nil || !factor.composition.Sealed() || factor.open || !factor.bound ||
		factor.composition.sealAuthority == 0 || surface.Factor != factor.semantic.compositionKey() ||
		surface.Form != equation.SurfaceWriteExact || surface.Local == 0 || surface.Local-1 >= factor.keyEnd ||
		(surface.Mode != equation.TargetModeStrong && surface.Mode != equation.TargetModeWeak) {
		return ruleTargetProof{}, false
	}
	return ruleTargetProof{
		sealAuthority: factor.composition.sealAuthority,
		factorIndex:   factor.bindIndex,
		raw:           surface.Local - 1,
		strong:        surface.Mode == equation.TargetModeStrong,
	}, true
}

func newRuleReadProof(factor *factorSchema, surface equation.Surface) (ruleReadProof, bool) {
	if factor == nil || factor.composition == nil || !factor.composition.Sealed() || factor.open || !factor.bound ||
		factor.composition.sealAuthority == 0 || surface.Factor != factor.semantic.compositionKey() ||
		surface.Form != equation.SurfaceReadExact || surface.Local == 0 || surface.Local-1 >= factor.keyEnd ||
		surface.Mode != equation.TargetModeNone {
		return ruleReadProof{}, false
	}
	return ruleReadProof{
		sealAuthority: factor.composition.sealAuthority,
		factorIndex:   factor.bindIndex,
		raw:           surface.Local - 1,
		exact:         true,
	}, true
}

func coldSelectorForWrite(rule *ruleSchema, write int) *coldWriteSelector {
	if rule == nil || write < 0 {
		return nil
	}
	for index := range rule.writeSelectors {
		selector := &rule.writeSelectors[index]
		if selector.write == write {
			return selector
		}
	}
	return nil
}

func appendUniqueTarget(targets []carrier.Target, candidate carrier.Target) []carrier.Target {
	for _, target := range targets {
		if target.Same(candidate) {
			return targets
		}
	}
	return append(targets, candidate)
}

// readBinding is the one private E-side sink for a cold Rule's ordered typed
// read projection. Factor and structural support Rules use this exact path;
// only Factor Rules additionally install an output Patch session.
type readBinding interface {
	ruleSchema() *ruleSchema
	appendReadRuntime(readRuntime) bool
}

// bindRuntimeRuleReads consumes a Rule's sealed positional read binders in
// Graph member order. The compiler supplies only the factor catalog; every
// typed normalization, equality, and selector callback remains schema-owned.
func bindRuntimeRuleReads(schema *ruleSchema, target readBinding, member equation.RuleMember, factors map[composition.Key]runtimeFactor) bool {
	if schema == nil || target == nil || len(schema.reads) != member.ReadCount() || factors == nil {
		return false
	}
	for index, declared := range schema.reads {
		if declared.bind == nil || declared.form == nil {
			return false
		}
		factor, ok := factors[declared.form.factor.semantic.compositionKey()]
		if !ok || factor == nil || !declared.bind.bindRuntimeRead(schema, target, member, factor) {
			return false
		}
		if index != len(schema.reads)-1 && target.ruleSchema() != schema {
			return false
		}
	}
	return true
}

func (bound *boundRule[V, O]) appendReadRuntime(read readRuntime) bool {
	if bound == nil || bound.rule == nil || read == nil || len(bound.reads) >= len(bound.rule.reads) {
		return false
	}
	bound.reads = append(bound.reads, read)
	return true
}

// bindExactRead attaches one equation-issued exact surface to the type-only
// Read resolver retained by the cold Rule. It neither mutates Read nor leaks
// the bound Unit into Rule/Factor declaration state.
func bindExactRead[K ~uint32 | ~uint64, V, S any](bound readBinding, read Read[S], factor *boundFactor[K, V], surface equation.Surface, normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) bool {
	if bound == nil {
		return false
	}
	schema := bound.ruleSchema()
	if schema == nil || read.rule != schema || read.index < 0 || read.index >= len(schema.reads) || read.resolve == nil || factor == nil || normalize == nil || equal == nil || fingerprint == nil || surface.Form != equation.SurfaceReadExact {
		return false
	}
	declared := schema.reads[read.index]
	if declared.input != read.input.index || declared.form == nil || declared.form.factor != factor.factor.schema || declared.form.readKind != exactReadForm {
		return false
	}
	unit, ok := factor.readUnit(surface)
	if !ok {
		return false
	}
	proof, proved := newRuleReadProof(factor.factor.schema, surface)
	if !proved {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: declared.input, binding: factor.binding, unit: unit, proof: proof, normalize: normalize, equal: equal, fingerprint: fingerprint})
}

// bindSummaryRead installs a Factor-owned normalizer over one finite summary
// Unit. The concrete key set came from the compiled Graph catalog before
// attachment; the normalizer/equality witnesses remain with the typed cold
// ReadForm and never cross into carrier or demand.
func bindSummaryRead[K ~uint32 | ~uint64, V, S any](bound readBinding, read Read[S], factor *boundFactor[K, V], surface equation.Surface, form ReadForm[V, S]) bool {
	if bound == nil {
		return false
	}
	schema := bound.ruleSchema()
	if schema == nil || read.rule != schema || read.index < 0 || read.index >= len(schema.reads) || read.resolve == nil || factor == nil || !form.valid() || form.schema.readKind != summaryReadForm || surface.Form != equation.SurfaceReadSummary || !matchesFactorReadForm(factor.factor.schema, surface, summaryReadForm) {
		return false
	}
	declared := schema.reads[read.index]
	if declared.input != read.input.index || declared.form != form.schema || declared.form.factor != factor.factor.schema {
		return false
	}
	unit, ok := factor.readUnit(surface)
	if !ok {
		return false
	}
	proof, proved := factor.summaryReadProof(surface, form.schema)
	if !proved {
		return false
	}
	return bound.appendReadRuntime(&typedReadRuntime[K, V, S]{input: declared.input, binding: factor.binding, unit: unit, summary: proof, normalize: form.normalize, equal: form.equal, fingerprint: form.fingerprint})
}

// bindMemberExactRead binds an exact token through its graph-owned resolved
// surface. Callers cannot substitute a separate occurrence coordinate.
func bindMemberExactRead[K ~uint32 | ~uint64, V, S any](bound readBinding, member equation.RuleMember, read Read[S], factor *boundFactor[K, V], normalize func(OrderedCells[V]) S, equal func(S, S) bool, fingerprint func(S) uint64) bool {
	if bound == nil || read.index < 0 {
		return false
	}
	surface, ok := member.ReadAt(read.index)
	return ok && bindExactRead(bound, read, factor, surface, normalize, equal, fingerprint)
}

// bindMemberSummaryRead binds one Factor-owned summary token through the
// member surface at the token's declared ordinal.
func bindMemberSummaryRead[K ~uint32 | ~uint64, V, S any](bound readBinding, member equation.RuleMember, read Read[S], factor *boundFactor[K, V], form ReadForm[V, S]) bool {
	if bound == nil || read.index < 0 {
		return false
	}
	surface, ok := member.ReadAt(read.index)
	return ok && bindSummaryRead(bound, read, factor, surface, form)
}

// bindMemberStagedRead installs one sparse row-local exact selector. The
// topology contributes only the sealed target Factor occurrence surface; it
// cannot pre-enumerate candidates. The typed Factor target and typed
// normalizer stay behind stagedFactor and ReadForm respectively.
func bindMemberStagedRead[V, S any, Tag selectionTag](bound readBinding, member equation.RuleMember, read Read[Selection[Tag, S]], target stagedFactor[V], source ReadForm[V, S], selector *coldReadSelector, locate func(SelectorContext) bool) bool {
	if bound == nil || target == nil || selector == nil || locate == nil {
		return false
	}
	schema := bound.ruleSchema()
	if schema == nil || read.rule != schema || read.index < 0 || read.index >= len(schema.reads) || read.resolve == nil || !source.valid() || source.schema.readKind != exactReadForm || selector.read != read.index || len(selector.depends) == 0 || !sameDependencies(selector.depends, schema.reads[read.index].depends) {
		return false
	}
	declared := schema.reads[read.index]
	if declared.input != read.input.index || declared.form != source.schema || declared.form.factor == nil || source.schema.factor == nil || declared.form.factor != source.schema.factor {
		return false
	}
	for _, dependency := range selector.depends {
		if dependency.kind != readDependency || dependency.index < 0 || dependency.index >= read.index {
			return false
		}
	}
	surface, ok := member.ReadAt(read.index)
	if !ok || surface.Form != equation.SurfaceReadSelect || surface.Factor != source.schema.factor.semantic.compositionKey() ||
		surface.Semantic != surface.Factor || surface.Normalizer.Available() || surface.Mode != equation.TargetModeNone || surface.Local == 0 {
		return false
	}
	if _, slotOK := target.stagedSlot(); !slotOK {
		return false
	}
	return bound.appendReadRuntime(&stagedReadRuntime[V, S, Tag]{input: declared.input, selector: selector, target: target, locate: locate, normalize: source.normalize})
}

func coldReadSelectorForRead(rule *ruleSchema, read int) *coldReadSelector {
	if rule == nil || read < 0 {
		return nil
	}
	for index := range rule.readSelectors {
		selector := &rule.readSelectors[index]
		if selector.read == read {
			return selector
		}
	}
	return nil
}

func (bound *boundRule[V, O]) execute(work *carrier.Work, base carrier.ContributionBase, inputs []carrier.State, within support.Mask) (carrier.Patch, []demand.Observation, bool, bool, SolveFailurePhase) {
	if bound == nil || bound.rule == nil || bound.transfer == nil || work == nil || !work.OwnsContributionStates(base, inputs) {
		return carrier.Patch{}, nil, false, false, SolveFailurePhasePreflight
	}
	epoch := bound.nextEpoch.Add(1)
	if epoch == 0 {
		return carrier.Patch{}, nil, false, false, SolveFailurePhasePreflight
	}
	execution := &ruleExecution{owner: bound, work: work, base: base, inputs: append([]carrier.State(nil), inputs...), epoch: epoch}
	execution.active.Store(epoch)
	defer func() {
		if execution.output != nil {
			execution.output.discard()
		}
		if execution.product != nil {
			execution.product.close()
		}
		execution.active.CompareAndSwap(epoch, 0)
	}()
	product, ok := newProductSession(execution, bound.reads, work, inputs, within)
	if !ok {
		return carrier.Patch{}, nil, false, false, SolveFailurePhasePreflight
	}
	execution.product = product
	if !bound.carryOnly {
		execution.output = bound.output.begin(execution)
		if execution.output == nil {
			return carrier.Patch{}, nil, false, false, SolveFailurePhasePreflight
		}
	}
	access := Access[V, O]{execution: execution, owner: bound, epoch: epoch, output: bound.output}
	transferred := bound.transfer(access)
	if !product.checkpoint() {
		return carrier.Patch{}, nil, false, false, SolveFailurePhaseCheckpoint
	}
	if !transferred || execution.failed.Load() {
		return carrier.Patch{}, nil, false, false, SolveFailurePhaseTransfer
	}
	reads := product.observations()
	if !product.requireCheckpoint() {
		return carrier.Patch{}, nil, false, false, SolveFailurePhaseCheckpoint
	}
	if !product.started.Load() || !bound.carryOnly && !execution.output.complete() {
		return carrier.Patch{}, nil, false, false, SolveFailurePhaseCheckpoint
	}
	// An empty support intersection has no Product row and therefore no
	// semantic derivation or conclusion to admit.  It is a successful
	// structural no-op: retain the already-resolved read dependencies, but
	// mint no evidence, invoke no domain checker, allocate no Patch, and emit
	// no row/disposition coverage.  Nonempty Products continue through the
	// ordinary derivation/admission cut below.
	if product.rows.Count() == 0 {
		return carrier.Patch{}, reads, false, true, SolveFailurePhaseNone
	}
	derivation, ticket, derivationOK := bound.derivation(execution, reads)
	if !derivationOK {
		if !product.checkpoint() {
			return carrier.Patch{}, nil, false, false, SolveFailurePhaseCheckpoint
		}
		return carrier.Patch{}, nil, false, false, SolveFailurePhaseDerivation
	}
	defer ticket.invalidate()
	evidence, admitted := bound.admission.admit(derivation, bound.rule.composition, bound.rule)
	if !execution.product.requireCheckpoint() {
		return carrier.Patch{}, nil, false, false, SolveFailurePhaseCheckpoint
	}
	if !admitted {
		return carrier.Patch{}, nil, false, false, SolveFailurePhaseAdmission
	}
	if bound.carryOnly {
		if !evidence.consume() {
			return carrier.Patch{}, nil, false, false, SolveFailurePhasePublication
		}
		return carrier.Patch{}, reads, false, true, SolveFailurePhaseNone
	}
	if !execution.output.hasStaged() {
		// An explicit all-omitted Product is a valid empty successor, not a
		// sparse Default write. It therefore consumes its admission evidence
		// but publishes no Factor patch and cannot prune structural support.
		if !evidence.consume() {
			return carrier.Patch{}, nil, false, false, SolveFailurePhasePublication
		}
		execution.output.discard()
		execution.output = nil
		return carrier.Patch{}, reads, false, true, SolveFailurePhaseNone
	}
	patch, ok := execution.output.accept(&evidence)
	execution.output = nil
	if !ok {
		return patch, reads, true, false, SolveFailurePhasePublication
	}
	return patch, reads, true, true, SolveFailurePhaseNone
}

func (bound *boundRule[V, O]) derivation(execution *ruleExecution, reads []demand.Observation) (RuleDerivation[V, O], *ruleAdmissionTicket, bool) {
	if bound == nil || bound.rule == nil || !bound.admission.same(bound.rule.admission) || execution == nil || execution.owner != bound || !bound.carryOnly && execution.output == nil || execution.product == nil || !execution.product.requireCheckpoint() || execution.epoch == 0 || execution.active.Load() != execution.epoch || bound.anchor.Available() == false || bound.rule.composition == nil || !bound.rule.composition.Sealed() {
		return RuleDerivation[V, O]{}, nil, false
	}
	ticket := &ruleAdmissionTicket{rule: bound.rule, composition: bound.rule.composition.ID(), identity: bound.admission.identity, epoch: execution.epoch, anchor: bound.anchor, execution: execution, product: execution.product, live: true}
	// Trusted theorem admission has no checker-visible operands. Preserve all
	// runtime authority in its one live ticket without copying input, read, or
	// staged-result proof payloads.
	if !bound.requiresDerivation() {
		return RuleDerivation[V, O]{ticket: ticket}, ticket, true
	}
	inputs := make([]RuleInput, len(execution.inputs))
	for index, input := range execution.inputs {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[V, O]{}, nil, false
		}
		if !input.Valid() && !input.Same(execution.base.State()) {
			return RuleDerivation[V, O]{}, nil, false
		}
		inputs[index] = RuleInput{state: input}
	}
	proofReads := make([]RuleRead, len(reads))
	for index, read := range reads {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[V, O]{}, nil, false
		}
		proofReads[index] = RuleRead{input: read.Input, unit: read.Unit}
	}
	var dispositions []RuleDisposition[V]
	if bound.requiresDerivation() {
		if bound.carryOnly {
			dispositions = []RuleDisposition[V]{}
		} else {
			if bound.output.derivation == nil {
				return RuleDerivation[V, O]{}, nil, false
			}
			var okay bool
			dispositions, okay = bound.output.derivation(execution.output)
			if !okay {
				return RuleDerivation[V, O]{}, nil, false
			}
			if !validRuleDispositionCoverage(dispositions, len(execution.product.values)) {
				return RuleDerivation[V, O]{}, nil, false
			}
		}
	}
	for index := range dispositions {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[V, O]{}, nil, false
		}
		if dispositions[index].row.index != index || dispositions[index].row.index < 0 || dispositions[index].row.index >= len(execution.product.values) || !bound.validCarryDisposition(dispositions[index]) {
			return RuleDerivation[V, O]{}, nil, false
		}
		dispositions[index].row.ticket = ticket
		dispositions[index].ordinal = index
		for outputIndex := range dispositions[index].outputs {
			output := &dispositions[index].outputs[outputIndex]
			if output.ordinal != outputIndex || output.witness.ticket != nil {
				return RuleDerivation[V, O]{}, nil, false
			}
			output.witness = ruleRouteOutputWitness{ticket: ticket, row: index, ordinal: outputIndex}
		}
	}
	if !execution.product.requireCheckpoint() {
		return RuleDerivation[V, O]{}, nil, false
	}
	return RuleDerivation[V, O]{rule: bound.rule, composition: bound.rule.composition.ID(), identity: bound.admission.identity, epoch: execution.epoch, anchor: bound.anchor, operandContent: bound.operandContent, coordinates: bound.coordinates, inputs: inputs, reads: proofReads, dispositions: dispositions, product: execution.product, ticket: ticket, operand: bound.operand}, ticket, true
}

// validRuleDispositionCoverage makes a checker-visible derivation total over
// the Product that executed it: every row is represented once as either a
// staged result or an explicit no-candidate omission. A checker never has to
// infer whether a missing result was accidental.
func validRuleDispositionCoverage[V any](dispositions []RuleDisposition[V], rows int) bool {
	if rows < 0 || len(dispositions) != rows {
		return false
	}
	for index, disposition := range dispositions {
		if disposition.row.index != index || disposition.ordinal != index || (disposition.kind != RuleDispositionStaged && disposition.kind != RuleDispositionNoCandidate) {
			return false
		}
	}
	return true
}

// StageValue is the only typed output mutation capability. It cannot select a
// target, support region, predecessor, or Factor slot.
func StageValue[V, O any](access Access[V, O], row Row, value V) bool {
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || access.execution.active.Load() != access.epoch || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.stage == nil || !access.output.stage(access.execution, access.epoch, row.index, value) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// StageTransform settles one Product row by applying that Rule's sole
// declared transformed carry.  It is available only when TransformCarryFrom
// installed the Factor-owned map; callers cannot choose a map, target, or
// predecessor and it never encodes the effect as a sentinel value.
func StageTransform[V, O any](access Access[V, O], row Row) bool {
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || access.execution.active.Load() != access.epoch || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.stageTransform == nil || !access.output.stageTransform(access.execution, access.epoch, row.index) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// StageSelection is the sole route-output capability. It consumes every
// selected ordinal of exactly one preceding Selection and stages one atomic
// row batch of authenticated target/value pairs. A transfer cannot select a
// different Ref, omit a nonempty route, or send a value through the ordinary
// StageValue path.
func StageSelection[V, O any, Tag selectionTag, S any](access Access[V, O], row Row, selection Selection[Tag, S], derive func(Tag, S) (V, bool)) bool {
	if derive == nil || !validSelection(access, row, selection) || selection.count == nil || selection.at == nil || selection.route == nil {
		poisonSelection(access)
		return false
	}
	count, ok := selection.count(row.index)
	if !ok || count <= 0 {
		poisonSelection(access)
		return false
	}
	batch := routeOutputBatch[V]{
		read: selection.read, selectionID: selection.selectionID, count: count,
		at: func(ordinal int) (exactRef, V, bool) {
			var zero V
			tag, value, valueOK := selection.at(row.index, ordinal)
			ref, refOK := selection.route(row.index, ordinal)
			output, outputOK := derive(tag, value)
			if !valueOK || !refOK || ref == nil || !outputOK {
				return nil, zero, false
			}
			return ref, output, true
		},
	}
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || access.execution.active.Load() != access.epoch || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.stageSelection == nil || !access.output.stageSelection(access.execution, access.epoch, row.index, batch) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// NoSelection settles an explicitly empty route selection. It is separate
// from NoCandidate because the empty proof is the same authenticated
// Selection that would otherwise have carried all exact route targets.
func NoSelection[V, O any, Tag selectionTag, S any](access Access[V, O], row Row, selection Selection[Tag, S]) bool {
	if !validSelection(access, row, selection) || selection.count == nil || selection.route == nil {
		poisonSelection(access)
		return false
	}
	count, ok := selection.count(row.index)
	if !ok || count != 0 {
		poisonSelection(access)
		return false
	}
	batch := routeOutputBatch[V]{read: selection.read, selectionID: selection.selectionID, count: 0}
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || access.execution.active.Load() != access.epoch || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.noSelection == nil || !access.output.noSelection(access.execution, access.epoch, row.index, batch) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// NoCandidate settles one Product row with an explicit empty successor. It
// is not a sparse write of Default or Bottom: it publishes no Fact update.
// Like StageValue, it is row-, owner-, and epoch-fenced, and a row may take
// exactly one of the two dispositions.
func NoCandidate[V, O any](access Access[V, O], row Row) bool {
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || access.execution.active.Load() != access.epoch || access.execution.product == nil || row.session != access.execution.product || row.epoch != access.epoch || row.index != access.execution.product.current || access.output.noCandidate == nil || !access.output.noCandidate(access.execution, access.epoch, row.index) {
		if access.execution != nil {
			access.execution.failed.Store(true)
		}
		return false
	}
	return true
}

// Operand returns the typed immutable instance payload installed by the cold
// compiler. It is unavailable outside a live transfer frame and cannot be
// retained as a capability to a later solve.
func Operand[V, O any](access Access[V, O]) (O, bool) {
	var zero O
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || access.epoch == 0 || access.execution.active.Load() != access.epoch {
		return zero, false
	}
	return access.owner.operand, true
}

// Coordinates returns the accepted activation tuple for this live transfer.
// It is unavailable for ordinary Rule rows and after the transfer closes.
func Coordinates[V, O any](access Access[V, O]) (ActivationCoordinates, bool) {
	if access.execution == nil || access.owner == nil || access.execution.owner != access.owner || access.epoch == 0 || access.execution.active.Load() != access.epoch || !access.owner.coordinates.Available() {
		return ActivationCoordinates{}, false
	}
	return access.owner.coordinates, true
}

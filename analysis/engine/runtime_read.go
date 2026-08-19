// runtime_read.go implements the typed and staged read runtimes, the selection sessions and read materialization.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

type typedReadRuntime[K ~uint32 | ~uint64, V, S any] struct {
	input         int
	binding       *factbinding.Binding[K, V]
	unit          carrier.Unit
	exactFactor   factorRuntimeBinding
	exactRaw      uint64
	exact         bool
	summaryFactor factorRuntimeBinding
	summaryForm   factorFormReceipt
	summaryKeys   []uint64
	summaryDigest [32]byte
	summary       bool
	normalize     func(OrderedCells[V]) S
	equal         func(S, S) bool
	fingerprint   func(S) uint64
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

// stagedTargetProvider is the typed, owner-local projection needed by a
// heterogeneous selected-read binder. It carries no coordinate or mutable
// runtime state; the bound Factor remains the sole staged target authority.
type stagedTargetProvider[V any] interface {
	stagedFactorTarget() stagedFactor[V]
	stagedFactorReceiptMatches(*schemaRuleReadOrigin) bool
}

func (factor *boundFactor[K, V]) stagedFactorTarget() stagedFactor[V] { return factor }

func (factor *boundFactor[K, V]) stagedFactorReceiptMatches(origin *schemaRuleReadOrigin) bool {
	if factor == nil || origin == nil || !factor.receipt.valid() || factor.receipt.state != origin.state || factor.receipt.schema != origin.state.schema || factor.receipt.ordinal != origin.factor || factor.receipt.semantic != origin.state.schema.factorSemanticAt(origin.factor) {
		return false
	}
	return factor.receipt.authority == origin.state.authority
}

// stagedReadRuntime is one dynamic exact-read node. It has no candidate
// vector: the locator sees only declared predecessor observations and emits
// owner-issued Refs through SelectRoute. Physical Unit work is deduplicated
// per row while canonical numeric tags retain every semantic route.
type stagedReadRuntime[V, S any, Tag selectionTag] struct {
	input     int
	selector  stagedReadSelector
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

func (*stagedReadRuntime[V, S, Tag]) exactAddress() (factorRuntimeBinding, uint64, bool) {
	return factorRuntimeBinding{}, 0, false
}

func (*stagedReadRuntime[V, S, Tag]) summaryAddress() (factorRuntimeBinding, factorFormReceipt, []uint64, [32]byte, bool) {
	return factorRuntimeBinding{}, factorFormReceipt{}, nil, [32]byte{}, false
}

func (runtime *stagedReadRuntime[V, S, Tag]) refine(session *productSession, index int) bool {
	if runtime == nil || session == nil || session.execution == nil || session.work == nil || index < 0 || index >= len(session.reads) ||
		runtime.input < 0 || runtime.input >= len(session.inputs) || runtime.selector == nil || runtime.selector.selectorReadIndex() != index ||
		runtime.target == nil || runtime.locate == nil || runtime.normalize == nil || runtime.selector.selectorDependencyCount() == 0 ||
		len(session.values) != session.rows.Count() || index >= len(session.sessions) || session.sessions[index] != nil {
		return false
	}
	for dependency := 0; dependency < index; dependency++ {
		if runtime.selector.selectorDeclaresRead(dependency) && (!session.requireCheckpoint() || dependency >= len(session.reads) || session.reads[dependency] == nil) {
			return false
		}
	}
	selected := &typedStagedSelectionSession[V, S, Tag]{
		values: make([][]stagedSelectionValue[Tag, S], 0, len(session.values)),
		frame:  selectorFrame{execution: session.execution, epoch: session.execution.epoch, read: runtime.selector, product: session, row: -1},
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
		frame.row, frame.routes = source, &sink
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

func (runtime *typedReadRuntime[K, V, S]) exactAddress() (factorRuntimeBinding, uint64, bool) {
	if runtime == nil || !runtime.exact {
		return factorRuntimeBinding{}, 0, false
	}
	return runtime.exactFactor, runtime.exactRaw, true
}

func (runtime *typedReadRuntime[K, V, S]) summaryAddress() (factorRuntimeBinding, factorFormReceipt, []uint64, [32]byte, bool) {
	if runtime == nil || !runtime.summary {
		return factorRuntimeBinding{}, factorFormReceipt{}, nil, [32]byte{}, false
	}
	return runtime.summaryFactor, runtime.summaryForm, runtime.summaryKeys, runtime.summaryDigest, true
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
	if !session.execution.active.holds(epoch) {
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

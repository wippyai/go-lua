package factbinding

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// directObservationMode is Work-local traversal state.  It is intentionally
// payload-free in the carrier sense: the typed bindingWork owns the pieces,
// groups, and entry slab; this enum only selects which already-partitioned
// emission sequence the next call advances.
type directObservationMode uint8

const (
	directObservationDone directObservationMode = iota
	directObservationPieces
	directObservationGroups
)

// DirectObservationStatus is the terminal disposition of one cursor step.
// A bool is intentionally retained by Next for the original exact-reader
// callers, but the execution seam needs to distinguish a clean end of the
// canonical row sequence from a refused/stale row.
type DirectObservationStatus uint8

const (
	DirectObservationRefuse DirectObservationStatus = iota
	DirectObservationAvailable
	DirectObservationExhausted
)

// Valid reports whether status is one of the sealed cursor dispositions.
func (status DirectObservationStatus) Valid() bool { return status <= DirectObservationExhausted }

// DirectObservation is caller-owned scratch for one generation-bound exact or
// summary observation
// observation.  It is deliberately a data-only cursor: no visitor, function,
// interface, or semantic registry is retained here.  The typed Work pointer is
// retained only for this cursor's open generation and is cleared by Close.
//
// The cursor's emission path is shared with ObserveUnder. Exact Units use the
// existing one-key partition; Summary Units use the existing correlated or
// distributive summary fold and its grouped emission sequence.
type DirectObservation[K scalar.Key, V any] struct {
	work      *carrier.Work
	typedWork *bindingWork[K, V]
	slot      shape.Slot
	unit      carrier.Unit
	row       carrier.ObservationRow
	view      Observation[V]
	current   bool
	failed    bool
	live      bool
}

// BeginDirectObservation opens a callback-free observation generation over
// input's root at binding's attached slot.  The caller owns scratch and must
// close it after consuming Next.  A foreign Binding, Work, State, Unit, or
// support mask is rejected before the generation opens.
func (binding *Binding[K, V]) BeginDirectObservation(scratch *DirectObservation[K, V], work *carrier.Work, input carrier.State, unit carrier.Unit, within support.Mask) bool {
	if binding == nil || scratch == nil || scratch.live || work == nil || !binding.live() || binding.plane == nil || !work.OwnsState(input) || !within.Valid() || within.Manager() != binding.plane.domain.Guards() {
		return false
	}
	slot, ok := binding.issuer.Slot()
	if !ok || !binding.ValidUnit(unit) || (unit.Kind() != carrier.ExactUnit && unit.Kind() != carrier.SummaryUnit) {
		return false
	}
	descriptor, declared := binding.units[unit]
	if !declared || unit.Kind() == carrier.ExactUnit && (len(descriptor.keys) != 1 || descriptor.distributive) {
		return false
	}
	if !within.Entails(input.Support()) {
		return false
	}
	root, ok := input.HandleAt(slot)
	if !ok {
		return false
	}
	slotWork, ok := work.SlotWork(slot)
	typedWork, typed := slotWork.(*bindingWork[K, V])
	if !ok || !typed || typedWork == nil || typedWork.binding != binding || !typedWork.live() {
		return false
	}
	inputPlane, ok := typedWork.resolve(root)
	if !ok || !typedWork.BeginObservation() {
		return false
	}

	// A closed cursor's scalar scratch is cleared before reuse, so no row or
	// typed view from a prior generation can remain observable.
	scratch.clear()
	scratch.work = work
	scratch.typedWork = typedWork
	scratch.slot = slot
	scratch.unit = unit
	scratch.live = true

	// Prepare once into the Work's existing observation scratch. No row callback
	// is installed or invoked here; each Next asks bindingWork for one canonical
	// emitted row. The exact and summary forms share this state machine and the
	// existing factbinding partition/fold owners.
	var prepared bool
	if unit.Kind() == carrier.ExactUnit {
		prepared = typedWork.beginExactObservation(inputPlane, root, unit, descriptor.keys[0], within)
	} else {
		prepared = typedWork.beginSummaryObservation(inputPlane, root, unit, descriptor.keys, descriptor.distributive, within)
	}
	if !prepared {
		typedWork.EndObservation()
		scratch.clear()
		return false
	}
	return true
}

// BeginPointObservation opens the same canonical callback-free cursor as
// BeginDirectObservation, but authenticates the carrier PointState role before
// reducing it to its state header.  PointState is the only semantic transport
// result the carrier publishes; accepting its raw State alone would let a
// caller substitute an unrelated state from the same Work and weaken the G8
// transport proof.  The Factor still owns the typed Unit and resolves every
// emitted row, while carrier owns point/work/lineage admission.  This is a
// generic sealed transport primitive: it accepts only the Factor's issued Unit
// and cannot address a variable-cardinality domain relation, an attribute, or
// a branch outcome.  Those owner/provider declarations must remain separate
// from G8; no relation copy, root, semantic plane, or second transport path is
// opened here.
func (binding *Binding[K, V]) BeginPointObservation(scratch *DirectObservation[K, V], work *carrier.Work, point carrier.PointState, unit carrier.Unit, within support.Mask) bool {
	if binding == nil || work == nil || !work.OwnsPointState(point) || !point.Valid() {
		return false
	}
	return binding.BeginDirectObservation(scratch, work, point.State(), unit, within)
}

// beginSummaryObservation prepares the already-sealed summary fold for a
// callback-free cursor. Correlated and distributive summaries deliberately
// call the same owners used by ObserveUnder; this function only leaves their
// canonical groups in Work scratch for resumable emission.
func (work *bindingWork[K, V]) beginSummaryObservation(input semantic.Plane[planeFactor, K, V], root carrier.RootHandle, unit carrier.Unit, keys []K, distributive bool, within support.Mask) bool {
	if work == nil || !work.observationLive || !work.live() || !within.Valid() || work.binding == nil || work.binding.plane == nil || within.Manager() != work.binding.plane.domain.Guards() {
		return false
	}
	work.observationCursorMode = directObservationDone
	work.observationCursorIndex = 0
	work.observationCursorRoot = root
	work.observationCursorUnit = unit
	work.observationCursorKeys = work.declaredKeyBase(unit)
	if support.Empty(within) {
		return true
	}
	if distributive {
		work.resetSpine()
		cell, count := -1, 0
		for _, key := range keys {
			if !work.live() {
				return false
			}
			value, present, valid := work.binding.plane.domain.JoinUnderKey(input, key, within, &work.scratch)
			if !valid {
				return false
			}
			cell, count = work.appendCell(cell, ObservationEntry[V]{value: value, present: present}), count+1
		}
		work.partials = work.partials[:0]
		work.partials = append(work.partials, observationGroup{cell: cell, count: count, region: within, previous: -1})
		work.observationCursorMode = directObservationGroups
		return true
	}

	// A summary of constant (including absent) keys has one raw sequence over
	// all of within. Probe using the existing PartitionKey owner; if a key is
	// branched, restart that same owner from its first key and use the existing
	// grouping spine, exactly as ObserveUnder does.
	work.resetSpine()
	constant := true
	cell, count := -1, 0
	for index, key := range keys {
		if !work.live() {
			return false
		}
		if !work.partitionKey(input, key, index, within) {
			return false
		}
		if len(work.pieces) != 1 {
			constant = false
			break
		}
		cell, count = work.appendCell(cell, work.pieces[0].entry), count+1
	}
	if constant {
		work.partials = work.partials[:0]
		work.partials = append(work.partials, observationGroup{cell: cell, count: count, region: within, previous: -1})
		work.observationCursorMode = directObservationGroups
		return true
	}

	unions := work.newSupportWork()
	if unions == nil || len(keys) == 0 || !work.partitionKey(input, keys[0], 0, within) || !work.seedGroups(unions) {
		if unions != nil {
			unions.Discard()
		}
		return false
	}
	for offset, key := range keys[1:] {
		if !work.live() || !work.partitionKey(input, key, offset+1, within) || !work.extendGroups(unions, within) {
			unions.Discard()
			return false
		}
	}
	if !unions.Seal() {
		unions.Discard()
		return false
	}
	work.observationCursorMode = directObservationGroups
	return true
}

// emitDirectObservations is the callback adapter over the cursor's one
// prepared state machine. It preserves ObserveUnder's visitor contract while
// ensuring direct and callback paths share partitioning and row emission.
func (work *bindingWork[K, V]) emitDirectObservations(visit func(carrier.ObservationRow) bool) bool {
	if work == nil || visit == nil {
		return false
	}
	for {
		row, _, ok := work.nextDirectObservation()
		if !ok {
			return !work.observationCursorFailed
		}
		if !visit(row) {
			return false
		}
	}
}

// beginExactObservation prepares the exact one-key partition and selects its
// canonical emission sequence.  Partitioning and equal-entry coalescing are
// exactly the operations historically owned by observeExact; only the final
// emission is now resumable.
func (work *bindingWork[K, V]) beginExactObservation(input semantic.Plane[planeFactor, K, V], root carrier.RootHandle, unit carrier.Unit, key K, within support.Mask) bool {
	if work == nil || !work.observationLive || !work.live() || !within.Valid() || work.binding == nil || work.binding.plane == nil || within.Manager() != work.binding.plane.domain.Guards() {
		return false
	}
	work.observationCursorMode = directObservationDone
	work.observationCursorIndex = 0
	work.observationCursorRoot = root
	work.observationCursorUnit = unit
	work.observationCursorKeys = work.declaredKeyBase(unit)
	if support.Empty(within) {
		return true
	}
	if !work.partitionKey(input, key, 0, within) || len(work.pieces) == 0 {
		return false
	}
	// A single or pairwise-distinct partition is already in canonical order.
	// Equal pieces require the existing coalescing spine and support shell.
	if len(work.pieces) == 1 || work.exactPiecesDistinct() {
		work.observationCursorMode = directObservationPieces
		return true
	}
	unions := work.newSupportWork()
	if unions == nil || !work.seedGroups(unions) || !unions.Seal() {
		if unions != nil {
			unions.Discard()
		}
		return false
	}
	work.observationCursorMode = directObservationGroups
	return true
}

// nextDirectObservation emits exactly one row from the state prepared by
// beginExactObservation and resolves its typed Observation at the same
// generation cut.  It retains no callback and never materializes a caller
// row slice.
func (work *bindingWork[K, V]) nextDirectObservation() (carrier.ObservationRow, Observation[V], bool) {
	if work == nil {
		return carrier.ObservationRow{}, Observation[V]{}, false
	}
	if !work.observationLive || !work.live() {
		work.observationCursorFailed = true
		return carrier.ObservationRow{}, Observation[V]{}, false
	}
	var row carrier.ObservationRow
	var ok bool
	switch work.observationCursorMode {
	case directObservationPieces:
		if work.observationCursorIndex >= len(work.pieces) {
			return carrier.ObservationRow{}, Observation[V]{}, false
		}
		piece := work.pieces[work.observationCursorIndex]
		work.observationCursorIndex++
		row, ok = work.emitExactPiece(work.observationCursorRoot, work.observationCursorUnit, piece)
	case directObservationGroups:
		if work.observationCursorIndex >= len(work.partials) {
			return carrier.ObservationRow{}, Observation[V]{}, false
		}
		group := work.partials[work.observationCursorIndex]
		work.observationCursorIndex++
		row, ok = work.emitGroup(work.observationCursorRoot, work.observationCursorUnit, group)
	default:
		return carrier.ObservationRow{}, Observation[V]{}, false
	}
	if !ok {
		work.observationCursorFailed = true
		return carrier.ObservationRow{}, Observation[V]{}, false
	}
	view, resolved := work.binding.ResolveObservation(work, row)
	if !resolved {
		work.observationCursorFailed = true
		return carrier.ObservationRow{}, Observation[V]{}, false
	}
	return row, view, true
}

func (work *bindingWork[K, V]) directObservationLive() bool {
	return work != nil && work.observationLive && work.live()
}

func (work *bindingWork[K, V]) directObservationCount() int {
	if !work.directObservationLive() {
		return 0
	}
	return len(work.records)
}

// Next returns the next emitted guarded row and its typed Observation view.
// Both values are generation-bound; after Close, or after a foreign/stale
// generation revokes the row, Next refuses instead of exposing recycled data.
func (observation *DirectObservation[K, V]) Next() (carrier.ObservationRow, Observation[V], bool) {
	row, view, status := observation.Step()
	return row, view, status == DirectObservationAvailable
}

// Step advances the cursor and distinguishes a clean end from a refused
// step. A refused step never exposes a row or a typed Observation view.
func (observation *DirectObservation[K, V]) Step() (carrier.ObservationRow, Observation[V], DirectObservationStatus) {
	if observation == nil || !observation.live || observation.work == nil || observation.typedWork == nil || !observation.work.Checkpoint() || observation.failed {
		return carrier.ObservationRow{}, Observation[V]{}, DirectObservationRefuse
	}
	if !observation.typedWork.directObservationLive() {
		observation.failed = true
		return carrier.ObservationRow{}, Observation[V]{}, DirectObservationRefuse
	}
	if observation.typedWork.observationCursorFailed {
		observation.failed = true
		return carrier.ObservationRow{}, Observation[V]{}, DirectObservationRefuse
	}
	if observation.typedWork.observationCursorExhausted() {
		return carrier.ObservationRow{}, Observation[V]{}, DirectObservationExhausted
	}
	row, view, ok := observation.typedWork.nextDirectObservation()
	if !ok || !view.Valid() || !row.Region().Valid() {
		observation.failed = true
		return carrier.ObservationRow{}, Observation[V]{}, DirectObservationRefuse
	}
	observation.row = row
	observation.view = view
	observation.current = true
	return row, view, DirectObservationAvailable
}

func (work *bindingWork[K, V]) observationCursorExhausted() bool {
	if work == nil || work.observationCursorFailed {
		return false
	}
	switch work.observationCursorMode {
	case directObservationPieces:
		return work.observationCursorIndex >= len(work.pieces)
	case directObservationGroups:
		return work.observationCursorIndex >= len(work.partials)
	default:
		return true
	}
}

// Advance moves to the next row and reports only whether one was available.
// Current returns the pair selected by the most recent successful Advance.
// These methods are conveniences for consumers that prefer a bool cursor loop
// while Next remains the allocation-free tuple form.
func (observation *DirectObservation[K, V]) Advance() bool {
	if observation == nil || !observation.live {
		return false
	}
	_, _, ok := observation.Next()
	return ok
}

// Current returns the row and typed view selected by the most recent
// successful Next or Advance call.
func (observation *DirectObservation[K, V]) Current() (carrier.ObservationRow, Observation[V], bool) {
	if observation == nil || !observation.live || !observation.current {
		return carrier.ObservationRow{}, Observation[V]{}, false
	}
	if !observation.view.Valid() || !observation.row.Region().Valid() {
		return carrier.ObservationRow{}, Observation[V]{}, false
	}
	return observation.row, observation.view, true
}

// Close revokes every row emitted by this cursor's generation and returns
// whether the owning SlotWork accepted the close.  A stale or double close
// refuses and does not reopen the observation generation.
func (observation *DirectObservation[K, V]) Close() bool {
	if observation == nil || !observation.live || observation.work == nil {
		return false
	}
	work := observation.work
	closed := work.Checkpoint() && observation.typedWork != nil && observation.typedWork.EndObservation()
	observation.clear()
	return closed
}

// Valid reports whether this cursor still owns an open observation
// generation. It also checks the current row when one has been selected, so a
// foreign EndObservation cannot leave a seemingly live cursor usable.
func (observation *DirectObservation[K, V]) Valid() bool {
	if observation == nil || !observation.live || observation.work == nil || observation.typedWork == nil || !observation.work.Checkpoint() {
		return false
	}
	if !observation.typedWork.directObservationLive() {
		return false
	}
	if observation.current {
		_, _, ok := observation.Current()
		return ok
	}
	return true
}

// Count reports the number of rows emitted by the open generation. Stale and
// closed cursors report zero.
func (observation *DirectObservation[K, V]) Count() int {
	if observation == nil || !observation.Valid() {
		return 0
	}
	return observation.typedWork.directObservationCount()
}

// Row returns the current guarded row selected by Next or Advance.
func (observation *DirectObservation[K, V]) Row() (carrier.ObservationRow, bool) {
	row, _, ok := observation.Current()
	return row, ok
}

// View returns the current typed Observation selected by Next or Advance.
func (observation *DirectObservation[K, V]) View() (Observation[V], bool) {
	_, view, ok := observation.Current()
	return view, ok
}

func (observation *DirectObservation[K, V]) clear() {
	if observation == nil {
		return
	}
	observation.work = nil
	observation.typedWork = nil
	observation.slot = 0
	observation.unit = carrier.Unit{}
	observation.row = carrier.ObservationRow{}
	observation.view = Observation[V]{}
	observation.current = false
	observation.failed = false
	observation.live = false
}

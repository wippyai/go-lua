package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// materialize executes a typed schema projection against one runtimeProgram
// factor handle. The program resolves the handle and unit once; execution has
// no query owner or receipt to authenticate.
func (projection factorProjection[V, R]) materialize(work *carrier.Work, state carrier.State, factor runtimeFactor, unit carrier.Unit) (frozenValue, solveBoundary, bool) {
	typed, ok := factor.(interface {
		stagedObserveWithFailure(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[V], support.Mask) bool) (stagedObservationFailure, bool)
	})
	if !ok || typed == nil || !projection.valid() || work == nil || !work.Checkpoint() {
		return nil, refused(SolveFailureFamilyObservation, "preflight"), false
	}
	var value R
	if projection.begin != nil {
		value = projection.begin()
	}
	observations := 0
	projectionFailure := boundaryNone
	visit := func(observation factbinding.Observation[V], _ support.Mask) bool {
		cells, ok := orderedCellsFromObservation(observation, projection.borrowIssued && projection.accumulate != nil)
		if !ok {
			projectionFailure = refused(SolveFailureFamilyObservation, "shape")
			return false
		}
		observations++
		if projection.accumulate != nil {
			next, ok := projection.accumulate(value, cells)
			if !ok {
				projectionFailure = refused(SolveFailureFamilyObservation, "projection")
				return false
			}
			value = next
			if !work.Checkpoint() {
				projectionFailure = refused(SolveFailureFamilyObservation, "preflight")
				return false
			}
			return true
		}
		value = projection.project(cells)
		if !work.Checkpoint() {
			projectionFailure = refused(SolveFailureFamilyObservation, "preflight")
			return false
		}
		return true
	}
	failure, valid := typed.stagedObserveWithFailure(work, state, unit, state.Support(), visit)
	if !valid || projection.accumulate == nil && observations > 1 || !work.Checkpoint() {
		boundary := refused(SolveFailureFamilyObservation, "projection")
		if projectionFailure != boundaryNone {
			boundary = projectionFailure
		} else if failure == stagedObservationFailureArguments {
			boundary = refused(SolveFailureFamilyObservation, "preflight")
		} else if failure == stagedObservationFailureCheckpoint || failure == stagedObservationFailureSlot || failure == stagedObservationFailureWork {
			boundary = refused(SolveFailureFamilyObservation, "work")
		} else if failure == stagedObservationFailureUnit {
			boundary = refused(SolveFailureFamilyObservation, "unit")
		} else if failure == stagedObservationFailureSupport {
			boundary = refused(SolveFailureFamilyObservation, "support")
		} else if failure == stagedObservationFailureRoot {
			boundary = refused(SolveFailureFamilyObservation, "root")
		} else if failure == stagedObservationFailureCarrier {
			boundary = refused(SolveFailureFamilyObservation, "carrier")
		} else if failure == stagedObservationFailureDecode {
			boundary = refused(SolveFailureFamilyObservation, "decode")
		}
		return nil, boundary, false
	}
	if observations == 0 && projection.begin == nil {
		// A Project callback has no lawful empty-row contract. Empty
		// observations must be represented by a typed Begin fold state.
		return nil, refused(SolveFailureFamilyObservation, "shape"), false
	}
	frozen := value
	if !projection.transferResult || projection.accumulate == nil {
		frozen = projection.result.Freeze(value)
	}
	if !work.Checkpoint() {
		return nil, refused(SolveFailureFamilyObservation, "freeze"), false
	}
	return &typedFrozenValue[R]{value: frozen, freeze: projection.result}, boundaryNone, true
}

// observationCellsView borrows the Binding-issued sequence for the duration
// of its synchronous callback. Observation itself owns the generation fence;
// this adapter neither copies nor extends it.
type observationCellsView[V any] struct{ observation factbinding.Observation[V] }

func (view observationCellsView[V]) Count() int { return view.observation.Count() }

func (view observationCellsView[V]) At(index int) (V, bool, bool) {
	entry, ok := view.observation.At(index)
	if !ok {
		var zero V
		return zero, false, false
	}
	value, present := entry.Read()
	return value, present, true
}

func orderedCellsFromObservation[V any](observation factbinding.Observation[V], borrow bool) (OrderedCells[V], bool) {
	if !observation.Valid() {
		return OrderedCells[V]{}, false
	}
	if borrow {
		return OrderedCells[V]{view: observationCellsView[V]{observation: observation}}, true
	}
	cells := make([]summaryCell[V], observation.Count())
	for index := range cells {
		entry, ok := observation.At(index)
		if !ok {
			return OrderedCells[V]{}, false
		}
		value, present := entry.Read()
		cells[index] = summaryCell[V]{value: value, present: present}
	}
	record := newOrderedCellsRecord(cells)
	return OrderedCells[V]{record: record}, true
}

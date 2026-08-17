package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// receiptQueryRuntime is the common solver query implementation for both
// catalog families. The only difference between Value summary and Effect
// exact is the sealed Factor surface carried by the typed implementation.
type receiptQueryFactor[V any] interface {
	runtimeFactor
	receiptMatches(*schemaBindingState, *schemaBindingAuthority, uint64, composition.Key) bool
	stagedObserve(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[V], support.Mask) bool) bool
}

type detailedReceiptQueryFactor[V any] interface {
	receiptQueryFactor[V]
	stagedObserveWithFailure(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[V], support.Mask) bool) (stagedObservationFailure, bool)
}

type receiptQueryRuntime[V, R any] struct {
	identity equation.Query
	owner    *receiptQueryOwner
	factor   receiptQueryFactor[V]
	surface  equation.Surface
	unit     carrier.Unit
	project  func(OrderedCells[V]) R
	begin    func() R
	accum    func(R, OrderedCells[V]) (R, bool)
	result   FrozenResult[R]
}

func (runtime *receiptQueryRuntime[V, R]) query() equation.Query {
	if runtime == nil {
		return equation.Query{}
	}
	return runtime.identity
}

func (runtime *receiptQueryRuntime[V, R]) queryOwner() queryOwner {
	if runtime == nil {
		return nil
	}
	return runtime.owner
}

func (runtime *receiptQueryRuntime[V, R]) materialize(work *carrier.Work, state carrier.State) (*queryResult, bool) {
	if runtime == nil || runtime.owner == nil || runtime.factor == nil || (runtime.project == nil && (runtime.begin == nil || runtime.accum == nil)) || !validFrozenResult(runtime.result) || work == nil || !work.Checkpoint() || runtime.owner.state == nil || runtime.owner.authority == nil || runtime.owner.state.phase != schemaBindingSealed || runtime.owner.state.authority != runtime.owner.authority {
		return nil, false
	}
	frozen, ok := materializeReceiptProjection(work, state, runtime.owner.state, runtime.owner.authority, runtime.factor, runtime.unit, runtime.project, runtime.begin, runtime.accum, runtime.result)
	if !ok {
		return nil, false
	}
	return &queryResult{owner: runtime.owner, key: runtime.identity.Key(), value: frozen}, true
}

// materializeReceiptProjection is the sole typed execution path for both
// committed graph Queries and optional solve-local observations. The latter
// changes only when the projection is requested; Factor reads, correlation,
// freezing, and checkpoint discipline remain identical.
func materializeReceiptProjection[V, R any](work *carrier.Work, state carrier.State, binding *schemaBindingState, authority *schemaBindingAuthority, factor receiptQueryFactor[V], unit carrier.Unit, project func(OrderedCells[V]) R, begin func() R, accum func(R, OrderedCells[V]) (R, bool), result FrozenResult[R]) (frozenValue, bool) {
	value, _, ok := materializeReceiptProjectionWithFailure(work, state, binding, authority, factor, unit, project, begin, accum, result)
	return value, ok
}

func materializeReceiptProjectionWithFailure[V, R any](work *carrier.Work, state carrier.State, binding *schemaBindingState, authority *schemaBindingAuthority, factor receiptQueryFactor[V], unit carrier.Unit, project func(OrderedCells[V]) R, begin func() R, accum func(R, OrderedCells[V]) (R, bool), result FrozenResult[R]) (frozenValue, solveBoundary, bool) {
	if factor == nil || (project == nil && (begin == nil || accum == nil)) || !validFrozenResult(result) || work == nil || !work.Checkpoint() || binding == nil || authority == nil || binding.phase != schemaBindingSealed || binding.authority != authority {
		return nil, refused(SolveFailureFamilyObservation, "preflight"), false
	}
	var value R
	if begin != nil {
		value = begin()
	}
	observations := 0
	projectionFailure := boundaryNone
	visit := func(observation factbinding.Observation[V], _ support.Mask) bool {
		cells, ok := orderedCellsFromObservation(observation)
		if !ok {
			projectionFailure = refused(SolveFailureFamilyObservation, "shape")
			return false
		}
		observations++
		if accum != nil {
			next, ok := accum(value, cells)
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
		value = project(cells)
		if !work.Checkpoint() {
			projectionFailure = refused(SolveFailureFamilyObservation, "preflight")
			return false
		}
		return true
	}
	valid := false
	failure := stagedObservationFailureNone
	if detailed, ok := factor.(detailedReceiptQueryFactor[V]); ok {
		failure, valid = detailed.stagedObserveWithFailure(work, state, unit, state.Support(), visit)
	} else {
		valid = factor.stagedObserve(work, state, unit, state.Support(), visit)
	}
	if !valid || (accum == nil && observations > 1) || !work.Checkpoint() {
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
	if observations == 0 && begin == nil {
		// A Project callback has no lawful empty-row contract. Empty
		// observations must be represented by a typed Begin fold state.
		return nil, refused(SolveFailureFamilyObservation, "shape"), false
	}
	frozen := result.Freeze(value)
	if !work.Checkpoint() {
		return nil, refused(SolveFailureFamilyObservation, "freeze"), false
	}
	return &typedFrozenValue[R]{value: frozen, freeze: result}, boundaryNone, true
}

func orderedCellsFromObservation[V any](observation factbinding.Observation[V]) (OrderedCells[V], bool) {
	if !observation.Valid() {
		return OrderedCells[V]{}, false
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

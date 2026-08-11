package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/facts/support"

// openSupports opens the immutable relation-support layout.  One slot belongs
// to one exact active relation; discovery of a new relation restarts the
// finite epoch so no runtime map becomes a second semantic graph.
func (transaction *transaction) openSupports() bool {
	if transaction == nil || transaction.solver == nil || transaction.guards == nil {
		return false
	}
	transaction.supports = make([]support.Mask, len(transaction.solver.supportCatalog))
	empty, ok := support.FromGuard(transaction.guards, transaction.guards.False())
	if !ok {
		return false
	}
	for index, binding := range transaction.solver.supportCatalog {
		if !transaction.solver.validActiveRelation(binding.relation) {
			return false
		}
		transaction.supports[index] = empty
	}
	return true
}

// beginActivation borrows the immutable selector/support tables compiled for
// this action. Only the replacement guards are transaction scratch. The
// scratch resets to false each execution, so withdrawn support disappears
// without a parallel activation store.
func (transaction *transaction) beginActivation(action *compiledAction) bool {
	if transaction == nil || transaction.canceled() || action == nil || transaction.activationOpen {
		return false
	}
	transaction.activation = action.selectors
	transaction.activationOutputs = action.supports
	work := support.New(transaction.guards)
	if work == nil {
		return false
	}
	if cap(transaction.activationNext) < len(transaction.activationOutputs) {
		transaction.activationNext = make([]support.Mask, len(transaction.activationOutputs))
	} else {
		transaction.activationNext = transaction.activationNext[:len(transaction.activationOutputs)]
	}
	for index, output := range transaction.activationOutputs {
		if transaction.canceled() {
			transaction.activation = nil
			transaction.activationOutputs = nil
			work.Discard()
			return false
		}
		if output.slot < 0 || output.slot >= len(transaction.supports) || !transaction.selectorActive(output.relation.source) {
			// An invalid compiled layout fails closed before activation opens.
			// Drop borrowed references so transaction abort cannot mistake them
			// for transaction-owned scratch.
			transaction.activation = nil
			transaction.activationOutputs = nil
			work.Discard()
			return false
		}
		transaction.activationNext[index] = work.False()
	}
	transaction.activationWork = work
	transaction.activationOpen = true
	return true
}

func (transaction *transaction) selectorActive(source activationSource) bool {
	if transaction == nil || transaction.solver == nil || source.rule == nil || !source.caller.Valid() {
		return false
	}
	low, high := 0, len(transaction.activation)
	for low < high {
		middle := low + (high-low)/2
		order, valid := transaction.solver.compareActivationSource(transaction.activation[middle], source)
		if !valid {
			return false
		}
		switch {
		case order < 0:
			low = middle + 1
		case order > 0:
			high = middle
		default:
			return true
		}
	}
	return false
}

// acceptsActivationSource is deliberately stricter than selectorActive: a
// selector is usable only while its owning action is executing.  This keeps
// relation discovery callback-scoped and prevents a retained Access from
// opening a new relation between actions.
func (transaction *transaction) acceptsActivationSource(source activationSource) bool {
	return transaction != nil && transaction.activationOpen && transaction.selectorActive(source)
}

// recordRelation updates an already compiled exact relation support.  A new
// relation is structural growth, never an auxiliary evaluator relation.
func (transaction *transaction) recordRelation(relation activeRelation, when support.Mask) bool {
	if transaction == nil || transaction.canceled() || transaction.guards == nil || transaction.activationWork == nil || !transaction.activationOpen || !transaction.solver.validActiveRelation(relation) || !transaction.selectorActive(relation.source) || !when.Valid() || when.Manager() != transaction.guards {
		return false
	}
	low, high := 0, len(transaction.activationOutputs)
	for low < high {
		middle := low + (high-low)/2
		output := transaction.activationOutputs[middle]
		order := transaction.solver.compareActiveRelation(output.relation, relation)
		switch {
		case order < 0:
			low = middle + 1
		case order > 0:
			high = middle
		default:
			next, ok := transaction.activationWork.Or(transaction.activationNext[middle], when)
			if !ok {
				return false
			}
			transaction.activationNext[middle] = next
			return true
		}
	}
	// An unknown relation cannot contribute until the next carrier epoch, so
	// its support is deliberately not accumulated here.  Keep the exact
	// discovery occurrence as scratch and canonicalize the complete batch at
	// the epoch boundary.  Looking through prior discoveries here made D
	// distinct resolutions cost quadratic time without adding semantics.
	// The resolver's operands are transaction scratch. A new relation alone
	// crosses the epoch boundary, so make its one durable copy here rather
	// than allocating a tuple for every already-compiled activation.
	relation.inputs = append([]termOrigin(nil), relation.inputs...)
	transaction.discovered = append(transaction.discovered, relation)
	transaction.rebuild = true
	return true
}

func (transaction *transaction) finishActivation(action *compiledAction) bool {
	if transaction == nil || transaction.canceled() || transaction.guards == nil || action == nil || transaction.activationWork == nil || !transaction.activationOpen || len(transaction.activationOutputs) != len(transaction.activationNext) {
		return false
	}
	defer func() {
		if transaction.activationWork != nil {
			transaction.activationWork.Discard()
		}
		transaction.activationWork = nil
		clear(transaction.activationNext)
		// selector/support slices borrow compiledAction storage. Never clear
		// them here: that would mutate the immutable compiled graph.
		transaction.activation = nil
		transaction.activationOutputs = nil
		transaction.activationNext = transaction.activationNext[:0]
		transaction.activationOpen = false
	}()
	if !transaction.activationWork.Seal() {
		return false
	}
	for index, output := range transaction.activationOutputs {
		if transaction.canceled() {
			return false
		}
		if output.slot < 0 || output.slot >= len(transaction.supports) || !transaction.activationNext[index].Valid() {
			return false
		}
		before, after := transaction.supports[output.slot], transaction.activationNext[index]
		transaction.supports[output.slot] = after
		if before.Equal(after) {
			continue
		}
		if output.slot >= len(transaction.solver.supportTargets) {
			return false
		}
		target := transaction.solver.supportTargets[output.slot]
		if target < 0 || target >= len(transaction.dirty) {
			return false
		}
		transaction.dirty[target] = true
	}
	return true
}

func (transaction *transaction) relationSupport(relation compiledRelation) (support.Mask, bool) {
	if transaction == nil || transaction.guards == nil || relation.support < 0 || relation.support >= len(transaction.supports) {
		return support.Mask{}, false
	}
	value := transaction.supports[relation.support]
	return value, value.Valid() && value.Manager() == transaction.guards
}

func (transaction *transaction) nextRelationEpoch() uint64 {
	if transaction == nil || transaction.relationEpoch == ^uint64(0) {
		return 0
	}
	transaction.relationEpoch++
	return transaction.relationEpoch
}

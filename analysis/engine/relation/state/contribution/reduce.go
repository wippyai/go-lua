package contribution

// Reducer consumes one exact output target and all of its producer rows. Returning
// false stops the walk; it does not invalidate the immutable state.
type Reducer interface {
	ReduceTarget(Target, []Row) bool
}

// ReducerFunc adapts a function to Reducer.
type ReducerFunc func(Target, []Row) bool

// ReduceTarget implements ReducerFunc.
func (function ReducerFunc) ReduceTarget(target Target, rows []Row) bool {
	return function(target, rows)
}

// Reduce visits every exact output target in deterministic order and supplies
// all producer rows for that target in deterministic contribution-key order.
// Rows are defensively copied before each callback.
func (state State) Reduce(reducer Reducer) bool {
	if !state.Available() || reducer == nil {
		return false
	}
	return state.reduceTargets(state.Targets(), reducer)
}

// ReduceAffected visits only the targets named by delta, using the delta
// successor as the reduction root. The delta must be rooted at state; this
// prevents a caller from applying an affected set to an unrelated fork.
func (state State) ReduceAffected(delta Delta, reducer Reducer) bool {
	if !state.Available() || !delta.Available() || !state.Same(delta.Next()) || reducer == nil {
		return false
	}
	return state.reduceTargets(delta.affected, reducer)
}

// ReduceAffectedTargets is the callback convenience form of
// ReduceAffected. It keeps the reduction interface usable without a wrapper
// type while preserving the same fencing and deterministic-order checks.
func (state State) ReduceAffectedTargets(delta Delta, visit func(Target, []Row) bool) bool {
	if visit == nil {
		return false
	}
	return state.ReduceAffected(delta, ReducerFunc(visit))
}

func (state State) reduceTargets(targets []Target, reducer Reducer) bool {
	for _, target := range targets {
		rows := state.RowsFor(target)
		if !reducer.ReduceTarget(target, append([]Row(nil), rows...)) {
			return false
		}
	}
	return true
}

package call

// DispatchRoute is one Call-owned member of the selected dispatch relation.
// The target selector remains private to Call: the engine transports only the
// opaque predicate returned by Predicate, and the candidate that consumes it
// re-authenticates it against the same Algebra before constructing a fact.
type DispatchRoute struct {
	owner       *Algebra
	key         Key
	selector    selector
	disposition dispatchDisposition
}

type dispatchDisposition uint8

const (
	dispatchDispositionInvalid dispatchDisposition = iota
	dispatchDispositionTarget
	dispatchDispositionOpaque
	dispatchDispositionTop
)

const dispatchPredicateDispositionBits = 2

// DispatchTargetRoute issues one route for an exact target alternative.
func (algebra *Algebra) DispatchTargetRoute(key Key, target Target) (DispatchRoute, bool) {
	if algebra == nil || !algebra.validKey(key) || !algebra.OwnsTarget(target) || !algebra.contains(key, target.selector) {
		return DispatchRoute{}, false
	}
	return DispatchRoute{owner: algebra, key: key, selector: target.selector, disposition: dispatchDispositionTarget}, true
}

// DispatchOpaqueRoute issues the one route representing an unresolved
// callable alternative. Opaque is a positive semantic disposition, never a
// fallback for missing evidence.
func (algebra *Algebra) DispatchOpaqueRoute(key Key) (DispatchRoute, bool) {
	if algebra == nil || !algebra.validKey(key) || !algebra.OpaqueAdmitted(key) {
		return DispatchRoute{}, false
	}
	return DispatchRoute{owner: algebra, key: key, disposition: dispatchDispositionOpaque}, true
}

// DispatchTopRoute issues the exact top disposition for a top callee fact.
func (algebra *Algebra) DispatchTopRoute(key Key) (DispatchRoute, bool) {
	if algebra == nil || !algebra.validKey(key) {
		return DispatchRoute{}, false
	}
	return DispatchRoute{owner: algebra, key: key, disposition: dispatchDispositionTop}, true
}

func (route DispatchRoute) valid() bool {
	if route.owner == nil || !route.owner.validKey(route.key) {
		return false
	}
	switch route.disposition {
	case dispatchDispositionTarget:
		target, ok := route.owner.targetForSelector(route.selector)
		return ok && route.owner.contains(route.key, target.selector)
	case dispatchDispositionOpaque:
		return !route.selector.valid() && route.owner.OpaqueAdmitted(route.key)
	case dispatchDispositionTop:
		return !route.selector.valid()
	default:
		return false
	}
}

// Coordinates returns the selected Call key and the output destination. They
// are the same owner-issued coordinate because dispatch refines the fact at
// the mounted application's own Call cell.
func (route DispatchRoute) Coordinates() (selected Key, destination Key, ok bool) {
	if !route.valid() {
		return Key{}, Key{}, false
	}
	return route.key, route.key, true
}

// Predicate returns the compact owner-local tag transported by the selected
// read. It is not a durable identity and is meaningful only when replayed by
// a CallCoordinate from this same sealed Algebra.
func (route DispatchRoute) Predicate() (uint64, bool) {
	if !route.valid() {
		return 0, false
	}
	return uint64(route.selector)<<dispatchPredicateDispositionBits | uint64(route.disposition), true
}

// DispatchValueForPredicate consumes one Call-owned route tag. The mounted
// candidate supplies both the destination key and the exact Algebra fence, so
// a foreign, forged, or stale tag cannot publish a Call fact.
func (row CallCoordinate) DispatchValueForPredicate(predicate uint64) (Value, bool) {
	key, keyOK := row.Key()
	if !keyOK {
		return Value{}, false
	}
	return row.owner.dispatchValueForPredicate(key, predicate)
}

func (algebra *Algebra) dispatchValueForPredicate(key Key, predicate uint64) (Value, bool) {
	if algebra == nil || !algebra.validKey(key) || predicate == 0 {
		return Value{}, false
	}
	disposition := dispatchDisposition(predicate & ((1 << dispatchPredicateDispositionBits) - 1))
	encodedSelector := selector(predicate >> dispatchPredicateDispositionBits)
	switch disposition {
	case dispatchDispositionTarget:
		target, ok := algebra.targetForSelector(encodedSelector)
		if !ok {
			return Value{}, false
		}
		return algebra.DispatchValue(key, []Target{target}, false)
	case dispatchDispositionOpaque:
		if encodedSelector.valid() {
			return Value{}, false
		}
		return algebra.DispatchValue(key, nil, true)
	case dispatchDispositionTop:
		if encodedSelector.valid() {
			return Value{}, false
		}
		return algebra.Top(), true
	default:
		return Value{}, false
	}
}

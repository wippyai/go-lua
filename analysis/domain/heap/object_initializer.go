package heap

// ObjectInit is the opaque, one-shot source-order construction capability for
// a complete Heap Object. Each value is consumed independently. Copying an
// unconsumed value intentionally forks its unpublished immutable snapshot, so
// callers can extend a common construction prefix without shared mutation.
// It exposes only Schema-issued construction operations; callers cannot derive
// it from a published Object or Value.
//
// Exact atomic selection is one known Lua key, so the later source entry
// replaces that coordinate, including RawAbsent. A finite or kind selection
// denotes an uncertain runtime key and can only weakly join its observation.
// The initializer is not a MutationLicence path: it exists before a Heap
// Object is published and cannot mutate an Object or Value supplied by a
// caller.
type ObjectInit struct {
	owner  *schema
	object Object
	sealed bool
}

// BeginObject is Schema's only ObjectInit issuer. Schema.Object
// enforces singleton Shape/Frozen seed headers and validates the optional
// initial metatable against this schema.
func (schema Schema) BeginObject(shape Shape, frozen Frozen, metatable Containment) (ObjectInit, bool) {
	object, ok := schema.Object(shape, frozen, metatable)
	if !ok {
		return ObjectInit{}, false
	}
	return ObjectInit{owner: schema.owner, object: object}, true
}

// Apply records one authored field in source order. It does not mutate the
// prior Object: both exact replacement and weak update rebuild the affected
// canonical partition before it becomes the next initializer state.
func (initializer *ObjectInit) Apply(selector KeySelector, replacement CellState) bool {
	if initializer == nil || initializer.sealed || initializer.owner == nil || !initializer.object.valid() ||
		initializer.object.owner != initializer.owner || !selector.valid() || selector.owner != initializer.owner ||
		!replacement.valid() || replacement.owner != initializer.owner {
		return false
	}
	var (
		next Object
		ok   bool
	)
	if selector.Kind() == KeySelectorAtom {
		next, ok = overwriteObjectCell(initializer.object, selector, replacement)
	} else {
		next, ok = weakObjectCell(initializer.object, selector, replacement)
	}
	if !ok {
		return false
	}
	initializer.object = next
	return true
}

// Finish publishes one immutable Object and permanently consumes this
// initializer value. Other values copied from it before consumption retain
// their independent unpublished snapshots.
func (initializer *ObjectInit) Finish() (Object, bool) {
	if initializer == nil || initializer.sealed || initializer.owner == nil || !initializer.object.valid() || initializer.object.owner != initializer.owner {
		return Object{}, false
	}
	object := initializer.object
	initializer.object = Object{}
	initializer.owner = nil
	initializer.sealed = true
	return object, true
}

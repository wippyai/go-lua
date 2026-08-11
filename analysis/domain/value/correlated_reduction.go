package value

import "github.com/wippyai/go-lua/analysis/domain/runtimekind"

// ForRuntimeKinds returns the sealed over-approximation of every Value atom
// that may have one of kinds. It is a precomputed immutable relation with all
// capability attachments possible for each retained atom: a runtime-class
// summary cannot make a capability must-not-hold claim. No runtime cache or
// new Fact plane is involved.
func (schema *Schema) ForRuntimeKinds(kinds runtimekind.Set) (Value, bool) {
	if schema == nil || !kinds.Valid() || schema.forRuntimeKinds == nil {
		return Value{}, false
	}
	return schema.forRuntimeKinds[int(kinds)], true
}

// FilterPresent removes only Lua nil alternatives. False is deliberately
// retained: it is present in Lua tables and is only falsy for control flow.
// atomNil is the first sealed atom and opaque nil has no duplicate atom, so a
// finite input is reduced by an immutable image view rather than allocation.
func (schema *Schema) FilterPresent(input Value) (Value, bool) {
	if schema == nil || !schema.owns(input) {
		return Value{}, false
	}
	if input.top {
		return schema.ForRuntimeKinds(runtimekind.All &^ runtimekind.Bit(runtimekind.Nil))
	}
	if len(input.image) == 0 || input.image[0] != 1 { // atomNil is sealed first.
		return input, true
	}
	return Value{schema: schema, image: input.image[schema.stride():]}, true
}

// FilterStoredNone retains precisely scalar/non-reference alternatives. It
// does not infer anything about a structural child: any rooted or opaque
// reference remains outside this projection. Its result is an immutable
// input-image prefix, so finite hot reductions preserve capability rows with
// no allocation.
func (schema *Schema) FilterStoredNone(input Value) (Value, bool) {
	if schema == nil || !schema.owns(input) || schema.firstStoredUnknown == 0 {
		return Value{}, false
	}
	if input.top {
		return schema.storedNoneTop, true
	}
	end := schema.storedLowerBound(input.image, schema.firstStoredUnknown)
	return schema.storedImage(input, 0, end), true
}

// FilterStoredUnknown retains precisely alternatives whose sealed Value
// provenance cannot name a tracked structural child: opaque reference
// families, endpoints, callables, runtime TypeValues, raw allocation Exact
// occurrences, and unsupported boot roles. It therefore preserves identity,
// role, and attached capabilities rather than collapsing those alternatives
// to Bottom. The class is entirely Value-owned; this package does not import
// or reconstruct another domain's containment vocabulary.
func (schema *Schema) FilterStoredUnknown(input Value) (Value, bool) {
	if schema == nil || !schema.owns(input) || schema.firstStoredUnknown == 0 || schema.firstStoredExact == 0 {
		return Value{}, false
	}
	if input.top {
		return schema.storedUnknownTop, true
	}
	start := schema.storedLowerBound(input.image, schema.firstStoredUnknown)
	end := schema.storedLowerBound(input.image, schema.firstStoredExact)
	return schema.storedImage(input, start, end), true
}

// FilterStoredExact retains one precisely named tracked structural child.
// Only allocation Recent/Summary and boot Exact alternatives are admissible;
// a raw allocation Exact occurrence must use FilterStoredUnknown until the
// owning allocation contribution strongly writes its fresh Recent result.
func (schema *Schema) FilterStoredExact(input Value, selector Atom) (Value, bool) {
	if schema == nil || !schema.owns(input) || !selector.valid() || selector.schema != schema || !schema.storedExactReference(schema.atoms[selector.id-1]) {
		return Value{}, false
	}
	if input.top {
		return schema.atomTop[selector.id], true
	}
	start := schema.storedLowerBound(input.image, selector.id)
	end := start + schema.stride()
	if start >= len(input.image) || uint32(input.image[start]) != selector.id {
		return schema.Bottom(), true
	}
	return Value{schema: schema, image: input.image[start:end]}, true
}

// storedLowerBound returns the byte-free Value-image row offset of the first
// atom ID at least target. Images are already canonically ordered, so all
// three stored projections use one O(log n), allocation-free search.
func (schema *Schema) storedLowerBound(image []uint64, target uint32) int {
	stride := schema.stride()
	low, high := 0, len(image)/stride
	for low < high {
		middle := low + (high-low)/2
		if uint32(image[middle*stride]) < target {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low * stride
}

// storedImage returns one existing immutable range when nonempty and the
// canonical sparse Bottom otherwise. The bounds come only from
// storedLowerBound over an owned Value image.
func (schema *Schema) storedImage(input Value, start, end int) Value {
	if start < 0 || end < start || end > len(input.image) {
		return Value{}
	}
	if start == end {
		return schema.Bottom()
	}
	if start == 0 && end == len(input.image) {
		return input
	}
	return Value{schema: schema, image: input.image[start:end]}
}

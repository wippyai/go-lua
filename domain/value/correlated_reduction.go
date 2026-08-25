package value

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// IdentityValue is Value's direct identity reducer. Value is already an
// immutable, owner-issued lattice fact, so the reducer carries the exact
// input without allocating, widening, or introducing a second ownership
// boundary. The owner fence (when needed) remains the Value/Schema algebra's
// responsibility rather than a reducer callback, so carrying an authenticated
// fact through is always a concrete conclusion.
func IdentityValue(input Value) (Value, structure.ReductionOutcome) {
	return input, structure.Concrete
}

// ArithmeticValue is Value's complete binary-arithmetic judgment. The
// candidate authenticates the operator and owning Schema; the two inputs are
// already owner-issued exact cells. Execution supplies those cells and their
// common support region, but owns none of this semantic decision.
func ArithmeticValue(candidate BinaryArithmetic, left, right Value) (Value, structure.ReductionOutcome) {
	if candidate.schema == nil || !candidate.schema.OwnsBinaryArithmetic(candidate) ||
		!candidate.schema.owns(left) || !candidate.schema.owns(right) {
		return Value{}, structure.Refuse
	}
	result, ok := candidate.schema.ApplyArithmetic(candidate, left, right)
	if !ok {
		return Value{}, structure.Refuse
	}
	if candidate.schema.Equal(result, candidate.schema.Bottom()) {
		return Value{}, structure.NoCandidate
	}
	return result, structure.Concrete
}

// PresenceRefinementValue is Value's complete branch-narrowing judgment. The
// candidate authenticates the guarded polarity and owning Schema; execution
// supplies the one already owner-issued exact cell at the guarded coordinate.
func PresenceRefinementValue(candidate PresenceRefinement, fact Value) (Value, structure.ReductionOutcome) {
	if candidate.schema == nil || !candidate.schema.OwnsPresenceRefinement(candidate) || !candidate.schema.owns(fact) {
		return Value{}, structure.Refuse
	}
	present, ok := candidate.Present()
	if !ok {
		return Value{}, structure.Refuse
	}
	result, ok := candidate.schema.FilterPresence(fact, present)
	if !ok {
		return Value{}, structure.Refuse
	}
	return result, structure.Concrete
}

// EqualityValue is Value's complete binary-equality judgment. The candidate
// authenticates the equality polarity and owning Schema; execution supplies
// the two already owner-issued exact cells and owns missing-cell handling.
// Unlike order, equality preserves the existing semantic behavior of staging
// an explicitly computed Bottom result as a concrete Value fact.
func EqualityValue(candidate BinaryEquality, left, right Value) (Value, structure.ReductionOutcome) {
	if candidate.schema == nil || !candidate.schema.OwnsBinaryEquality(candidate) ||
		!candidate.schema.owns(left) || !candidate.schema.owns(right) {
		return Value{}, structure.Refuse
	}
	notEqual, ok := candidate.NotEqual()
	if !ok {
		return Value{}, structure.Refuse
	}
	result, ok := candidate.schema.CompareEquality(left, right, notEqual)
	if !ok {
		return Value{}, structure.Refuse
	}
	return result, structure.Concrete
}

// OrderValue is Value's complete relational-order judgment. The candidate
// authenticates the closed operator and owning Schema; execution supplies the
// two already owner-issued exact cells and owns missing-cell handling. An
// explicitly Bottom comparison has no reachable order candidate, matching the
// existing order rule's NoCandidate policy.
func OrderValue(candidate BinaryOrder, left, right Value) (Value, structure.ReductionOutcome) {
	if candidate.schema == nil || !candidate.schema.OwnsBinaryOrder(candidate) ||
		!candidate.schema.owns(left) || !candidate.schema.owns(right) {
		return Value{}, structure.Refuse
	}
	op, ok := candidate.Op()
	if !ok {
		return Value{}, structure.Refuse
	}
	result, ok := candidate.schema.CompareOrder(left, right, op)
	if !ok {
		return Value{}, structure.Refuse
	}
	if candidate.schema.Equal(result, candidate.schema.Bottom()) {
		return Value{}, structure.NoCandidate
	}
	return result, structure.Concrete
}

// SourceFact is Value's zero-input source reducer. It rederives the immutable
// source fact from one owner-issued SourceSeed.
func SourceFact(seed SourceSeed) (Value, structure.ReductionOutcome) {
	coordinate, fact, ok := seed.Result()
	if !ok || seed.schema == nil || !seed.schema.AdmitsCoordinate(coordinate, fact) || seed.schema.Equal(fact, seed.schema.Default()) {
		return Value{}, structure.Refuse
	}
	return fact, structure.Concrete
}

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

// RuntimeKindNames projects a Value through the sealed type() result
// vocabulary. The result atoms are issued during Value sealing from
// structure.CategoryRuntimeKind; this hot query only selects a precomputed
// immutable image and therefore allocates nothing.
func (schema *Schema) RuntimeKindNames(input Value) (Value, bool) {
	if schema == nil || !schema.owns(input) || schema.forRuntimeNames == nil {
		return Value{}, false
	}
	if input.top {
		return schema.forRuntimeNames[int(runtimekind.All)], true
	}
	kinds := schema.RuntimeKinds(input)
	if !kinds.Valid() {
		return Value{}, false
	}
	return schema.forRuntimeNames[int(kinds)], true
}

// RuntimeKindNameMatch evaluates the sealed runtime-kind name relation for one
// candidate family. It exposes only may-equal/may-differ outcomes to the
// consumer of an authenticated predicate; scalar payload ownership and the
// runtime-kind spelling projection remain inside Value.
func (schema *Schema) RuntimeKindNameMatch(comparison Value, kind runtimekind.Kind) (mayEqual, mayDiffer bool, ok bool) {
	if schema == nil || !schema.owns(comparison) || !kind.Valid() {
		return false, false, false
	}
	if comparison.IsBottom() {
		return false, false, true
	}
	// The sealed name of one family is read from the presealed name table
	// directly. Routing it through the atom universe reads back the may-kinds
	// of every atom that could be this family, which is wider than the family
	// itself whenever an opaque atom carries two kinds, and the widened set
	// names two spellings instead of one.
	names, namesOK := schema.runtimeKindName(kind)
	nameScalar, nameOK := schema.ExactScalar(names)
	if !namesOK || !nameOK {
		return false, false, false
	}
	comparisonScalar, comparisonOK := schema.ExactScalar(comparison)
	if !comparisonOK {
		return true, true, true
	}
	equal := exactScalarEqual(nameScalar, comparisonScalar)
	return equal, !equal, true
}

// runtimeKindName is the sealed singleton spelling of one runtime family.
func (schema *Schema) runtimeKindName(kind runtimekind.Kind) (Value, bool) {
	set := runtimekind.Bit(kind)
	if schema == nil || schema.forRuntimeNames == nil || !set.Valid() || int(set) >= len(schema.forRuntimeNames) {
		return Value{}, false
	}
	return schema.forRuntimeNames[int(set)], true
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
		return schema.ForRuntimeKinds(runtimekind.NonNil)
	}
	if len(input.image) == 0 || input.image[0] != 1 { // atomNil is sealed first.
		return input, true
	}
	return Value{schema: schema, image: input.image[schema.stride():]}, true
}

// FilterPresence retains exactly the nilability partition selected by one
// owner-issued control-flow refinement. The finite path preserves the exact
// correlated atom/capability row; Top uses the schema's immutable runtime-kind
// census. False remains in the present partition, matching Lua nilability.
func (schema *Schema) FilterPresence(input Value, present bool) (Value, bool) {
	if present {
		return schema.FilterPresent(input)
	}
	if schema == nil || !schema.owns(input) {
		return Value{}, false
	}
	if input.top {
		return schema.ForRuntimeKinds(runtimekind.Bit(runtimekind.Nil))
	}
	if len(input.image) == 0 || input.image[0] != 1 { // atomNil is sealed first.
		return schema.Bottom(), true
	}
	return Value{schema: schema, image: input.image[:schema.stride()]}, true
}

// FilterRuntimeKinds retains the exact correlated alternatives whose sealed
// may-runtime-kind intersects kinds. It is the Value-owned interpretation of
// an authenticated operation predicate; no caller may filter by atom IDs or
// by runtime-kind spellings. Capability tails remain attached to every row.
func (schema *Schema) FilterRuntimeKinds(input Value, kinds runtimekind.Set) (Value, bool) {
	if schema == nil || !schema.owns(input) || !kinds.Valid() {
		return Value{}, false
	}
	if input.top {
		return schema.ForRuntimeKinds(kinds)
	}
	if len(input.image) == 0 || kinds == 0 {
		return schema.Bottom(), true
	}
	stride := schema.stride()
	image := make([]uint64, 0, len(input.image))
	for offset := 0; offset < len(input.image); offset += stride {
		if schema.atomKinds(uint32(input.image[offset]))&kinds == 0 {
			continue
		}
		image = append(image, input.image[offset:offset+stride]...)
	}
	return schema.canonical(image), true
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

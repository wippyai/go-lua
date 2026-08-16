package body

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the immutable result of the Body structural seal. The arrays are
// dense and private so a caller cannot mutate a published result or retain an
// alternate authority over the Body forest.
type Result struct {
	// Provenance is a scalar fence for the sealed Body projections. Body never
	// retains the Source or authored Flow owners themselves; every downstream
	// consumer must assemble against these exact identities.
	sourceID    identity.ContentID
	flowID      identity.ContentID
	parents     []keyspace.Term
	roots       []keyspace.Term
	rootOffsets []uint32
	activation  []keyspace.Term
	nearestLoop []keyspace.Term
	// pre and post are dense Euler timestamps for the sealed Body forest.
	// Index zero is reserved for the invalid zero Term, just like parents.
	pre  []uint32
	post []uint32
}

// Matches reports whether r was sealed from the supplied Source and authored
// Flow identities. Unavailable identities never match, including a malformed
// Result carrying otherwise plausible projection slices.
func Matches(r *Result, sourceID, flowID identity.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID
}

func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available()
}

// BodyCount reports the exact number of sealed Bodies.
func (r *Result) BodyCount() int {
	if !r.available() || len(r.parents) < 2 {
		return 0
	}
	return len(r.parents) - 1
}

// BodyAt returns the canonical dense Body at index.
func (r *Result) BodyAt(index int) (keyspace.Term, bool) {
	if index < 0 || index >= r.BodyCount() {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), true
}

// Parent returns a Body's lexical parent. The Entry has no parent and returns
// false; every other valid Body returns true.
func (r *Result) Parent(body keyspace.Term) (keyspace.Term, bool) {
	ordinal, ok := r.bodyOrdinal(body)
	if !ok {
		return 0, false
	}
	parent := r.parents[ordinal]
	return parent, parent != 0
}

// RootCount reports the number of direct statement roots in a Body.
func (r *Result) RootCount(body keyspace.Term) (int, bool) {
	ordinal, ok := r.bodyOrdinal(body)
	if !ok || len(r.rootOffsets) != len(r.parents) {
		return 0, false
	}
	start, end := r.rootOffsets[ordinal-1], r.rootOffsets[ordinal]
	if end < start || uint64(end) > uint64(len(r.roots)) {
		return 0, false
	}
	return int(end - start), true
}

// RootAt returns one direct statement root in source order.
func (r *Result) RootAt(body keyspace.Term, index int) (keyspace.Term, bool) {
	ordinal, ok := r.bodyOrdinal(body)
	if !ok || index < 0 || len(r.rootOffsets) != len(r.parents) {
		return 0, false
	}
	start, end := r.rootOffsets[ordinal-1], r.rootOffsets[ordinal]
	if end < start || uint64(end) > uint64(len(r.roots)) || uint64(index) >= uint64(end-start) {
		return 0, false
	}
	return r.roots[start+uint32(index)], true
}

// Activation reports the nearest enclosing Function for body. The chunk and
// ordinary Bodies return zero with ok=true.
func (r *Result) Activation(body keyspace.Term) (keyspace.Term, bool) {
	ordinal, ok := r.bodyOrdinal(body)
	if !ok || len(r.activation) != len(r.parents) {
		return 0, false
	}
	return r.activation[ordinal], true
}

// NearestLoop reports the nearest enclosing Loop for body. A Body with no
// enclosing Loop returns zero with ok=true.
func (r *Result) NearestLoop(body keyspace.Term) (keyspace.Term, bool) {
	ordinal, ok := r.bodyOrdinal(body)
	if !ok || len(r.nearestLoop) != len(r.parents) {
		return 0, false
	}
	return r.nearestLoop[ordinal], true
}

// Contains reports lexical Body containment, including the identity case.
// The sealed Euler intervals make this query constant time and allocation
// free. A nil or malformed result fails closed.
func (r *Result) Contains(outer, inner keyspace.Term) bool {
	return r.AncestorOrSelf(outer, inner)
}

// AncestorOrSelf reports whether ancestor is an ancestor (or the identity) of
// descendant in the sealed lexical Body forest. It is deliberately a direct
// interval query: callers must not reconstruct the parent walk.
func (r *Result) AncestorOrSelf(ancestor, descendant keyspace.Term) bool {
	if !r.available() || len(r.parents) < 2 || len(r.pre) != len(r.parents) || len(r.post) != len(r.parents) {
		return false
	}
	ancestorOrdinal, ok := r.bodyOrdinal(ancestor)
	if !ok {
		return false
	}
	descendantOrdinal, ok := r.bodyOrdinal(descendant)
	if !ok {
		return false
	}
	ancestorPre, ancestorPost := r.pre[ancestorOrdinal], r.post[ancestorOrdinal]
	descendantPre, descendantPost := r.pre[descendantOrdinal], r.post[descendantOrdinal]
	// Timestamps are one-based and strictly nested in a valid Euler walk.
	// Check endpoint shape as well as the interval relation so truncated or
	// hand-constructed Results fail closed without a validating scan.
	maxTimestamp := uint64(len(r.parents)-1) * 2
	if ancestorPre == 0 || ancestorPost == 0 || descendantPre == 0 || descendantPost == 0 ||
		ancestorPre >= ancestorPost || descendantPre >= descendantPost ||
		uint64(ancestorPre) > maxTimestamp || uint64(ancestorPost) > maxTimestamp ||
		uint64(descendantPre) > maxTimestamp || uint64(descendantPost) > maxTimestamp {
		return false
	}
	return ancestorPre <= descendantPre && descendantPost <= ancestorPost
}

func (r *Result) bodyOrdinal(body keyspace.Term) (uint32, bool) {
	if !r.available() {
		return 0, false
	}
	if keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.parents)) {
		return 0, false
	}
	return ordinal, true
}

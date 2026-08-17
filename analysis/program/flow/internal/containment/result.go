// Package containment proves Flow's one canonical child-to-parent relation.
//
// The pass is deliberately a derived proof only. It retains dense parents,
// interval coordinates, and the exact static-expression bitset; source,
// owner views, spans, and all construction scratch disappear at return.
package containment

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the immutable containment proof. Each family slice is dense by
// its canonical one-based Term ordinal; index zero is represented by the
// absence of a slice entry, not by a sentinel Term stored in the result.
//
// Results are retained and queried through *Result so the proof's slice
// headers are never copied by value. The fields remain private and there is
// no mutating or secondary view API.
type Result struct {
	total   uint32
	parents [keyspace.FamilyCount][]keyspace.Term
	roles   [keyspace.FamilyCount][]uint64
	pre     [keyspace.FamilyCount][]uint32
	post    [keyspace.FamilyCount][]uint32
	static  [keyspace.FamilyCount][]uint64
	// Construction-only provenance. These scalar fences are the narrow
	// authority used by the final assembly; they retain no owner pointers or
	// generic token.
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
}

// StructuralRole returns the exact owner-issued semantic edge role and
// one-based role-local rank for child. It is deliberately narrower than a
// generic edge view: Parent remains the sole containment relation, while this
// projection exists only so Flow's semantic-path certificate can name the
// already-proved edge without raw Term ordinals.
func (r *Result) StructuralRole(child keyspace.Term) (role, rank uint32, ok bool) {
	family, ordinal, valid := r.ordinal(child)
	if !valid || uint64(ordinal) > uint64(len(r.roles[family])) {
		return 0, 0, false
	}
	packed := r.roles[family][ordinal-1]
	role, rank = uint32(packed>>32), uint32(packed)
	return role, rank, role != 0 && rank != 0
}

// Matches reports whether r was sealed from exactly the supplied Source,
// authored Flow, Static, and Module identities.  The argument order is the
// canonical post-containment provenance order used by every Flow proof.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// available is the query fence for the published proof. A Result carrying
// plausible dense relation rows but any unavailable owner identity is not a
// usable containment authority.
func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available()
}

// Count reports the complete final Term denominator.
func (r *Result) Count() int {
	if !r.available() {
		return 0
	}
	return int(r.total)
}

// At returns one canonical Term in stable family/ordinal order.
func (r *Result) At(index int) (keyspace.Term, bool) {
	if !r.available() {
		return 0, false
	}
	if index < 0 || uint64(index) >= uint64(r.total) {
		return 0, false
	}
	remaining := uint64(index)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := uint64(len(r.parents[family]))
		if remaining < count {
			return keyspace.MakeTerm(family, uint32(remaining+1)), true
		}
		remaining -= count
	}
	return 0, false
}

// Parent returns the unique canonical parent. A root returns false.
func (r *Result) Parent(term keyspace.Term) (keyspace.Term, bool) {
	if !r.available() {
		return 0, false
	}
	family, ordinal, ok := r.ordinal(term)
	if !ok {
		return 0, false
	}
	parent := r.parents[family][ordinal-1]
	return parent, parent != 0
}

// Contains reports transitive containment, including the identity case.
func (r *Result) Contains(outer, inner keyspace.Term) bool {
	if !r.available() {
		return false
	}
	outerFamily, outerOrdinal, outerOK := r.ordinal(outer)
	innerFamily, innerOrdinal, innerOK := r.ordinal(inner)
	if !outerOK || !innerOK {
		return false
	}
	return r.pre[outerFamily][outerOrdinal-1] <= r.pre[innerFamily][innerOrdinal-1] &&
		r.post[innerFamily][innerOrdinal-1] <= r.post[outerFamily][outerOrdinal-1]
}

// Static reports exact static-expression membership. It is a compact bitset
// query and carries no executability or inferred-type meaning.
func (r *Result) Static(term keyspace.Term) bool {
	if !r.available() {
		return false
	}
	family, ordinal, ok := r.ordinal(term)
	if !ok {
		return false
	}
	words := r.static[family]
	word := (ordinal - 1) >> 6
	if uint64(word) >= uint64(len(words)) {
		return false
	}
	return words[word]&(uint64(1)<<((ordinal-1)&63)) != 0
}

func (r *Result) ordinal(term keyspace.Term) (keyspace.Family, uint32, bool) {
	if !r.available() {
		return 0, 0, false
	}
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		ordinal == 0 || uint64(ordinal) > uint64(len(r.parents[family])) {
		return 0, 0, false
	}
	return family, ordinal, true
}

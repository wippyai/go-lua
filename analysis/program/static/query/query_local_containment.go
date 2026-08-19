package query

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// LocalContainment is a lifecycle-bound Static proof query surface. It holds
// the validated immutable relation image supplied by Static's constructor;
// copied views observe the same lifecycle cell and expire together.
type LocalContainment struct {
	proof *Proof
	live  *uint32
}

// LocalContainment returns the validated local proof while this view remains
// live. Published Component views deliberately expose no construction proof.
func (view View) LocalContainment() LocalContainment {
	if !view.available() || !view.snapshot.proof.availableProof() || view.live == nil {
		return LocalContainment{}
	}
	return LocalContainment{proof: view.snapshot.proof, live: view.live}
}

func (proof LocalContainment) snapshot() *Proof {
	if proof.proof == nil || !proof.proof.available || proof.live == nil || atomic.LoadUint32(proof.live) == 0 {
		return nil
	}
	return proof.proof
}

// Parent returns the exact local Static containment parent for one concrete
// static type term. A valid root has parent zero with ok=false.
func (proof LocalContainment) Parent(term keyspace.Term) (parent keyspace.Term, ok bool) {
	local := proof.snapshot()
	if local == nil {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if !StaticTypeFamily(family) || ordinal == 0 {
		return 0, false
	}
	parents := local.parents[family]
	if uint64(ordinal) > uint64(len(parents)) {
		return 0, false
	}
	parent = parents[ordinal-1]
	return parent, parent != 0
}

// FieldOwner returns the exact Record or Interface owner of one Field.
func (proof LocalContainment) FieldOwner(field keyspace.Term) (owner keyspace.Term, ok bool) {
	local := proof.snapshot()
	if local == nil || keyspace.TermFamily(field) != keyspace.FamilyTypeField {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(field)
	if ordinal == 0 || uint64(ordinal) > uint64(len(local.fieldOwners)) {
		return 0, false
	}
	owner = local.fieldOwners[ordinal-1]
	return owner, owner != 0
}

// Count returns the finite closed Static type-family denominator.
func (proof LocalContainment) Count() int {
	local := proof.snapshot()
	if local == nil {
		return 0
	}
	total := 0
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeConditional; family++ {
		if StaticTypeFamily(family) {
			total += len(local.parents[family])
		}
	}
	return total
}

// At returns one term from Count's deterministic closed Static family order.
func (proof LocalContainment) At(index int) (keyspace.Term, bool) {
	local := proof.snapshot()
	if local == nil || index < 0 {
		return 0, false
	}
	offset := uint64(index)
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeConditional; family++ {
		if !StaticTypeFamily(family) {
			continue
		}
		count := uint64(len(local.parents[family]))
		if offset < count {
			return keyspace.MakeTerm(family, uint32(offset+1)), true
		}
		offset -= count
	}
	return 0, false
}

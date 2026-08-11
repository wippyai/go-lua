package static

import "github.com/wippyai/go-lua/program/keyspace"

// localContainmentProof is the one immutable local relation image retained
// while a Static Draft is claimed. It has no direct-return evidence, generic
// edge records, or Component pointer; terminal Finalizer actions clear the
// Draft's only reference to it.
type localContainmentProof struct {
	parents     [keyspace.FamilyCount][]keyspace.Term
	fieldOwners []keyspace.Term
}

// LocalContainment is a lifecycle-bound Static proof query surface. Values
// are intentionally handles to shared Draft state, not copies of the proof:
// copied handles all expire after Commit or Abort.
type LocalContainment struct{ state *draftState }

// LocalContainment exposes the validated local Static parent rows while this
// construction View remains claimed. It is intentionally derived only from
// View.state: a published Component View has no construction state and
// therefore returns an expired handle. Copied Views share the lifecycle fence
// and all expire together after Commit or Abort.
func (view View) LocalContainment() LocalContainment {
	state := view.state
	if state == nil {
		return LocalContainment{}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftClaimed || state.localContainment == nil {
		return LocalContainment{}
	}
	return LocalContainment{state: state}
}

// snapshot returns one immutable proof pointer after taking the lifecycle
// fence. Terminal actions never mutate the pointed-to rows, so a query that
// has acquired this snapshot may finish safely even if termination follows.
func (proof LocalContainment) snapshot() *localContainmentProof {
	state := proof.state
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftClaimed {
		return nil
	}
	return state.localContainment
}

// Parent returns the exact local Static containment parent for one concrete
// static type term. A valid root has parent zero with ok=false, matching the
// parent-query convention used by the other Program owners; malformed,
// foreign, and out-of-range family/ordinal terms also fail closed. Field
// membership is queried separately with FieldOwner.
func (proof LocalContainment) Parent(term keyspace.Term) (parent keyspace.Term, ok bool) {
	local := proof.snapshot()
	if local == nil {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if !localStaticTypeFamily(family) || ordinal == 0 {
		return 0, false
	}
	parents := local.parents[family]
	if uint64(ordinal) > uint64(len(parents)) {
		return 0, false
	}
	parent = parents[ordinal-1]
	return parent, parent != 0
}

// FieldOwner returns the exact Record or Interface owner of one Field. It is
// separate from Parent because Field-to-value-type and owner-to-Field are
// distinct authored relations.
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

// Count returns the finite closed Static type-family denominator needed by a
// coordinator to enumerate the proof's Parent domain.
func (proof LocalContainment) Count() int {
	local := proof.snapshot()
	if local == nil {
		return 0
	}
	total := 0
	for _, family := range staticTypeFamilies {
		total += len(local.parents[family])
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
	for _, family := range staticTypeFamilies {
		count := uint64(len(local.parents[family]))
		if offset < count {
			return keyspace.MakeTerm(family, uint32(offset+1)), true
		}
		offset -= count
	}
	return 0, false
}

func localStaticTypeFamily(family keyspace.Family) bool {
	for _, candidate := range staticTypeFamilies {
		if family == candidate {
			return true
		}
	}
	return false
}

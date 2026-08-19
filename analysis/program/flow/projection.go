package flow

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// Activation is the retained Body activation projection.  It is deliberately
// separate from Source's Body parent/root index.
type Activation struct{ projection *activationProjection }

func (view Activation) Count() int {
	if view.projection == nil {
		return 0
	}
	return len(view.projection.terms)
}
func (view Activation) At(index int) (keyspace.Term, bool) {
	if view.projection == nil || index < 0 || index >= len(view.projection.terms) {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), true
}
func (view Activation) For(body keyspace.Term) (keyspace.Term, bool) {
	if view.projection == nil || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) > uint64(len(view.projection.terms)) {
		return 0, false
	}
	activation := view.projection.terms[ordinal-1]
	return activation, true
}

// Containment retains every canonical term, including Body roots, together
// with its parent edge and static classification. Body rows are needed by
// consumers that distinguish a statically owned function body from an
// executable body; omitting them would erase that owner-level judgment.
type Containment struct{ projection *containmentProjection }

func (view Containment) Count() int {
	if view.projection == nil {
		return 0
	}
	return len(view.projection.terms)
}
func (view Containment) At(index int) (keyspace.Term, bool) {
	if view.projection == nil || index < 0 || index >= len(view.projection.terms) {
		return 0, false
	}
	return view.projection.terms[index], true
}
func (view Containment) Parent(term keyspace.Term) (keyspace.Term, bool) {
	index, ok := view.index(term)
	if !ok {
		return 0, false
	}
	parent := view.projection.parents[index]
	return parent, parent != 0
}
func (view Containment) Static(term keyspace.Term) bool {
	index, ok := view.index(term)
	return ok && view.projection.static[index]
}
func (view Containment) index(term keyspace.Term) (int, bool) {
	if view.projection == nil {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return 0, false
	}
	plane := view.projection.index[family]
	if uint64(ordinal) >= uint64(len(plane)) || plane[ordinal] == 0 {
		return 0, false
	}
	return int(plane[ordinal] - 1), true
}

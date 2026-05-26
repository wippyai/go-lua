package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/constraint/theory"
	"github.com/wippyai/go-lua/types/narrow"
)

type atomTruthState uint8

const (
	atomSeenTruthy atomTruthState = 1 << iota
	atomSeenFalsy
	atomSeenNil
	atomSeenNotNil
)

type atomTypeStateKey struct {
	path constraint.PathKey
	key  narrow.TypeKey
}

type atomContradictionTracker struct {
	equalities *theory.EGraph
	truth      map[constraint.PathKey]atomTruthState
	hasType    map[atomTypeStateKey]bool
	notType    map[atomTypeStateKey]bool
}

func newAtomContradictionTracker(equalities *theory.EGraph) *atomContradictionTracker {
	return &atomContradictionTracker{equalities: equalities}
}

func (t *atomContradictionTracker) add(atom constraint.Atom) bool {
	switch atom.Kind {
	case constraint.AtomKindTruthy:
		return atom.Left.IsVar() && t.addTruth(atom.Left.Path, atomSeenTruthy)
	case constraint.AtomKindFalsy:
		return atom.Left.IsVar() && t.addTruth(atom.Left.Path, atomSeenFalsy)
	case constraint.AtomKindEq:
		return t.addNilEquality(atom)
	case constraint.AtomKindNe:
		return t.addNilInequality(atom)
	case constraint.AtomKindHasType:
		return t.addHasType(atom)
	case constraint.AtomKindNotHasType:
		return t.addNotHasType(atom)
	default:
		return false
	}
}

func (t *atomContradictionTracker) addNilEquality(atom constraint.Atom) bool {
	if atom.Left.IsVar() && atom.Right.IsNil() && t.addTruth(atom.Left.Path, atomSeenNil) {
		return true
	}
	return atom.Right.IsVar() && atom.Left.IsNil() && t.addTruth(atom.Right.Path, atomSeenNil)
}

func (t *atomContradictionTracker) addNilInequality(atom constraint.Atom) bool {
	if atom.Left.IsVar() && atom.Right.IsNil() && t.addTruth(atom.Left.Path, atomSeenNotNil) {
		return true
	}
	return atom.Right.IsVar() && atom.Left.IsNil() && t.addTruth(atom.Right.Path, atomSeenNotNil)
}

func (t *atomContradictionTracker) addTruth(path constraint.PathKey, state atomTruthState) bool {
	path = t.canonical(path)
	if path == "" {
		return false
	}
	if t.truth == nil {
		t.truth = make(map[constraint.PathKey]atomTruthState)
	}
	next := t.truth[path] | state
	if contradictoryTruth(next) {
		return true
	}
	t.truth[path] = next
	return false
}

func contradictoryTruth(state atomTruthState) bool {
	if state&atomSeenTruthy != 0 && (state&atomSeenFalsy != 0 || state&atomSeenNil != 0) {
		return true
	}
	return state&atomSeenNil != 0 && state&atomSeenNotNil != 0
}

func (t *atomContradictionTracker) addHasType(atom constraint.Atom) bool {
	if !atom.Left.IsVar() {
		return false
	}
	key, ok := t.typeStateKey(atom)
	if !ok {
		return false
	}
	if t.notType != nil && t.notType[key] {
		return true
	}
	if t.hasType == nil {
		t.hasType = make(map[atomTypeStateKey]bool)
	}
	t.hasType[key] = true
	return false
}

func (t *atomContradictionTracker) addNotHasType(atom constraint.Atom) bool {
	if !atom.Left.IsVar() {
		return false
	}
	key, ok := t.typeStateKey(atom)
	if !ok {
		return false
	}
	if t.hasType != nil && t.hasType[key] {
		return true
	}
	if t.notType == nil {
		t.notType = make(map[atomTypeStateKey]bool)
	}
	t.notType[key] = true
	return false
}

func (t *atomContradictionTracker) typeStateKey(atom constraint.Atom) (atomTypeStateKey, bool) {
	path := t.canonical(atom.Left.Path)
	if path == "" {
		return atomTypeStateKey{}, false
	}
	return atomTypeStateKey{path: path, key: atom.TypeKey}, true
}

func (t *atomContradictionTracker) canonical(path constraint.PathKey) constraint.PathKey {
	if path == "" || t.equalities == nil {
		return path
	}
	return t.equalities.Find(path)
}

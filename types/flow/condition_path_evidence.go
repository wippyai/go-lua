package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/constraint/theory"
)

type conditionPathEquality struct {
	left  constraint.PathKey
	right constraint.PathKey
}

type conditionPathEvidence struct {
	keys       pathKeyList
	equalities []conditionPathEquality
}

func newConditionPathEvidence(atoms []constraint.Atom, constraints []constraint.Constraint, resolve constraint.PathResolver) conditionPathEvidence {
	evidence := conditionPathEvidence{}
	for _, c := range constraints {
		evidence.addConstraint(c, resolve)
	}
	for _, atom := range atoms {
		evidence.addAtom(atom)
	}
	return evidence
}

func newConstraintPathEvidence(c constraint.Constraint, resolve constraint.PathResolver) conditionPathEvidence {
	evidence := conditionPathEvidence{}
	evidence.addConstraint(c, resolve)
	return evidence
}

func (e *conditionPathEvidence) addConstraint(c constraint.Constraint, resolve constraint.PathResolver) {
	if resolve == nil {
		return
	}
	constraint.VisitPaths(c, func(path constraint.Path) bool {
		e.addKey(resolve(path))
		return false
	})
	if eq, ok := c.(constraint.EqPath); ok {
		e.addEquality(resolve(eq.Left), resolve(eq.Right))
	}
}

func (e *conditionPathEvidence) addAtom(atom constraint.Atom) {
	for _, key := range atom.Paths() {
		e.addKey(key)
	}
	if atom.Kind == constraint.AtomKindEq && atom.Left.IsVar() && atom.Right.IsVar() {
		e.addEquality(atom.Left.Path, atom.Right.Path)
	}
}

func (e *conditionPathEvidence) addKey(key constraint.PathKey) {
	if key == "" {
		return
	}
	e.keys.Add(key)
}

func (e *conditionPathEvidence) addEquality(left, right constraint.PathKey) {
	if left == "" || right == "" {
		return
	}
	e.equalities = append(e.equalities, conditionPathEquality{left: left, right: right})
}

func (e conditionPathEvidence) RegisterInto(graph *theory.EGraph) {
	if graph == nil {
		return
	}
	for _, key := range e.keys.SortedValues() {
		graph.RegisterKey(key)
	}
	for _, eq := range e.equalities {
		graph.Union(eq.left, eq.right)
	}
}

func (e conditionPathEvidence) Keys() []constraint.PathKey {
	return e.keys.SortedValues()
}

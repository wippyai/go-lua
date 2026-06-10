package flow

import "github.com/wippyai/go-lua/types/constraint"

type conditionAtomClass int

const (
	conditionAtomClassNone conditionAtomClass = iota
	conditionAtomClassType
	conditionAtomClassNumeric
	conditionAtomClassBoth
)

func classifyConditionAtom(atom constraint.Atom) conditionAtomClass {
	switch atom.Kind {
	case constraint.AtomKindHasType, constraint.AtomKindNotHasType,
		constraint.AtomKindTruthy, constraint.AtomKindFalsy:
		return conditionAtomClassType

	case constraint.AtomKindLt, constraint.AtomKindLe,
		constraint.AtomKindGt, constraint.AtomKindGe,
		constraint.AtomKindModEq:
		return conditionAtomClassNumeric

	case constraint.AtomKindEq, constraint.AtomKindNe:
		if atom.Left.IsNil() || atom.Right.IsNil() {
			return conditionAtomClassType
		}
		if atom.Left.IsConst() || atom.Right.IsConst() {
			return conditionAtomClassNumeric
		}
		return conditionAtomClassBoth
	}
	return conditionAtomClassNone
}

package numeric

import "github.com/wippyai/go-lua/types/constraint"

type numericAtomApplier struct {
	domain *Domain
	atom   constraint.Atom
}

func (a numericAtomApplier) apply() {
	switch a.atom.Kind {
	case constraint.AtomKindLt:
		a.lt()
	case constraint.AtomKindLe:
		a.le()
	case constraint.AtomKindGe:
		a.ge()
	case constraint.AtomKindGt:
		a.gt()
	case constraint.AtomKindEq:
		a.eq()
	case constraint.AtomKindModEq:
		a.modEq()
	}
}

func (a numericAtomApplier) lt() {
	if !a.atom.Left.IsVar() || !a.atom.Right.IsVar() {
		return
	}
	a.domain.state.ApplyLt(a.atom.Left.Path, a.atom.Right.Path)
	a.domain.theory.AddDifferenceConstraint(a.atom.Left.Path, a.atom.Right.Path, -1)
}

func (a numericAtomApplier) le() {
	switch {
	case a.atom.Left.IsVar() && a.atom.Right.IsConst():
		a.domain.state.ApplyLeConst(a.atom.Left.Path, a.atom.Right.Const)
		a.domain.theory.AddBounds(a.atom.Left.Path, -maxWeight, a.atom.Right.Const)
	case a.atom.Left.IsVar() && a.atom.Right.IsLen():
		a.domain.state.ApplyLeLenOf(a.atom.Left.Path, a.atom.Right.Path)
	case a.atom.Left.IsLen() && a.atom.Right.IsConst():
		a.domain.state.ApplyLenLeConst(a.atom.Left.Path, a.atom.Right.Const)
	case a.atom.Left.IsVar() && a.atom.Right.IsVar():
		a.domain.state.ApplyLe(a.atom.Left.Path, a.atom.Right.Path)
		a.domain.theory.AddDifferenceConstraint(a.atom.Left.Path, a.atom.Right.Path, 0)
	}
}

func (a numericAtomApplier) ge() {
	switch {
	case a.atom.Left.IsVar() && a.atom.Right.IsConst():
		a.domain.state.ApplyGeConst(a.atom.Left.Path, a.atom.Right.Const)
		a.domain.theory.AddBounds(a.atom.Left.Path, a.atom.Right.Const, maxWeight)
	case a.atom.Left.IsLen() && a.atom.Right.IsConst():
		a.domain.state.ApplyLenGeConst(a.atom.Left.Path, a.atom.Right.Const)
	case a.atom.Left.IsVar() && a.atom.Right.IsVar():
		a.domain.state.ApplyGe(a.atom.Left.Path, a.atom.Right.Path)
		a.domain.theory.AddDifferenceConstraint(a.atom.Right.Path, a.atom.Left.Path, 0)
	}
}

func (a numericAtomApplier) gt() {
	if !a.atom.Left.IsVar() || !a.atom.Right.IsVar() {
		return
	}
	a.domain.state.ApplyGt(a.atom.Left.Path, a.atom.Right.Path)
	a.domain.theory.AddDifferenceConstraint(a.atom.Right.Path, a.atom.Left.Path, -1)
}

func (a numericAtomApplier) eq() {
	switch {
	case a.atom.Left.IsVar() && a.atom.Right.IsConst():
		a.domain.state.ApplyEqConst(a.atom.Left.Path, a.atom.Right.Const)
		a.domain.theory.AddBounds(a.atom.Left.Path, a.atom.Right.Const, a.atom.Right.Const)
	case a.atom.Left.IsVar() && a.atom.Right.IsVar():
		a.domain.state.ApplyEq(a.atom.Left.Path, a.atom.Right.Path)
		a.domain.theory.AddDifferenceConstraint(a.atom.Left.Path, a.atom.Right.Path, 0)
		a.domain.theory.AddDifferenceConstraint(a.atom.Right.Path, a.atom.Left.Path, 0)
	}
}

func (a numericAtomApplier) modEq() {
	if !a.atom.Left.IsVar() {
		return
	}
	a.domain.state.ApplyModEq(a.atom.Left.Path, a.atom.Mod, a.atom.Rem)
	a.domain.theory.AddModular(a.atom.Left.Path, a.atom.Mod, a.atom.Rem)
}

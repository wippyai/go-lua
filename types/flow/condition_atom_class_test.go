package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestClassifyConditionAtom_HasType(t *testing.T) {
	atom := constraint.Atom{Kind: constraint.AtomKindHasType}
	if got := classifyConditionAtom(atom); got != conditionAtomClassType {
		t.Errorf("classifyConditionAtom(HasType) = %v, want conditionAtomClassType", got)
	}
}

func TestClassifyConditionAtom_Truthy(t *testing.T) {
	atom := constraint.Atom{Kind: constraint.AtomKindTruthy}
	if got := classifyConditionAtom(atom); got != conditionAtomClassType {
		t.Errorf("classifyConditionAtom(Truthy) = %v, want conditionAtomClassType", got)
	}
}

func TestClassifyConditionAtom_Lt(t *testing.T) {
	atom := constraint.Atom{Kind: constraint.AtomKindLt}
	if got := classifyConditionAtom(atom); got != conditionAtomClassNumeric {
		t.Errorf("classifyConditionAtom(Lt) = %v, want conditionAtomClassNumeric", got)
	}
}

func TestClassifyConditionAtom_EqNil(t *testing.T) {
	atom := constraint.Atom{
		Kind:  constraint.AtomKindEq,
		Right: constraint.TermNil(),
	}
	if got := classifyConditionAtom(atom); got != conditionAtomClassType {
		t.Errorf("classifyConditionAtom(Eq nil) = %v, want conditionAtomClassType", got)
	}
}

func TestClassifyConditionAtom_EqConst(t *testing.T) {
	atom := constraint.Atom{
		Kind:  constraint.AtomKindEq,
		Left:  constraint.TermConst(5),
		Right: constraint.TermConst(10),
	}
	if got := classifyConditionAtom(atom); got != conditionAtomClassNumeric {
		t.Errorf("classifyConditionAtom(Eq const) = %v, want conditionAtomClassNumeric", got)
	}
}

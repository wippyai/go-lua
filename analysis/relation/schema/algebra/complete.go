package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Complete closes a child relation against an authenticated logical
// denominator. It carries no anti-join, scan, or physical arrangement choice.
type Complete struct {
	child       Expression
	denominator model.DenominatorRef
}

// NewComplete constructs a completion expression without deciding whether the
// denominator is authorized for the child.
func NewComplete(child Expression, denominator model.DenominatorRef) Complete {
	return Complete{child: child, denominator: denominator}
}

// Child returns the completed child expression.
func (complete Complete) Child() Expression { return complete.child }

// Denominator returns the stable denominator reference.
func (complete Complete) Denominator() model.DenominatorRef { return complete.denominator }

// Kind implements Expression.
func (complete Complete) Kind() Kind { return KindComplete }

// Digest returns the deterministic structural identity.
func (complete Complete) Digest() identity.ContentID {
	parts := appendExpr(nil, complete.child)
	parts = appendDenominator(parts, complete.denominator)
	return derive("analysis/relation/schema/algebra/complete/v1", parts)
}

func (complete Complete) expression() {}

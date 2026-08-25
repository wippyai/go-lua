package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Select retains rows from one child under a sealed scope contract. It does
// not contain a callback or executable predicate. Equality, tag, and other
// row conditions are normalized into oriented Join expressions over declared
// columns; a semantic condition is an Apply that publishes a qualifying
// relation, which is then joined to the child.
type Select struct {
	child    Expression
	contract SelectContract
}

// NewSelect constructs a Select expression without applying typing rules.
func NewSelect(child Expression, contract SelectContract) Select {
	return Select{child: child, contract: contract}
}

// Child returns the retained child expression.
func (selectExpr Select) Child() Expression { return selectExpr.child }

// Contract returns the immutable filter contract.
func (selectExpr Select) Contract() SelectContract { return selectExpr.contract }

// Kind implements Expression.
func (selectExpr Select) Kind() Kind { return KindSelect }

// Digest returns the deterministic structural identity.
func (selectExpr Select) Digest() identity.ContentID {
	parts := appendExpr(nil, selectExpr.child)
	return derive("analysis/relation/schema/algebra/select/v2", append(parts, selectExpr.contract.digestBytes()...))
}

func (selectExpr Select) expression() {}

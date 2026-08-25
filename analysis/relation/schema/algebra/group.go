package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Group collects one child under a declared key and delivery contract. Any
// semantic reduction is an explicit Apply expression; Group itself only
// describes relational grouping and delivered cardinality.
type Group struct {
	child    Expression
	contract GroupContract
}

// NewGroup constructs a Group expression without applying checker rules.
func NewGroup(child Expression, contract GroupContract) Group {
	return Group{child: child, contract: contract}
}

// Child returns the grouped child expression.
func (group Group) Child() Expression { return group.child }

// Contract returns the immutable group contract.
func (group Group) Contract() GroupContract { return group.contract }

// Kind implements Expression.
func (group Group) Kind() Kind { return KindGroup }

// Digest returns the deterministic structural identity.
func (group Group) Digest() identity.ContentID {
	parts := appendExpr(nil, group.child)
	return derive("analysis/relation/schema/algebra/group/v1", append(parts, group.contract.digestBytes()...))
}

func (group Group) expression() {}

package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Join composes two logical children under an oriented declarative
// correspondence contract. Left/right orientation is part of identity.
type Join struct {
	left     Expression
	right    Expression
	contract JoinContract
}

// NewJoin constructs a Join expression without deciding key compatibility.
func NewJoin(left, right Expression, contract JoinContract) Join {
	return Join{left: left, right: right, contract: contract}
}

// Left returns the oriented left child.
func (join Join) Left() Expression { return join.left }

// Right returns the oriented right child.
func (join Join) Right() Expression { return join.right }

// Contract returns the immutable join contract.
func (join Join) Contract() JoinContract { return join.contract }

// Kind implements Expression.
func (join Join) Kind() Kind { return KindJoin }

// Digest returns the deterministic structural identity. Child orientation is
// intentionally framed as left then right.
func (join Join) Digest() identity.ContentID {
	parts := appendExpr(nil, join.left)
	parts = appendExpr(parts, join.right)
	return derive("analysis/relation/schema/algebra/join/v2", append(parts, join.contract.digestBytes()...))
}

func (join Join) expression() {}

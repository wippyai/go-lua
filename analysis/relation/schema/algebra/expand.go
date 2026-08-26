package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Expand redeems a sealed finite key vector published by the contract's P
// relation for each row on the C-left child, then appends the corresponding R
// row columns in the vector's declared order. The owner vector is mount
// evidence; this logical node carries only its stable identities and delivery
// contract, never a callback, ordinal, coordinate, or runtime token.
type Expand struct {
	child    Expression
	contract model.ExpandContract
}

// NewExpand constructs a dependent join without applying schema checks.
func NewExpand(child Expression, contract model.ExpandContract) Expand {
	return Expand{child: child, contract: contract}
}

// Child returns the C-left expression being expanded.
func (expand Expand) Child() Expression { return expand.child }

// Contract returns the immutable C/P/R/key/correlation contract.
func (expand Expand) Contract() model.ExpandContract { return expand.contract }

// Kind implements Expression.
func (expand Expand) Kind() Kind { return KindExpand }

// Digest returns the complete logical identity. The model owns the contract
// encoding; this node contributes only its child and the sealed contract
// digest. Mount vector contents intentionally are not present because they are
// evidence for the same logical contract.
func (expand Expand) Digest() identity.ContentID {
	parts := appendExpr(nil, expand.child)
	parts = appendContent(parts, expand.contract.Digest())
	return derive("analysis/relation/schema/algebra/expand/v2", parts)
}

func (expand Expand) expression() {}

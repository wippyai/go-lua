package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Apply invokes one authenticated typed semantic operation over ordered child
// relations. The operation itself is not executable data in this node.
type Apply struct {
	inputs   []Expression
	contract ApplyContract
}

// NewApply copies input expressions and applies no signature or capability
// validation.
func NewApply(inputs []Expression, contract ApplyContract) Apply {
	return Apply{inputs: cloneExpressions(inputs), contract: contract}
}

// Inputs returns a defensive copy in authored order.
func (apply Apply) Inputs() []Expression { return cloneExpressions(apply.inputs) }

// Contract returns the immutable operation contract.
func (apply Apply) Contract() ApplyContract { return apply.contract }

// Kind implements Expression.
func (apply Apply) Kind() Kind { return KindApply }

// Digest returns the deterministic structural identity.
func (apply Apply) Digest() identity.ContentID {
	parts := appendExprs(nil, apply.inputs)
	return derive("analysis/relation/schema/algebra/apply/v1", append(parts, apply.contract.digestBytes()...))
}

func (apply Apply) expression() {}

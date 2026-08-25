package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Merge combines alternative derivations under a declared key. The lattice
// algebra is resolved from the output column TypeIDs by the checker/runtime
// binding; this node carries no second reducer authority. The authored input
// order is retained for deterministic structural identity.
type Merge struct {
	inputs   []Expression
	contract MergeContract
}

// NewMerge copies the input expression slice and applies no semantic checks.
func NewMerge(inputs []Expression, contract MergeContract) Merge {
	return Merge{inputs: cloneExpressions(inputs), contract: contract}
}

// Inputs returns a defensive copy in authored order.
func (merge Merge) Inputs() []Expression { return cloneExpressions(merge.inputs) }

// Contract returns the immutable merge contract.
func (merge Merge) Contract() MergeContract { return merge.contract }

// Kind implements Expression.
func (merge Merge) Kind() Kind { return KindMerge }

// Digest returns the deterministic structural identity.
func (merge Merge) Digest() identity.ContentID {
	parts := appendExprs(nil, merge.inputs)
	return derive("analysis/relation/schema/algebra/merge/v2", append(parts, merge.contract.digestBytes()...))
}

func (merge Merge) expression() {}

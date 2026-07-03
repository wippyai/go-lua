package factflow

import "github.com/wippyai/go-lua/analysis/domain/value/product"

type ExpressionRefinementMode uint8

const (
	ExpressionRefinementMeet ExpressionRefinementMode = iota
	ExpressionRefinementDeclaredContract
)

// ExpressionRefinement describes a source expression whose value is resolved
// from an inner source and then refined with a product value.
type ExpressionRefinement struct {
	source     ValueSource
	refinement product.Value
	mode       ExpressionRefinementMode
}

// NewExpressionRefinement creates an expression-value refinement fact.
func NewExpressionRefinement(source ValueSource, refinement product.Value) ExpressionRefinement {
	return ExpressionRefinement{
		source:     source,
		refinement: refinement,
		mode:       ExpressionRefinementMeet,
	}
}

// NewExpressionDeclaredContract creates a refinement that overlays a declared
// contract onto the source value without treating the contract as a validation
// proof that erases existing evidence.
func NewExpressionDeclaredContract(source ValueSource, declared product.Value) ExpressionRefinement {
	return ExpressionRefinement{
		source:     source,
		refinement: declared,
		mode:       ExpressionRefinementDeclaredContract,
	}
}

// Source returns the inner value source.
func (r ExpressionRefinement) Source() ValueSource { return r.source }

// Refinement returns the product value met onto the resolved inner source value.
func (r ExpressionRefinement) Refinement() product.Value { return r.refinement }

// Mode returns how the refinement value should be applied to the inner source.
func (r ExpressionRefinement) Mode() ExpressionRefinementMode { return r.mode }

func (r ExpressionRefinement) copy() ExpressionRefinement { return r }

func copyExpressionRefinementMap(in map[ExprRef]ExpressionRefinement) map[ExprRef]ExpressionRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ExpressionRefinement, len(in))
	for expr, fact := range in {
		out[expr] = fact.copy()
	}
	return out
}

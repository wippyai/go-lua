package factflow

import "github.com/wippyai/go-lua/analysis/domain/value/product"

// ExpressionRefinement describes a source expression whose value is resolved
// from an inner source and then conjunctively refined with a product value.
type ExpressionRefinement struct {
	source     ValueSource
	refinement product.Value
}

// NewExpressionRefinement creates an expression-value refinement fact.
func NewExpressionRefinement(source ValueSource, refinement product.Value) ExpressionRefinement {
	return ExpressionRefinement{
		source:     source,
		refinement: refinement,
	}
}

// Source returns the inner value source.
func (r ExpressionRefinement) Source() ValueSource { return r.source }

// Refinement returns the product value met onto the resolved inner source value.
func (r ExpressionRefinement) Refinement() product.Value { return r.refinement }

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

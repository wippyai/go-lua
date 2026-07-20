package factflow

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type ExpressionRefinementMode uint8

const (
	ExpressionRefinementMeet ExpressionRefinementMode = iota
	ExpressionRefinementDeclaredContract
	ExpressionRefinementRuntimeValidation
)

// ExpressionRefinement describes a source expression whose value is resolved
// from an inner source and then refined with a product value.
type ExpressionRefinement struct {
	source        ValueSource
	refinement    product.Value
	mode          ExpressionRefinementMode
	resultPath    pathdom.Path
	hasResultPath bool
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

// NewExpressionRuntimeValidation creates a refinement that models a runtime
// check: if the expression produces a value, it satisfies the validated target.
func NewExpressionRuntimeValidation(source ValueSource, validated product.Value) ExpressionRefinement {
	return ExpressionRefinement{
		source:     source,
		refinement: validated,
		mode:       ExpressionRefinementRuntimeValidation,
	}
}

// Source returns the inner value source.
func (r ExpressionRefinement) Source() ValueSource { return r.source }

// Refinement returns the product value met onto the resolved inner source value.
func (r ExpressionRefinement) Refinement() product.Value { return r.refinement }

// Mode returns how the refinement value should be applied to the inner source.
func (r ExpressionRefinement) Mode() ExpressionRefinementMode { return r.mode }

// WithResultPath binds the exact addressable result of this wrapper. The path
// describes the post-refinement expression value, not its pre-validation
// Source. Keeping the two roles in one atom prevents consumers from inferring
// wrapper identity by comparing unrelated SSA spellings.
func (r ExpressionRefinement) WithResultPath(path pathdom.Path) ExpressionRefinement {
	if path.IsEmpty() {
		return r
	}
	r.resultPath = path.Clone()
	r.hasResultPath = true
	return r
}

// ResultPath returns the exact addressable wrapper result, when lowering
// proved one.
func (r ExpressionRefinement) ResultPath() (pathdom.Path, bool) {
	if !r.hasResultPath || r.resultPath.IsEmpty() {
		return pathdom.Path{}, false
	}
	return r.resultPath.Clone(), true
}

// ResultPathRef returns the wrapper result for immediate read-only use.
func (r ExpressionRefinement) ResultPathRef() (pathdom.Path, bool) {
	if !r.hasResultPath || r.resultPath.IsEmpty() {
		return pathdom.Path{}, false
	}
	return r.resultPath, true
}

func (r ExpressionRefinement) copy() ExpressionRefinement {
	r.resultPath = r.resultPath.Clone()
	return r
}

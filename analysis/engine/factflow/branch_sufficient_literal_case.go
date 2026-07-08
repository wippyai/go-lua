package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// BranchSufficientLiteralCase records that a path holding LiteralValue is
// sufficient to select Edge at a branch. Unlike BranchRefinement, this is not an
// edge-implies-value fact: for `x == "a" or x == "b"`, either literal is
// sufficient for the true edge, but the true edge does not imply either one.
type BranchSufficientLiteralCase struct {
	targetPath   path.Path
	literalValue product.Value
	edge         bool
}

// BranchSufficientLiteralCaseSet groups sufficient literal cases for one branch.
type BranchSufficientLiteralCaseSet struct {
	cases []BranchSufficientLiteralCase
}

// NewBranchSufficientLiteralCase creates a branch sufficient literal fact.
func NewBranchSufficientLiteralCase(targetPath path.Path, literalValue product.Value, edge bool) BranchSufficientLiteralCase {
	return BranchSufficientLiteralCase{
		targetPath:   targetPath.Clone(),
		literalValue: literalValue,
		edge:         edge,
	}
}

// NewBranchSufficientLiteralCaseSet creates a case set.
func NewBranchSufficientLiteralCaseSet(cases ...BranchSufficientLiteralCase) BranchSufficientLiteralCaseSet {
	return BranchSufficientLiteralCaseSet{cases: copyBranchSufficientLiteralCaseSlice(cases)}
}

// TargetPath returns the path whose literal value selects the branch edge.
func (c BranchSufficientLiteralCase) TargetPath() path.Path { return c.targetPath.Clone() }

// TargetPathRef returns the internal path for read-only hot paths.
func (c BranchSufficientLiteralCase) TargetPathRef() path.Path { return c.targetPath }

// LiteralValue returns the product value carrying the sufficient literal.
func (c BranchSufficientLiteralCase) LiteralValue() product.Value { return c.literalValue }

// Edge returns the branch edge selected by this sufficient literal.
func (c BranchSufficientLiteralCase) Edge() bool { return c.edge }

// Cases returns the cases in deterministic order.
func (s BranchSufficientLiteralCaseSet) Cases() []BranchSufficientLiteralCase {
	return copyBranchSufficientLiteralCaseSlice(s.cases)
}

func (s BranchSufficientLiteralCaseSet) copy() BranchSufficientLiteralCaseSet {
	return BranchSufficientLiteralCaseSet{cases: copyBranchSufficientLiteralCaseSlice(s.cases)}
}

func copyBranchSufficientLiteralCaseSlice(in []BranchSufficientLiteralCase) []BranchSufficientLiteralCase {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchSufficientLiteralCase, len(in))
	for i, c := range in {
		c.targetPath = c.targetPath.Clone()
		out[i] = c
	}
	return out
}

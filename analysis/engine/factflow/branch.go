package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ValueRefinement describes one conjunctive product-value constraint. The
// explicit has bit distinguishes "no edge constraint" from a deliberate
// product.Top() no-op constraint.
type ValueRefinement struct {
	constraint    product.Value
	hasConstraint bool
}

// NewValueRefinement creates an empty value refinement.
func NewValueRefinement() ValueRefinement {
	return ValueRefinement{}
}

// NewValueConstraint creates a value refinement from an already-built product
// constraint.
func NewValueConstraint(constraint product.Value) ValueRefinement {
	return ValueRefinement{constraint: constraint, hasConstraint: true}
}

// WithConstraint returns r additionally constrained by constraint.
func (r ValueRefinement) WithConstraint(reg *axis.Registry, constraint product.Value) ValueRefinement {
	if !r.hasConstraint {
		r.constraint = constraint
		r.hasConstraint = true
		return r
	}
	r.constraint = product.Meet(reg, r.constraint, constraint)
	return r
}

// Constraint returns the product constraint, if present.
func (r ValueRefinement) Constraint() (product.Value, bool) {
	return r.constraint, r.hasConstraint
}

// IsEmpty reports whether r carries no axis refinements.
func (r ValueRefinement) IsEmpty() bool {
	return !r.hasConstraint
}

// BranchRefinement describes branch-edge value refinements for one access path.
// Each edge may be independently absent when the condition gives no fact for
// that direction.
type BranchRefinement struct {
	targetPath path.Path

	trueValue    ValueRefinement
	hasTrueValue bool

	falseValue    ValueRefinement
	hasFalseValue bool
}

// NewBranchRefinement creates a branch refinement fact.
func NewBranchRefinement(
	targetPath path.Path,
	trueValue ValueRefinement,
	hasTrueValue bool,
	falseValue ValueRefinement,
	hasFalseValue bool,
) BranchRefinement {
	return BranchRefinement{
		targetPath:    copyPath(targetPath),
		trueValue:     trueValue,
		hasTrueValue:  hasTrueValue,
		falseValue:    falseValue,
		hasFalseValue: hasFalseValue,
	}
}

// TargetPath returns the refined path.
func (r BranchRefinement) TargetPath() path.Path { return copyPath(r.targetPath) }

// TrueValue returns the true-edge value refinement, if present.
func (r BranchRefinement) TrueValue() (ValueRefinement, bool) {
	return r.trueValue, r.hasTrueValue
}

// FalseValue returns the false-edge value refinement, if present.
func (r BranchRefinement) FalseValue() (ValueRefinement, bool) {
	return r.falseValue, r.hasFalseValue
}

// ValueForEdge returns the refinement selected by a CFG branch edge.
func (r BranchRefinement) ValueForEdge(cond bool) (ValueRefinement, bool) {
	if cond {
		return r.TrueValue()
	}
	return r.FalseValue()
}

func (r BranchRefinement) copy() BranchRefinement {
	r.targetPath = copyPath(r.targetPath)
	return r
}

func copyBranchRefinementMap(in map[cfg.Point]BranchRefinement) map[cfg.Point]BranchRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchRefinement, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

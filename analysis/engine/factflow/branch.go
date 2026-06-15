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

	// negatedLiteral marks a descendant literal constraint as the negated edge
	// of a discriminant equality guard: the subject path provably does not hold
	// the constraint literal. The applicator removes the matching variant-origin
	// cases from the root rather than meeting the literal in.
	negatedLiteral bool

	// falsyAbsent marks the falsy edge of a bare truthiness guard (if x then):
	// the subject is falsy, which proves it absent only when its present type can
	// never be the boolean false. The applicator applies the Absent constraint
	// conditionally on that runtime-kind check so a boolean subject is not narrowed.
	falsyAbsent bool
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

// NewNegatedLiteralConstraint creates a value refinement carrying a descendant
// literal the subject path provably does not hold. It drives negated
// discriminant narrowing on the root.
func NewNegatedLiteralConstraint(constraint product.Value) ValueRefinement {
	return ValueRefinement{constraint: constraint, hasConstraint: true, negatedLiteral: true}
}

// NegatedLiteral reports whether the constraint is a provably-absent literal.
func (r ValueRefinement) NegatedLiteral() bool {
	return r.negatedLiteral
}

// NewFalsyAbsentConstraint creates a value refinement carrying an Absent presence
// the applicator applies only when the subject's present type can never be the
// boolean false, i.e. the falsy edge of a bare truthiness guard proves nil.
func NewFalsyAbsentConstraint(constraint product.Value) ValueRefinement {
	return ValueRefinement{constraint: constraint, hasConstraint: true, falsyAbsent: true}
}

// FalsyAbsent reports whether the constraint is a conditional falsy-edge Absent
// proof that holds only for a subject that can never be the boolean false.
func (r ValueRefinement) FalsyAbsent() bool {
	return r.falsyAbsent
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

// BranchLenRefinement records a proven length floor for an array path that
// holds on a branch's true edge: a non-empty/in-range guard such as #xs > 0
// raises len(xs) >= Lo. It is a must-fact applied only on the true edge; the
// false edge and merges do not carry it.
type BranchLenRefinement struct {
	arrayPath path.Path
	lo        int64
}

// NewBranchLenRefinement creates a true-edge length-floor fact for arrayPath.
func NewBranchLenRefinement(arrayPath path.Path, lo int64) BranchLenRefinement {
	return BranchLenRefinement{arrayPath: copyPath(arrayPath), lo: lo}
}

// ArrayPath returns the array path whose length floor this fact raises.
func (r BranchLenRefinement) ArrayPath() path.Path { return copyPath(r.arrayPath) }

// Floor returns the proven lower bound on the array length.
func (r BranchLenRefinement) Floor() int64 { return r.lo }

func (r BranchLenRefinement) copy() BranchLenRefinement {
	r.arrayPath = copyPath(r.arrayPath)
	return r
}

// BranchRefinementSet groups branch-edge refinements emitted at the same CFG
// branch point.
type BranchRefinementSet struct {
	refinements []BranchRefinement
	lenFloors   []BranchLenRefinement
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

// NewBranchRefinementSet creates a branch refinement set.
func NewBranchRefinementSet(refinements ...BranchRefinement) BranchRefinementSet {
	return BranchRefinementSet{refinements: copyBranchRefinementSlice(refinements)}
}

// WithLenRefinements returns s extended with true-edge length-floor facts.
func (s BranchRefinementSet) WithLenRefinements(lenFloors ...BranchLenRefinement) BranchRefinementSet {
	out := s.copy()
	out.lenFloors = append(out.lenFloors, copyBranchLenRefinementSlice(lenFloors)...)
	return out
}

// LenRefinements returns the true-edge length-floor facts in deterministic
// order.
func (s BranchRefinementSet) LenRefinements() []BranchLenRefinement {
	return copyBranchLenRefinementSlice(s.lenFloors)
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

// Refinements returns the branch refinements in deterministic order.
func (s BranchRefinementSet) Refinements() []BranchRefinement {
	return copyBranchRefinementSlice(s.refinements)
}

func (s BranchRefinementSet) copy() BranchRefinementSet {
	return BranchRefinementSet{
		refinements: copyBranchRefinementSlice(s.refinements),
		lenFloors:   copyBranchLenRefinementSlice(s.lenFloors),
	}
}

func copyBranchLenRefinementSlice(in []BranchLenRefinement) []BranchLenRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchLenRefinement, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}

func copyBranchRefinementSetMap(in map[cfg.Point]BranchRefinementSet) map[cfg.Point]BranchRefinementSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchRefinementSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func mergeBranchRefinementSetMap(
	base map[cfg.Point]BranchRefinementSet,
	add map[cfg.Point]BranchRefinementSet,
) map[cfg.Point]BranchRefinementSet {
	out := copyBranchRefinementSetMap(base)
	if len(add) == 0 {
		return out
	}
	if out == nil {
		out = make(map[cfg.Point]BranchRefinementSet, len(add))
	}
	for point, set := range add {
		existing := out[point]
		refinements := existing.Refinements()
		refinements = append(refinements, set.Refinements()...)
		merged := NewBranchRefinementSet(refinements...)
		lenFloors := append(existing.LenRefinements(), set.LenRefinements()...)
		out[point] = merged.WithLenRefinements(lenFloors...)
	}
	return out
}

func copyBranchRefinementSlice(in []BranchRefinement) []BranchRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchRefinement, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}

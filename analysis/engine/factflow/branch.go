package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
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

// HasPresence reports whether r's constraint pins presence to want.
func (r ValueRefinement) HasPresence(want presence.Value) bool {
	constraint, ok := r.Constraint()
	if !ok {
		return false
	}
	return presence.Equal(product.PresenceOf(constraint), want)
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

// BranchNumFloorRefinement records a proven numeric floor for a path that holds
// on a branch's true edge. It is separate from length floors because callers
// often need both facts in an evidence chain, for example i <= #xs and i >= 1
// before treating xs[i] as definitely present.
type BranchNumFloorRefinement struct {
	targetPath path.Path
	lo         int64
}

// NewBranchLenRefinement creates a true-edge length-floor fact for arrayPath.
func NewBranchLenRefinement(arrayPath path.Path, lo int64) BranchLenRefinement {
	return BranchLenRefinement{arrayPath: arrayPath.Clone(), lo: lo}
}

// NewBranchNumFloorRefinement creates a true-edge numeric floor fact for
// targetPath.
func NewBranchNumFloorRefinement(targetPath path.Path, lo int64) BranchNumFloorRefinement {
	return BranchNumFloorRefinement{targetPath: targetPath.Clone(), lo: lo}
}

// ArrayPath returns the array path whose length floor this fact raises.
func (r BranchLenRefinement) ArrayPath() path.Path { return r.arrayPath.Clone() }

// Floor returns the proven lower bound on the array length.
func (r BranchLenRefinement) Floor() int64 { return r.lo }

func (r BranchLenRefinement) copy() BranchLenRefinement {
	r.arrayPath = r.arrayPath.Clone()
	return r
}

// TargetPath returns the numeric path whose floor this fact raises.
func (r BranchNumFloorRefinement) TargetPath() path.Path { return r.targetPath.Clone() }

// Floor returns the proven lower bound on the numeric path.
func (r BranchNumFloorRefinement) Floor() int64 { return r.lo }

func (r BranchNumFloorRefinement) copy() BranchNumFloorRefinement {
	r.targetPath = r.targetPath.Clone()
	return r
}

// BranchDiffConstraint records a relational fact proven on a branch's true edge:
// CoHi*term(Hi) - term(Lo) <= C, where each term is a value path or, when the
// IsLength flag is set, the length of that array path. When HasHi2 is set it
// carries a second positive term and reads CoHi*term(Hi) + CoHi2*term(Hi2) -
// term(Lo) <= C, capturing bounded affine guards such as i + j <= #xs and
// 2*i <= #xs. It captures relational guards like i < j, i + 1 <= #xs, #a == #b,
// i + j <= #xs, and 2*i <= #xs for transitive entailment.
type BranchDiffConstraint struct {
	coHi        int64
	hiPath      path.Path
	hiIsLength  bool
	coHi2       int64
	hi2Path     path.Path
	hi2IsLength bool
	hasHi2      bool
	loPath      path.Path
	loIsLength  bool
	c           int64
}

// NewBranchDiffConstraint creates a true-edge two-term difference-logic fact.
func NewBranchDiffConstraint(hiPath path.Path, hiIsLength bool, loPath path.Path, loIsLength bool, c int64) BranchDiffConstraint {
	return BranchDiffConstraint{coHi: 1, hiPath: hiPath.Clone(), hiIsLength: hiIsLength, loPath: loPath.Clone(), loIsLength: loIsLength, c: c}
}

// NewBranchSumConstraint creates a true-edge bounded three-term fact:
// term(hi) + term(hi2) - term(lo) <= c.
func NewBranchSumConstraint(hiPath path.Path, hiIsLength bool, hi2Path path.Path, hi2IsLength bool, loPath path.Path, loIsLength bool, c int64) BranchDiffConstraint {
	return NewBranchScaledConstraint(1, hiPath, hiIsLength, 1, hi2Path, hi2IsLength, loPath, loIsLength, c)
}

// NewBranchScaledConstraint creates a true-edge bounded affine fact:
// coHi*term(hi) + coHi2*term(hi2) - term(lo) <= c. An empty hi2Path drops the
// second positive term, giving coHi*term(hi) - term(lo) <= c.
func NewBranchScaledConstraint(coHi int64, hiPath path.Path, hiIsLength bool, coHi2 int64, hi2Path path.Path, hi2IsLength bool, loPath path.Path, loIsLength bool, c int64) BranchDiffConstraint {
	return BranchDiffConstraint{
		coHi:        coHi,
		hiPath:      hiPath.Clone(),
		hiIsLength:  hiIsLength,
		coHi2:       coHi2,
		hi2Path:     hi2Path.Clone(),
		hi2IsLength: hi2IsLength,
		hasHi2:      !hi2Path.IsEmpty(),
		loPath:      loPath.Clone(),
		loIsLength:  loIsLength,
		c:           c,
	}
}

func (r BranchDiffConstraint) CoHi() int64        { return r.coHi }
func (r BranchDiffConstraint) HiPath() path.Path  { return r.hiPath.Clone() }
func (r BranchDiffConstraint) HiIsLength() bool   { return r.hiIsLength }
func (r BranchDiffConstraint) CoHi2() int64       { return r.coHi2 }
func (r BranchDiffConstraint) Hi2Path() path.Path { return r.hi2Path.Clone() }
func (r BranchDiffConstraint) Hi2IsLength() bool  { return r.hi2IsLength }
func (r BranchDiffConstraint) HasHi2() bool       { return r.hasHi2 }
func (r BranchDiffConstraint) LoPath() path.Path  { return r.loPath.Clone() }
func (r BranchDiffConstraint) LoIsLength() bool   { return r.loIsLength }
func (r BranchDiffConstraint) C() int64           { return r.c }

func (r BranchDiffConstraint) copy() BranchDiffConstraint {
	r.hiPath = r.hiPath.Clone()
	r.hi2Path = r.hi2Path.Clone()
	r.loPath = r.loPath.Clone()
	return r
}

func copyBranchDiffConstraintSlice(in []BranchDiffConstraint) []BranchDiffConstraint {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchDiffConstraint, len(in))
	for i, fact := range in {
		out[i] = fact.copy()
	}
	return out
}

// BranchRefinementSet groups branch-edge refinements emitted at the same CFG
// branch point.
type BranchRefinementSet struct {
	refinements     []BranchRefinement
	lenFloors       []BranchLenRefinement
	numFloors       []BranchNumFloorRefinement
	diffConstraints []BranchDiffConstraint
}

// WithDiffConstraints returns s extended with true-edge difference-logic facts.
func (s BranchRefinementSet) WithDiffConstraints(diffs ...BranchDiffConstraint) BranchRefinementSet {
	out := s.copy()
	out.diffConstraints = append(out.diffConstraints, copyBranchDiffConstraintSlice(diffs)...)
	return out
}

// DiffConstraints returns the true-edge difference-logic facts.
func (s BranchRefinementSet) DiffConstraints() []BranchDiffConstraint {
	return copyBranchDiffConstraintSlice(s.diffConstraints)
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
		targetPath:    targetPath.Clone(),
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

// WithNumFloorRefinements returns s extended with true-edge numeric-floor facts.
func (s BranchRefinementSet) WithNumFloorRefinements(numFloors ...BranchNumFloorRefinement) BranchRefinementSet {
	out := s.copy()
	out.numFloors = append(out.numFloors, copyBranchNumFloorRefinementSlice(numFloors)...)
	return out
}

// LenRefinements returns the true-edge length-floor facts in deterministic
// order.
func (s BranchRefinementSet) LenRefinements() []BranchLenRefinement {
	return copyBranchLenRefinementSlice(s.lenFloors)
}

// NumFloorRefinements returns the true-edge numeric-floor facts in deterministic
// order.
func (s BranchRefinementSet) NumFloorRefinements() []BranchNumFloorRefinement {
	return copyBranchNumFloorRefinementSlice(s.numFloors)
}

// TargetPath returns the refined path.
func (r BranchRefinement) TargetPath() path.Path { return r.targetPath.Clone() }

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
	r.targetPath = r.targetPath.Clone()
	return r
}

// Refinements returns the branch refinements in deterministic order.
func (s BranchRefinementSet) Refinements() []BranchRefinement {
	return copyBranchRefinementSlice(s.refinements)
}

func (s BranchRefinementSet) copy() BranchRefinementSet {
	return BranchRefinementSet{
		refinements:     copyBranchRefinementSlice(s.refinements),
		lenFloors:       copyBranchLenRefinementSlice(s.lenFloors),
		numFloors:       copyBranchNumFloorRefinementSlice(s.numFloors),
		diffConstraints: copyBranchDiffConstraintSlice(s.diffConstraints),
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

func copyBranchNumFloorRefinementSlice(in []BranchNumFloorRefinement) []BranchNumFloorRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchNumFloorRefinement, len(in))
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
		numFloors := append(existing.NumFloorRefinements(), set.NumFloorRefinements()...)
		out[point] = merged.WithLenRefinements(lenFloors...).WithNumFloorRefinements(numFloors...)
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

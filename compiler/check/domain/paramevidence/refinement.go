package paramevidence

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ParamNarrow is one parameter-narrowing effect a function body proves on every
// live exit: parameter Param, optionally at Segments, satisfies Check when the
// function returns normally. EqParam records a direct parameter equality or
// inequality relation; NotEqual selects the inequality polarity.
type ParamNarrow struct {
	Param    int
	Segments []constraint.Segment
	Check    cfg.CondCheckKind
	TypeKey  narrow.TypeKey
	EqParam  int
	NotEqual bool
	CondArg  bool
	CastType typ.Type
}

// IsParamEquality reports whether e is the canonical direct parameter equality
// effect: on every normal return, parameter Param equals parameter EqParam. The
// equality effect is intentionally pure; combining it with path segments,
// truthiness checks, condition-argument effects, or type casts would mix transfer
// vocabularies and is rejected at projection/application boundaries.
func (e ParamNarrow) IsParamEquality() bool {
	return e.Param >= 0 &&
		e.EqParam >= 0 &&
		e.Param != e.EqParam &&
		!e.NotEqual &&
		len(e.Segments) == 0 &&
		e.Check == cfg.CheckNone &&
		e.TypeKey.IsZero() &&
		!e.CondArg &&
		e.CastType == nil
}

// IsParamInequality reports whether e is the canonical direct parameter
// inequality effect: on every normal return, parameter Param does not equal
// parameter EqParam.
func (e ParamNarrow) IsParamInequality() bool {
	return e.Param >= 0 &&
		e.EqParam >= 0 &&
		e.Param != e.EqParam &&
		e.NotEqual &&
		len(e.Segments) == 0 &&
		e.Check == cfg.CheckNone &&
		e.TypeKey.IsZero() &&
		!e.CondArg &&
		e.CastType == nil
}

// CompareParamNarrow orders parameter-narrowing effects deterministically and
// structurally for finite-set lattice operations.
func CompareParamNarrow(a, b ParamNarrow) int {
	if c := cmp.Compare(a.Param, b.Param); c != 0 {
		return c
	}
	if c := cmp.Compare(a.EqParam, b.EqParam); c != 0 {
		return c
	}
	if c := compareBool(a.NotEqual, b.NotEqual); c != 0 {
		return c
	}
	if c := cmp.Compare(uint(a.Check), uint(b.Check)); c != 0 {
		return c
	}
	if c := compareTypeKey(a.TypeKey, b.TypeKey); c != 0 {
		return c
	}
	if c := compareBool(a.CondArg, b.CondArg); c != 0 {
		return c
	}
	if c := cmp.Compare(typeHash(a.CastType), typeHash(b.CastType)); c != 0 {
		return c
	}
	return compareSegments(a.Segments, b.Segments)
}

// CloneParamNarrow returns a copy whose segment slice cannot alias the source.
func CloneParamNarrow(e ParamNarrow) ParamNarrow {
	if len(e.Segments) > 0 {
		e.Segments = append([]constraint.Segment(nil), e.Segments...)
	}
	return e
}

// SortParamNarrows returns a sorted, compacted copy of in.
func SortParamNarrows(in []ParamNarrow) []ParamNarrow {
	if len(in) == 0 {
		return nil
	}
	out := make([]ParamNarrow, len(in))
	for i, e := range in {
		out[i] = CloneParamNarrow(e)
	}
	slices.SortFunc(out, CompareParamNarrow)
	return compactParamNarrows(out)
}

// ReturnPostconditions is the portable, placeholder-rooted proof a callee
// publishes about a normal return. Transfer may discover these facts through
// local ParamNarrow extraction, but the boundary carrier is the constraint
// language itself: truthiness/nilness, type keys, concrete closed type hashes,
// and placeholder relations.
type ReturnPostconditions struct {
	condition constraint.Condition
	has       bool
}

// ReturnPostconditionsDomain is the proof lattice for facts guaranteed on every
// normal return. Its bottom is "no published facts" (true). Join adds facts by
// conjunction, unlike the path-condition lattice where join is disjunction.
var ReturnPostconditionsDomain = lattice.Lattice[ReturnPostconditions]{
	Bottom: func() ReturnPostconditions { return ReturnPostconditions{} },
	Top:    func() ReturnPostconditions { return ReturnPostconditionsFromCondition(constraint.FalseCondition()) },
	Equal: func(a, b ReturnPostconditions) bool {
		return a.Condition().Equals(b.Condition())
	},
	LessOrEq: func(a, b ReturnPostconditions) bool {
		return a.Condition().Subsumes(b.Condition())
	},
	Join: func(a, b ReturnPostconditions) ReturnPostconditions {
		return ReturnPostconditionsFromCondition(constraint.And(a.Condition(), b.Condition()))
	},
	Meet: nil,
	Widen: func(prev, next ReturnPostconditions) ReturnPostconditions {
		return ReturnPostconditionsFromCondition(constraint.And(prev.Condition(), next.Condition()))
	},
}

// ReturnPostconditionsFromParamNarrows projects solved local parameter effects to
// the portable normal-return vocabulary used at module and call boundaries.
func ReturnPostconditionsFromParamNarrows(narrows []ParamNarrow) ReturnPostconditions {
	var onReturn []constraint.Constraint
	for _, e := range narrows {
		if e.Param < 0 {
			continue
		}
		c, ok := ParamNarrowConstraint(e)
		if !ok {
			continue
		}
		onReturn = append(onReturn, c)
	}
	if len(onReturn) == 0 {
		return ReturnPostconditions{}
	}
	return ReturnPostconditionsFromCondition(constraint.FromConstraints(onReturn...))
}

// ReturnPostconditionsFromCondition imports a portable normal-return condition.
// Only constraints present in every disjunct can be recovered as ParamNarrow facts;
// the full DNF remains available for direct call-site instantiation.
func ReturnPostconditionsFromCondition(cond constraint.Condition) ReturnPostconditions {
	if cond.IsTrue() || (!cond.IsFalse() && !cond.HasConstraints()) {
		return ReturnPostconditions{}
	}
	return ReturnPostconditions{condition: cond, has: true}
}

func CloneReturnPostconditions(p ReturnPostconditions) ReturnPostconditions {
	if !p.HasConstraints() {
		return ReturnPostconditions{}
	}
	cond := p.Condition()
	if cond.IsFalse() {
		return ReturnPostconditionsFromCondition(constraint.FalseCondition())
	}
	disjuncts := make([][]constraint.Constraint, len(cond.Disjuncts))
	for i, disjunct := range cond.Disjuncts {
		disjuncts[i] = append([]constraint.Constraint(nil), disjunct...)
	}
	return ReturnPostconditionsFromCondition(constraint.FromDisjuncts(disjuncts))
}

func (p ReturnPostconditions) HasConstraints() bool {
	return p.has && !p.condition.IsTrue()
}

func (p ReturnPostconditions) Condition() constraint.Condition {
	if !p.HasConstraints() {
		return constraint.TrueCondition()
	}
	return p.condition
}

func (p ReturnPostconditions) Substitute(args []constraint.Path) constraint.Condition {
	if !p.HasConstraints() {
		return constraint.TrueCondition()
	}
	return p.condition.Substitute(args)
}

func (p ReturnPostconditions) FunctionRefinement(terminates bool) *constraint.FunctionRefinement {
	if !p.HasConstraints() && !terminates {
		return nil
	}
	refinement := &constraint.FunctionRefinement{Terminates: terminates}
	if p.HasConstraints() {
		refinement.OnReturn = p.condition
	}
	return refinement
}

// ParamNarrowConstraint maps one body-proven parameter check to the portable
// OnReturn constraint rooted at placeholder $Param.
func ParamNarrowConstraint(e ParamNarrow) (constraint.Constraint, bool) {
	if e.Param < 0 {
		return nil, false
	}
	if e.EqParam >= 0 {
		switch {
		case e.IsParamEquality():
			return constraint.NewEqPath(constraint.ParamPath(e.Param), constraint.ParamPath(e.EqParam)), true
		case e.IsParamInequality():
			return constraint.NewNotEqPath(constraint.ParamPath(e.Param), constraint.ParamPath(e.EqParam)), true
		default:
			return nil, false
		}
	}
	path := constraint.ParamPath(e.Param)
	for _, seg := range e.Segments {
		path = path.Append(seg)
	}
	if e.CastType != nil && !typ.IsAbsentOrUnknown(e.CastType) {
		key := narrow.HashTypeKey(e.CastType.Hash())
		if key.IsZero() {
			return nil, false
		}
		return constraint.HasType{Path: path, Type: key}, true
	}
	switch e.Check {
	case cfg.CheckNil:
		return constraint.IsNil{Path: path}, true
	case cfg.CheckFalsy:
		return constraint.Falsy{Path: path}, true
	case cfg.CheckNotNil:
		return constraint.NotNil{Path: path}, true
	case cfg.CheckTruthy:
		return constraint.Truthy{Path: path}, true
	case cfg.CheckTypeEqual:
		if e.TypeKey.IsZero() {
			return nil, false
		}
		return constraint.HasType{Path: path, Type: e.TypeKey}, true
	case cfg.CheckTypeNot:
		if e.TypeKey.IsZero() {
			return nil, false
		}
		return constraint.NotHasType{Path: path, Type: e.TypeKey}, true
	default:
		return nil, false
	}
}

// ParamNarrowsFromFunctionType recovers parameter-narrowing effects from an
// imported callee signature's portable FunctionRefinement.
func ParamNarrowsFromFunctionType(sig typ.Type) []ParamNarrow {
	fn := unwrap.Function(sig)
	if fn == nil || fn.Refinement == nil {
		return nil
	}
	refinement, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok || refinement == nil || !refinement.OnReturn.HasConstraints() {
		return nil
	}
	return ParamNarrowsFromReturnPostconditions(ReturnPostconditionsFromCondition(refinement.OnReturn))
}

// ReturnPostconditionsFromFunctionType imports a function signature's portable
// normal-return proof without projecting it through the finite ParamNarrow view.
func ReturnPostconditionsFromFunctionType(sig typ.Type) ReturnPostconditions {
	fn := unwrap.Function(sig)
	if fn == nil || fn.Refinement == nil {
		return ReturnPostconditionsDomain.Bottom()
	}
	refinement, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok || refinement == nil || !refinement.OnReturn.HasConstraints() {
		return ReturnPostconditionsDomain.Bottom()
	}
	return ReturnPostconditionsFromCondition(refinement.OnReturn)
}

// ParamNarrowsFromReturnPostconditions recovers the finite ParamNarrow projection
// from imported postconditions. Disjunctive facts that are not guaranteed on every
// return remain in the condition domain instead of being downgraded to a narrow.
func ParamNarrowsFromReturnPostconditions(post ReturnPostconditions) []ParamNarrow {
	if !post.HasConstraints() {
		return nil
	}
	var out []ParamNarrow
	for _, c := range post.Condition().MustConstraints() {
		if e, ok := ParamNarrowFromConstraint(c); ok {
			out = append(out, e)
		}
	}
	return out
}

// ParamNarrowFromConstraint reverses ParamNarrowConstraint for placeholder-rooted
// OnReturn constraints. Non-portable paths and unsupported constraint kinds are
// rejected rather than leaking callee-local identities into caller evidence.
func ParamNarrowFromConstraint(c constraint.Constraint) (ParamNarrow, bool) {
	if relation, ok := constraint.DirectPathRelation(c); ok {
		return paramNarrowFromPathRelation(relation)
	}
	predicate, ok := constraint.SinglePathPredicate(c)
	if !ok || !predicate.Path.IsPlaceholder() {
		return ParamNarrow{}, false
	}
	check, key, ok := condCheckFromPathPredicate(predicate)
	if !ok {
		return ParamNarrow{}, false
	}
	path := predicate.Path
	idx := path.PlaceholderIndex()
	if idx < 0 {
		return ParamNarrow{}, false
	}
	var segs []constraint.Segment
	if len(path.Segments) > 0 {
		segs = append(segs, path.Segments...)
	}
	return ParamNarrow{Param: idx, Segments: segs, Check: check, TypeKey: key, EqParam: -1}, true
}

func paramNarrowFromPathRelation(relation constraint.PathRelation) (ParamNarrow, bool) {
	if !relation.Left.IsPlaceholder() || !relation.Right.IsPlaceholder() || len(relation.Left.Segments) != 0 || len(relation.Right.Segments) != 0 {
		return ParamNarrow{}, false
	}
	a, b := relation.Left.PlaceholderIndex(), relation.Right.PlaceholderIndex()
	if a < 0 || b < 0 || a == b {
		return ParamNarrow{}, false
	}
	equality, ok := relation.IsEquality()
	if !ok {
		return ParamNarrow{}, false
	}
	return ParamNarrow{Param: a, EqParam: b, NotEqual: !equality}, true
}

func condCheckFromPathPredicate(predicate constraint.PathPredicate) (cfg.CondCheckKind, narrow.TypeKey, bool) {
	switch predicate.Kind {
	case constraint.PathPredicateIsNil:
		return cfg.CheckNil, narrow.TypeKey{}, true
	case constraint.PathPredicateFalsy:
		return cfg.CheckFalsy, narrow.TypeKey{}, true
	case constraint.PathPredicateNotNil:
		return cfg.CheckNotNil, narrow.TypeKey{}, true
	case constraint.PathPredicateTruthy:
		return cfg.CheckTruthy, narrow.TypeKey{}, true
	case constraint.PathPredicateHasType:
		return cfg.CheckTypeEqual, predicate.Type, !predicate.Type.IsZero()
	case constraint.PathPredicateNotHasType:
		return cfg.CheckTypeNot, predicate.Type, !predicate.Type.IsZero()
	default:
		return cfg.CheckNone, narrow.TypeKey{}, false
	}
}

func compactParamNarrows(in []ParamNarrow) []ParamNarrow {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	var prev ParamNarrow
	for i, e := range in {
		if i > 0 && CompareParamNarrow(prev, e) == 0 {
			continue
		}
		out = append(out, e)
		prev = e
	}
	return out
}

func compareSegments(a, b []constraint.Segment) int {
	if c := cmp.Compare(len(a), len(b)); c != 0 {
		return c
	}
	for i := range a {
		if c := cmp.Compare(a[i].Kind, b[i].Kind); c != 0 {
			return c
		}
		if c := cmp.Compare(a[i].Name, b[i].Name); c != 0 {
			return c
		}
		if c := cmp.Compare(a[i].Index, b[i].Index); c != 0 {
			return c
		}
	}
	return 0
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a && b:
		return -1
	default:
		return 1
	}
}

func compareTypeKey(a, b narrow.TypeKey) int {
	if c := cmp.Compare(uint(a.Kind), uint(b.Kind)); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Name, b.Name); c != 0 {
		return c
	}
	return cmp.Compare(a.Hash, b.Hash)
}

func typeHash(t typ.Type) uint64 {
	if t == nil {
		return 0
	}
	return t.Hash()
}

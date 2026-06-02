package paramevidence

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
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

// FunctionRefinementFromParamNarrows projects solved parameter-narrowing effects
// to the portable placeholder-rooted FunctionRefinement vocabulary used at module
// export/import boundaries.
func FunctionRefinementFromParamNarrows(narrows []ParamNarrow, terminates bool) *constraint.FunctionRefinement {
	var onReturn []constraint.Constraint
	for _, e := range narrows {
		if e.CondArg || e.CastType != nil || e.Param < 0 {
			continue
		}
		c, ok := ParamNarrowConstraint(e)
		if !ok {
			continue
		}
		onReturn = append(onReturn, c)
	}
	if len(onReturn) == 0 && !terminates {
		return nil
	}
	refinement := &constraint.FunctionRefinement{Terminates: terminates}
	if len(onReturn) > 0 {
		refinement.OnReturn = constraint.FromConstraints(onReturn...)
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
	var out []ParamNarrow
	for _, c := range refinement.OnReturn.MustConstraints() {
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
	switch rel := c.(type) {
	case constraint.EqPath:
		left, right := rel.Left, rel.Right
		if !left.IsPlaceholder() || !right.IsPlaceholder() || len(left.Segments) != 0 || len(right.Segments) != 0 {
			return ParamNarrow{}, false
		}
		a, b := left.PlaceholderIndex(), right.PlaceholderIndex()
		if a < 0 || b < 0 || a == b {
			return ParamNarrow{}, false
		}
		return ParamNarrow{Param: a, EqParam: b}, true
	case constraint.NotEqPath:
		left, right := rel.Left, rel.Right
		if !left.IsPlaceholder() || !right.IsPlaceholder() || len(left.Segments) != 0 || len(right.Segments) != 0 {
			return ParamNarrow{}, false
		}
		a, b := left.PlaceholderIndex(), right.PlaceholderIndex()
		if a < 0 || b < 0 || a == b {
			return ParamNarrow{}, false
		}
		return ParamNarrow{Param: a, EqParam: b, NotEqual: true}, true
	}
	path, check, key, ok := portableConstraintPathAndCheck(c)
	if !ok || !path.IsPlaceholder() {
		return ParamNarrow{}, false
	}
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

func portableConstraintPathAndCheck(c constraint.Constraint) (constraint.Path, cfg.CondCheckKind, narrow.TypeKey, bool) {
	switch v := c.(type) {
	case constraint.IsNil:
		return v.Path, cfg.CheckNil, narrow.TypeKey{}, true
	case constraint.Falsy:
		return v.Path, cfg.CheckFalsy, narrow.TypeKey{}, true
	case constraint.NotNil:
		return v.Path, cfg.CheckNotNil, narrow.TypeKey{}, true
	case constraint.Truthy:
		return v.Path, cfg.CheckTruthy, narrow.TypeKey{}, true
	case constraint.HasType:
		if v.Type.IsZero() {
			return constraint.Path{}, cfg.CheckNone, narrow.TypeKey{}, false
		}
		return v.Path, cfg.CheckTypeEqual, v.Type, true
	case constraint.NotHasType:
		if v.Type.IsZero() {
			return constraint.Path{}, cfg.CheckNone, narrow.TypeKey{}, false
		}
		return v.Path, cfg.CheckTypeNot, v.Type, true
	default:
		return constraint.Path{}, cfg.CheckNone, narrow.TypeKey{}, false
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

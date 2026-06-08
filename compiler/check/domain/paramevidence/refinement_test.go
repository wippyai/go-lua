package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnPostconditionsFunctionRefinementProjectsPortableConstraints(t *testing.T) {
	seg := constraint.Segment{Kind: constraint.SegmentField, Name: "value"}
	refinement := ReturnPostconditionsFromParamNarrows([]ParamNarrow{
		{Param: 1, Segments: []constraint.Segment{seg}, Check: cfg.CheckNotNil, EqParam: -1},
	}).FunctionRefinement(true)
	if refinement == nil {
		t.Fatal("FunctionRefinement returned nil")
	}
	if !refinement.Terminates {
		t.Fatal("Terminates was not preserved")
	}
	want := constraint.NotNil{Path: constraint.ParamPath(1).Field("value")}
	if !containsConstraint(refinement.OnReturn.MustConstraints(), want) {
		t.Fatalf("OnReturn missing %s; got %#v", want.String(), refinement.OnReturn.MustConstraints())
	}
}

func TestReturnPostconditionsFunctionRefinementProjectsParamEquality(t *testing.T) {
	refinement := ReturnPostconditionsFromParamNarrows([]ParamNarrow{
		{Param: 0, EqParam: 1},
	}).FunctionRefinement(false)
	if refinement == nil {
		t.Fatal("FunctionRefinement returned nil")
	}
	want := constraint.NewEqPath(constraint.ParamPath(0), constraint.ParamPath(1))
	if !containsConstraint(refinement.OnReturn.MustConstraints(), want) {
		t.Fatalf("OnReturn missing %s; got %#v", want.String(), refinement.OnReturn.MustConstraints())
	}
}

func TestReturnPostconditionsFunctionRefinementProjectsParamInequality(t *testing.T) {
	refinement := ReturnPostconditionsFromParamNarrows([]ParamNarrow{
		{Param: 0, EqParam: 1, NotEqual: true},
	}).FunctionRefinement(false)
	if refinement == nil {
		t.Fatal("FunctionRefinement returned nil")
	}
	want := constraint.NewNotEqPath(constraint.ParamPath(0), constraint.ParamPath(1))
	if !containsConstraint(refinement.OnReturn.MustConstraints(), want) {
		t.Fatalf("OnReturn missing %s; got %#v", want.String(), refinement.OnReturn.MustConstraints())
	}
}

func TestReturnPostconditionsFunctionRefinementProjectsTypeKey(t *testing.T) {
	key := narrow.BuiltinTypeKey("string")
	refinement := ReturnPostconditionsFromParamNarrows([]ParamNarrow{
		{Param: 0, Check: cfg.CheckTypeEqual, TypeKey: key, EqParam: -1},
	}).FunctionRefinement(false)
	if refinement == nil {
		t.Fatal("FunctionRefinement returned nil")
	}
	want := constraint.HasType{Path: constraint.ParamPath(0), Type: key}
	if !containsConstraint(refinement.OnReturn.MustConstraints(), want) {
		t.Fatalf("OnReturn missing %s; got %#v", want.String(), refinement.OnReturn.MustConstraints())
	}
}

func TestReturnPostconditionsOwnPortableConditionProjection(t *testing.T) {
	post := ReturnPostconditionsFromParamNarrows([]ParamNarrow{
		{Param: 0, Check: cfg.CheckNotNil, EqParam: -1},
		{Param: 1, EqParam: 2},
		{Param: 3, CondArg: true, Check: cfg.CheckTruthy, EqParam: -1},
		{Param: 4, CastType: typ.String, EqParam: -1},
	})
	if !post.HasConstraints() {
		t.Fatal("ReturnPostconditionsFromParamNarrows dropped portable facts")
	}
	got := post.Substitute([]constraint.Path{
		constraint.NewPath(10, "a"),
		constraint.NewPath(11, "b"),
		constraint.NewPath(12, "c"),
	})
	if !containsConstraint(got.MustConstraints(), constraint.NotNil{Path: constraint.NewPath(10, "a")}) {
		t.Fatalf("substituted postcondition missing a ~= nil: %#v", got.MustConstraints())
	}
	if !containsConstraint(got.MustConstraints(), constraint.NewEqPath(constraint.NewPath(11, "b"), constraint.NewPath(12, "c"))) {
		t.Fatalf("substituted postcondition missing b == c: %#v", got.MustConstraints())
	}
}

func TestParamNarrowsFromReturnPostconditionsRecoversOnlyMustFacts(t *testing.T) {
	common := constraint.NotNil{Path: constraint.ParamPath(0)}
	left := constraint.Truthy{Path: constraint.ParamPath(1)}
	right := constraint.Falsy{Path: constraint.ParamPath(1)}
	post := ReturnPostconditionsFromCondition(constraint.FromDisjuncts([][]constraint.Constraint{
		{common, left},
		{common, right},
	}))
	got := ParamNarrowsFromReturnPostconditions(post)
	if len(got) != 1 || got[0].Param != 0 || got[0].Check != cfg.CheckNotNil {
		t.Fatalf("ParamNarrowsFromReturnPostconditions = %#v, want only common not-nil fact", got)
	}
}

func TestReturnPostconditionsDomainJoinsByConjunction(t *testing.T) {
	a := ReturnPostconditionsFromCondition(constraint.FromConstraints(
		constraint.NotNil{Path: constraint.ParamPath(0)},
	))
	b := ReturnPostconditionsFromCondition(constraint.FromConstraints(
		constraint.Truthy{Path: constraint.ParamPath(1)},
	))
	got := ReturnPostconditionsDomain.Join(a, b)
	must := got.Condition().MustConstraints()
	if !containsConstraint(must, constraint.NotNil{Path: constraint.ParamPath(0)}) ||
		!containsConstraint(must, constraint.Truthy{Path: constraint.ParamPath(1)}) {
		t.Fatalf("Join = %v, want both return proofs", got.Condition())
	}
	if !ReturnPostconditionsDomain.LessOrEq(ReturnPostconditionsDomain.Bottom(), got) {
		t.Fatal("bottom/no-proof should be below a stronger proof")
	}
}

func TestReturnPostconditionsFunctionRefinementSkipsInvalidEffects(t *testing.T) {
	if got := ReturnPostconditionsFromParamNarrows(nil).FunctionRefinement(false); got != nil {
		t.Fatalf("empty nonterminating refinement = %#v, want nil", got)
	}
	refinement := ReturnPostconditionsFromParamNarrows(nil).FunctionRefinement(true)
	if refinement == nil || !refinement.Terminates || refinement.OnReturn.HasConstraints() {
		t.Fatalf("terminating-only refinement = %#v, want terminates without constraints", refinement)
	}
	refinement = ReturnPostconditionsFromParamNarrows([]ParamNarrow{
		{Param: 0, EqParam: 0},
		{Param: 0, EqParam: 1, Check: cfg.CheckNotNil},
		{Param: 0, EqParam: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}}},
		{Param: -1, Check: cfg.CheckNotNil, EqParam: -1},
		{Param: 0, Check: cfg.CheckNone, EqParam: -1},
	}).FunctionRefinement(false)
	if refinement != nil {
		t.Fatalf("invalid effects produced refinement %#v", refinement)
	}
}

func TestReturnPostconditionsFunctionRefinementProjectsConditionArgsAndCasts(t *testing.T) {
	refinement := ReturnPostconditionsFromParamNarrows([]ParamNarrow{
		{Param: 0, CondArg: true, Check: cfg.CheckFalsy, EqParam: -1},
		{Param: 1, CastType: typ.String, EqParam: -1},
	}).FunctionRefinement(false)
	if refinement == nil || !refinement.OnReturn.HasConstraints() {
		t.Fatalf("portable condition/cast effects produced %#v, want refinement", refinement)
	}
	must := refinement.OnReturn.MustConstraints()
	if !containsConstraint(must, constraint.Falsy{Path: constraint.ParamPath(0)}) {
		t.Fatalf("OnReturn = %v, want condition argument falsy proof", refinement.OnReturn)
	}
	if !containsConstraint(must, constraint.HasType{Path: constraint.ParamPath(1), Type: narrow.HashTypeKey(typ.String.Hash())}) {
		t.Fatalf("OnReturn = %v, want concrete cast type proof", refinement.OnReturn)
	}
}

func TestParamNarrowsFromFunctionTypeRecoversPortableRefinement(t *testing.T) {
	key := narrow.BuiltinTypeKey("string")
	refinement := &constraint.FunctionRefinement{
		OnReturn: constraint.FromConstraints(
			constraint.IsNil{Path: constraint.ParamPath(0).Field("err")},
			constraint.Truthy{Path: constraint.ParamPath(1)},
			constraint.HasType{Path: constraint.ParamPath(0), Type: key},
		),
	}
	sig := typ.Func().
		Param("err", typ.Any).
		Param("ok", typ.Any).
		WithRefinement(refinement).
		Build()
	got := ParamNarrowsFromFunctionType(sig)
	if !containsNarrow(got, ParamNarrow{
		Param:    0,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
		Check:    cfg.CheckNil,
		EqParam:  -1,
	}) {
		t.Fatalf("missing IsNil field narrow in %#v", got)
	}
	if !containsNarrow(got, ParamNarrow{Param: 1, Check: cfg.CheckTruthy, EqParam: -1}) {
		t.Fatalf("missing Truthy narrow in %#v", got)
	}
	if !containsNarrow(got, ParamNarrow{Param: 0, Check: cfg.CheckTypeEqual, TypeKey: key, EqParam: -1}) {
		t.Fatalf("missing HasType narrow in %#v", got)
	}
}

func TestParamNarrowFromConstraintRecoversParamEquality(t *testing.T) {
	got, ok := ParamNarrowFromConstraint(constraint.NewEqPath(
		constraint.ParamPath(0),
		constraint.ParamPath(1),
	))
	if !ok {
		t.Fatal("ParamNarrowFromConstraint rejected placeholder equality")
	}
	if got.Param != 0 || got.EqParam != 1 {
		t.Fatalf("equality narrow = %#v, want Param=0 EqParam=1", got)
	}
}

func TestParamNarrowFromConstraintRecoversParamInequality(t *testing.T) {
	got, ok := ParamNarrowFromConstraint(constraint.NewNotEqPath(
		constraint.ParamPath(0),
		constraint.ParamPath(1),
	))
	if !ok {
		t.Fatal("ParamNarrowFromConstraint rejected placeholder inequality")
	}
	if got.Param != 0 || got.EqParam != 1 || !got.NotEqual {
		t.Fatalf("inequality narrow = %#v, want Param=0 EqParam=1 NotEqual", got)
	}
}

func TestParamNarrowConstraintRejectsMixedEqualityShape(t *testing.T) {
	cases := []ParamNarrow{
		{Param: 0, EqParam: 0},
		{Param: 0, EqParam: 1, Check: cfg.CheckTruthy},
		{Param: 0, EqParam: 1, Check: cfg.CheckTypeEqual, TypeKey: narrow.BuiltinTypeKey("string")},
		{Param: 0, EqParam: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}}},
		{Param: 0, EqParam: 1, NotEqual: true, Check: cfg.CheckTruthy},
		{Param: 0, EqParam: 1, CondArg: true},
		{Param: 0, EqParam: 1, CastType: typ.String},
	}
	for _, e := range cases {
		if got, ok := ParamNarrowConstraint(e); ok {
			t.Fatalf("ParamNarrowConstraint(%#v) = %v, want reject", e, got)
		}
	}
}

func TestParamNarrowFromConstraintRejectsNonPortableEquality(t *testing.T) {
	cases := []constraint.Constraint{
		constraint.NewEqPath(constraint.ParamPath(0), constraint.ParamPath(0)),
		constraint.NewEqPath(constraint.ParamPath(0).Field("id"), constraint.ParamPath(1)),
		constraint.NewEqPath(constraint.NewPath(7, "local"), constraint.ParamPath(0)),
	}
	for _, c := range cases {
		if got, ok := ParamNarrowFromConstraint(c); ok {
			t.Fatalf("ParamNarrowFromConstraint(%v) = %#v, want reject", c, got)
		}
	}
}

func TestSortParamNarrowsIsDeterministicAndDefensive(t *testing.T) {
	seg := constraint.Segment{Kind: constraint.SegmentField, Name: "value"}
	in := []ParamNarrow{
		{Param: 1, Check: cfg.CheckTruthy, EqParam: -1},
		{Param: 0, Segments: []constraint.Segment{seg}, Check: cfg.CheckNotNil, EqParam: -1},
		{Param: 0, Segments: []constraint.Segment{seg}, Check: cfg.CheckNotNil, EqParam: -1},
	}
	got := SortParamNarrows(in)
	if len(got) != 2 {
		t.Fatalf("SortParamNarrows len = %d, want compacted len 2: %#v", len(got), got)
	}
	if CompareParamNarrow(got[0], got[1]) >= 0 {
		t.Fatalf("SortParamNarrows did not order effects: %#v", got)
	}
	in[1].Segments[0].Name = "mutated"
	if got[0].Segments[0].Name != "value" {
		t.Fatalf("SortParamNarrows returned aliased segments: %#v", got[0].Segments)
	}
}

func containsConstraint(haystack []constraint.Constraint, needle constraint.Constraint) bool {
	for _, c := range haystack {
		if c.Equals(needle) {
			return true
		}
	}
	return false
}

func containsNarrow(haystack []ParamNarrow, needle ParamNarrow) bool {
	for _, e := range haystack {
		if e.Param != needle.Param ||
			e.EqParam != needle.EqParam ||
			e.NotEqual != needle.NotEqual ||
			e.Check != needle.Check ||
			e.TypeKey != needle.TypeKey ||
			e.CondArg != needle.CondArg ||
			(e.CastType == nil) != (needle.CastType == nil) ||
			!segmentsEqual(e.Segments, needle.Segments) {
			continue
		}
		if e.CastType != nil && !typ.TypeEquals(e.CastType, needle.CastType) {
			continue
		}
		return true
	}
	return false
}

func segmentsEqual(a, b []constraint.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

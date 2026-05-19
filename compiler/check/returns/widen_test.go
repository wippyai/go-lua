package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenFacts_DoesNotOverrideSummaryWithNilNarrow(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.Integer}},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: []typ.Type{typ.Nil}},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.FunctionFacts.Summary(1)
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected summary[1]=integer, got %v", got)
	}
}

func TestWidenFacts_ElidesOptionalFromNarrowFunctionFact(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.NewOptional(typ.Integer)}},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: []typ.Type{typ.Integer}},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.FunctionFacts.Summary(1)
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected summary[1]=integer, got %v", got)
	}
}

func TestWidenFacts_RefinesOptionalForFirstOrderFunctionSummary(t *testing.T) {
	prev := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: []typ.Type{typ.NewOptional(typ.Integer)}},
	}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: []typ.Type{typ.Integer}},
	}}

	merged := WidenFacts(prev, next)
	got := merged.FunctionFacts.Summary(1)
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected integer after first-order refinement, got %v", got)
	}
}

func TestWidenFacts_UsesMonotoneJoinForHigherOrderFunctionSummary(t *testing.T) {
	nestedUnknown := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	nestedString := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.String).Build()).
		Build()

	base := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedUnknown).Build()).
		Build()
	refined := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedString).Build()).
		Build()

	prev := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: []typ.Type{base}},
	}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: []typ.Type{refined}},
	}}

	merged := WidenFacts(prev, next)
	got := merged.FunctionFacts.Summary(1)
	if len(got) != 1 || !typ.TypeEquals(got[0], base) {
		t.Fatalf("expected stable upper bound for higher-order return, got %v", got)
	}
}

func TestWidenFacts_InterfaceMethodsDoNotBlockOptionalElision(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	prev := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: []typ.Type{typ.NewOptional(dbType)}},
	}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: []typ.Type{dbType}},
	}}

	merged := WidenFacts(prev, next)
	got := merged.FunctionFacts.Summary(1)
	if len(got) != 1 || !typ.TypeEquals(got[0], dbType) {
		t.Fatalf("expected optional elision for interface return, got %v", got)
	}
}

func TestReturnSummaryMerge_StopsRecursiveContainerReturnGrowth(t *testing.T) {
	recordMap := func(value typ.Type) typ.Type {
		return typ.NewRecord().MapComponent(typ.String, value).Build()
	}
	recordField := func(value typ.Type) typ.Type {
		return typ.NewRecord().Field("value", value).SetOpen(true).Build()
	}

	tests := []struct {
		name   string
		stable typ.Type
		growth typ.Type
	}{
		{
			name:   "map",
			stable: typ.NewMap(typ.String, typ.Any),
			growth: typ.NewMap(typ.String, typ.NewMap(typ.String, typ.Nil)),
		},
		{
			name:   "record map component",
			stable: recordMap(typ.Any),
			growth: recordMap(recordMap(typ.Nil)),
		},
		{
			name:   "record field",
			stable: recordField(typ.Any),
			growth: recordField(recordField(typ.Nil)),
		},
		{
			name:   "array",
			stable: typ.NewArray(typ.Any),
			growth: typ.NewArray(typ.NewArray(typ.Nil)),
		},
		{
			name:   "tuple",
			stable: typ.NewTuple(typ.Any),
			growth: typ.NewTuple(typ.NewTuple(typ.Nil)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := returnsummary.Merge([]typ.Type{tt.stable}, []typ.Type{tt.growth})
			if len(merged) != 1 || !typ.TypeEquals(merged[0], tt.stable) {
				t.Fatalf("expected stable recursive return shape, got %v", merged)
			}
		})
	}
}

func TestReturnSummaryMerge_KeepsNonRecursiveContainerRefinement(t *testing.T) {
	stable := typ.NewMap(typ.String, typ.Any)
	refined := typ.NewMap(typ.String, typ.String)

	merged := returnsummary.Merge([]typ.Type{stable}, []typ.Type{refined})
	if len(merged) != 1 || !typ.TypeEquals(merged[0], refined) {
		t.Fatalf("expected non-recursive map refinement to survive, got %v", merged)
	}
}

func TestWidenCapturedFieldAssigns_NormalizesOptionalFunctionValues(t *testing.T) {
	fn := typ.Func().Param("fn", typ.Unknown).Build()
	merged := WidenCapturedFieldAssigns(nil, api.CapturedFieldAssigns{
		1: {2: {"after_all": typ.NewOptional(fn)}},
	})

	got := merged[1][2]["after_all"]
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("expected optional function value to canonicalize to function, got %v", got)
	}
}

func TestWidenCapturedFieldAssigns_MergesSameShapeFunctionValues(t *testing.T) {
	prevFn := typ.Func().
		Param("name", typ.Unknown).
		Returns(typ.NewRecord().
			Field("full_path", typ.String).
			SetOpen(true).
			Build()).
		Build()
	nextFn := typ.Func().
		Param("name", typ.Unknown).
		Returns(typ.NewRecord().
			Field("full_path", typ.String).
			Field("children", typ.NewArray(typ.Unknown)).
			SetOpen(true).
			Build()).
		Build()

	merged := WidenCapturedFieldAssigns(
		api.CapturedFieldAssigns{1: {2: {"describe": prevFn}}},
		api.CapturedFieldAssigns{1: {2: {"describe": nextFn}}},
	)

	got := merged[1][2]["describe"]
	if _, ok := got.(*typ.Union); ok {
		t.Fatalf("expected function observations to merge, got union %v", got)
	}
	fn, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function, got %T", got)
	}
	if len(fn.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(fn.Returns))
	}
	rec, ok := fn.Returns[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record return, got %T", fn.Returns[0])
	}
	if rec.GetField("full_path") == nil || rec.GetField("children") == nil {
		t.Fatalf("expected merged return fields, got %v", rec)
	}
}

func TestMergeFunctionReturnsIfSameShape_GenericFunctions(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("T", nil).
		Returns(typ.Integer).
		Build()

	mergedType, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if !ok {
		t.Fatal("expected generic same-shape functions to merge")
	}
	merged, ok := mergedType.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function type, got %T", mergedType)
	}
	if len(merged.TypeParams) != 1 || merged.TypeParams[0] == nil || merged.TypeParams[0].Name != "T" {
		t.Fatalf("expected merged generic type parameter T, got %+v", merged.TypeParams)
	}
	if len(merged.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(merged.Returns))
	}
	want := typ.NewUnion(typ.String, typ.Integer)
	if !typ.TypeEquals(merged.Returns[0], want) {
		t.Fatalf("expected merged return %v, got %v", want, merged.Returns[0])
	}
}

func TestMergeFunctionReturnsIfSameShape_GenericTypeParamsMustMatch(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("U", nil).
		Returns(typ.Integer).
		Build()

	_, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if ok {
		t.Fatal("expected mismatched generic params not to merge")
	}
}

func TestFunctionFactMergeType_DoesNotRegressToNarrowerNilReturn(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewOptional(typ.Integer)).
		Build()
	next := typ.Func().
		Returns(typ.Nil).
		Build()

	merged := functionfact.MergeType(prev, next)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function return, got %T", merged)
	}
	if !typ.TypeEquals(fn.Returns[0], typ.NewOptional(typ.Integer)) {
		t.Fatalf("expected integer? return after merge, got %v", fn.Returns[0])
	}
}

func TestMergeFunctionReturnsIfSameShape_NormalizesLeakedTypeParams(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewTypeParam("T", nil)).
		Build()
	next := typ.Func().
		Returns(typ.Integer).
		Build()

	mergedType, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if !ok {
		t.Fatal("expected same-shape functions to merge")
	}
	merged, ok := mergedType.(*typ.Function)
	if !ok || len(merged.Returns) != 1 {
		t.Fatalf("expected merged function return, got %T", mergedType)
	}
	if !typ.TypeEquals(merged.Returns[0], typ.Integer) {
		t.Fatalf("expected leaked type param to normalize to integer, got %v", merged.Returns[0])
	}
}

func TestFunctionFactMergeType_PrefersWiderSupertypeOnSubtypeRelation(t *testing.T) {
	merged := functionfact.MergeType(typ.Integer, typ.Number)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}

	merged = functionfact.MergeType(typ.Number, typ.Integer)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}
}

func TestFunctionFactMergeType_IsCommutativeForIncomparableSignatures(t *testing.T) {
	coarse := typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build()
	refined := typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build()

	forward := functionfact.MergeType(coarse, refined)
	reverse := functionfact.MergeType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestFunctionFactMergeType_AliasInputsUseCanonicalJoin(t *testing.T) {
	coarse := typ.NewAlias("CoarseFn", typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build())
	refined := typ.NewAlias("RefinedFn", typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build())

	forward := functionfact.MergeType(coarse, refined)
	reverse := functionfact.MergeType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative alias merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestFunctionFactMergeType_MapVsOpenRecordUsesCanonicalJoin(t *testing.T) {
	coarse := typ.Func().
		Param("t", typ.NewRecord().SetOpen(true).Build()).
		Returns(typ.String).
		Build()
	refined := typ.Func().
		Param("t", typ.NewMap(typ.String, typ.NewArray(typ.String))).
		Returns(typ.String).
		Build()

	forward := functionfact.MergeType(coarse, refined)
	reverse := functionfact.MergeType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative map/open-record merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestWidenLiteralSigs_DoesNotNarrowComparableSignature(t *testing.T) {
	lit := &ast.FunctionExpr{}

	prev := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Number).Build(),
	}
	next := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Integer).Build(),
	}

	merged := WidenLiteralSigs(prev, next)
	got := merged[lit]
	if got == nil {
		t.Fatal("expected merged literal signature")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(got.Returns))
	}
	if !subtype.IsSubtype(prev[lit].Returns[0], got.Returns[0]) {
		t.Fatalf("expected merged return to be supertype of prev (%v), got %v", prev[lit].Returns[0], got.Returns[0])
	}
	if !subtype.IsSubtype(next[lit].Returns[0], got.Returns[0]) {
		t.Fatalf("expected merged return to be supertype of next (%v), got %v", next[lit].Returns[0], got.Returns[0])
	}
	if typ.TypeEquals(got.Returns[0], next[lit].Returns[0]) {
		t.Fatalf("expected merged return not to regress to narrower next-only type %v", got.Returns[0])
	}
}

func TestWidenLiteralSigs_PrefersMergedSameShapeSignature(t *testing.T) {
	lit := &ast.FunctionExpr{}

	prev := api.LiteralSigs{
		lit: typ.Func().Returns(typ.String).Build(),
	}
	next := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Integer).Build(),
	}

	merged := WidenLiteralSigs(prev, next)
	got := merged[lit]
	if got == nil {
		t.Fatal("expected merged literal signature")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(got.Returns))
	}
	want := typ.NewUnion(typ.String, typ.Integer)
	if !typ.TypeEquals(got.Returns[0], want) {
		t.Fatalf("expected merged return %v, got %v", want, got.Returns[0])
	}
}

func TestWidenLiteralSigs_NormalizesNilBranch(t *testing.T) {
	lit := &ast.FunctionExpr{}
	sig := typ.Func().
		Returns(typ.NewUnion(typ.NewRecord().Build(), typ.String)).
		Build()

	merged := WidenLiteralSigs(nil, api.LiteralSigs{lit: sig})
	got := merged[lit]
	want := maybeWidenFunctionForConvergence(sig)
	if got == nil || !typ.TypeEquals(got, want) {
		t.Fatalf("expected nil-branch literal signature %v to be normalized to %v, got %v", sig, want, got)
	}
}

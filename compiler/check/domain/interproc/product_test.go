package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/domain/value"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenFacts_DoesNotOverrideSummaryWithNilNarrow(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: product.LiftVector([]typ.Type{typ.Integer})},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: product.LiftVector([]typ.Type{typ.Nil})},
		},
	}

	merged := WidenFacts(prev, next)
	got := functionfact.ReturnSummary(merged.FunctionFacts, 1)
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected summary[1]=integer, got %v", got)
	}
}

func TestWidenFacts_ElidesOptionalFromNarrowFunctionFact(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: product.LiftVector([]typ.Type{typ.NewOptional(typ.Integer)})},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: product.LiftVector([]typ.Type{typ.Integer})},
		},
	}

	merged := WidenFacts(prev, next)
	got := functionfact.ReturnSummary(merged.FunctionFacts, 1)
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected summary[1]=integer, got %v", got)
	}
}

func TestWidenFacts_ReplacesEmptyReturnSeedWithMetatableEvidence(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	// nil Metatable means known-absent under the sound subtype order, so a seed
	// representing "no metatable evidence yet" carries Unknown on that axis.
	seed := typ.NewRecord().Metatable(typ.Unknown).Build()
	observed := typ.NewRecord().Metatable(metatable).Build()

	prev := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: product.LiftVector([]typ.Type{seed, typ.Nil}), Narrow: product.LiftVector([]typ.Type{seed, typ.Nil})},
	}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: product.LiftVector([]typ.Type{observed, typ.Nil}), Narrow: product.LiftVector([]typ.Type{observed, typ.Nil})},
	}}

	merged := WidenFacts(prev, next)
	got := functionfact.ReturnSummary(merged.FunctionFacts, 1)
	if len(got) != 2 {
		t.Fatalf("expected two return slots, got %v", got)
	}
	if mt, ok := querycore.Method(got[0], "ready"); !ok {
		t.Fatalf("merged metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}
	narrow := merged.FunctionFacts[1].Narrow
	if len(narrow) != 2 {
		t.Fatalf("expected two narrow slots, got %v", narrow)
	}
	if mt, ok := querycore.Method(narrow[0].ProjectValue(), "ready"); !ok {
		t.Fatalf("merged narrow metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, narrow[0])
	}
}

func TestWidenFacts_RefinesOptionalForFirstOrderFunctionSummary(t *testing.T) {
	prev := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: product.LiftVector([]typ.Type{typ.NewOptional(typ.Integer)})},
	}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: product.LiftVector([]typ.Type{typ.Integer})},
	}}

	merged := WidenFacts(prev, next)
	got := functionfact.ReturnSummary(merged.FunctionFacts, 1)
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
		1: {Summary: product.LiftVector([]typ.Type{base})},
	}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: product.LiftVector([]typ.Type{refined})},
	}}

	merged := WidenFacts(prev, next)
	got := functionfact.ReturnSummary(merged.FunctionFacts, 1)
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
		1: {Summary: product.LiftVector([]typ.Type{typ.NewOptional(dbType)})},
	}}
	next := api.Facts{FunctionFacts: api.FunctionFacts{
		1: {Summary: product.LiftVector([]typ.Type{dbType})},
	}}

	merged := WidenFacts(prev, next)
	got := functionfact.ReturnSummary(merged.FunctionFacts, 1)
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
		1: {2: {"after_all": product.FromType(typ.NewOptional(fn))}},
	})

	got := merged[1][2]["after_all"].ProjectValue()
	if !typ.TypeEquals(got, fn) {
		t.Fatalf("expected optional function value to canonicalize to function, got %v", got)
	}
}

func TestWidenCapturedFieldAssigns_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()
	merged := WidenCapturedFieldAssigns(
		api.CapturedFieldAssigns{1: {2: {"get_x": product.FromType(seed)}}},
		api.CapturedFieldAssigns{1: {2: {"get_x": product.FromType(solved)}}},
	)

	got := merged[1][2]["get_x"].ProjectValue()
	if !typ.TypeEquals(got, solved) {
		t.Fatalf("captured function seed should not dominate solved projection: got %v, want %v", got, solved)
	}
}

func TestWidenCapturedTypes_ReplacesTopSeedWithPreciseSnapshot(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	merged := WidenCapturedTypes(
		api.CapturedTypes{1: product.FromType(typ.Any)},
		api.CapturedTypes{1: product.FromType(entry)},
	)

	if got := merged[1].ProjectValue(); !typ.TypeEquals(got, entry) {
		t.Fatalf("captured top seed should not poison later precise snapshot: got %v, want %v", got, entry)
	}
	again := WidenCapturedTypes(merged, api.CapturedTypes{1: product.FromType(entry)})
	if got := again[1].ProjectValue(); !typ.TypeEquals(got, entry) {
		t.Fatalf("captured precision replacement must be idempotent: got %v, want %v", got, entry)
	}
}

func TestJoinCapturedTypes_ReplacesTopSeedWithPreciseSnapshot(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	merged := JoinCapturedTypes(
		api.CapturedTypes{1: product.FromType(typ.NewOptional(typ.Any))},
		api.CapturedTypes{1: product.FromType(typ.NewOptional(entry))},
	)
	want := typ.NewOptional(entry)
	if got := merged[1].ProjectValue(); !typ.TypeEquals(got, want) {
		t.Fatalf("captured precise snapshot should replace optional top seed: got %v, want %v", got, want)
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
		api.CapturedFieldAssigns{1: {2: {"describe": product.FromType(prevFn)}}},
		api.CapturedFieldAssigns{1: {2: {"describe": product.FromType(nextFn)}}},
	)

	got := merged[1][2]["describe"].ProjectValue()
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

func TestJoinCapturedFieldAssigns_UsesCanonicalRecursiveProductJoin(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("proc", typ.Any).
			Build()
	})

	merged := JoinCapturedFieldAssigns(
		api.CapturedFieldAssigns{1: {2: {"suite": product.FromType(left)}}},
		api.CapturedFieldAssigns{1: {2: {"suite": product.FromType(right)}}},
	)
	got := merged[1][2]["suite"].ProjectValue()
	if _, ok := got.(*typ.Union); ok {
		t.Fatalf("captured field join returned raw recursive union: %v", got)
	}
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("captured field join = %T %[1]v, want recursive product", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	proc := body.GetField("proc")
	if proc == nil || !proc.Optional {
		t.Fatalf("merged recursive body should retain optional proc field, got %v", body)
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
	want := value.WidenFunctionForConvergence(sig)
	if got == nil || !typ.TypeEquals(got, want) {
		t.Fatalf("expected nil-branch literal signature %v to be normalized to %v, got %v", sig, want, got)
	}
}

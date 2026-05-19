package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestJoin_InitialObservation(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()

	got := Join(api.FunctionFact{}, api.FunctionFact{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Type:    fn,
	})

	if !returnsummary.Equal(got.Summary, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got.Summary)
	}
	if !returnsummary.Equal(got.Narrow, []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got.Narrow)
	}
	if !typ.TypeEquals(got.Type, fn) {
		t.Fatalf("func mismatch: got %v", got.Type)
	}
}

func TestJoin_NarrowSummaryReplacesOpenTopPlaceholder(t *testing.T) {
	openTop := typ.NewRecord().SetOpen(true).Build()
	existingFunc := typ.Func().Returns(openTop).Build()
	candidateFunc := typ.Func().Returns(openTop).Build()
	narrow := []typ.Type{typ.NewArray(typ.Unknown)}

	out := Join(
		api.FunctionFact{Summary: []typ.Type{openTop}, Type: existingFunc},
		api.FunctionFact{Summary: []typ.Type{openTop}, Narrow: narrow, Type: candidateFunc},
	)

	if !returnsummary.Equal(returnsummary.NormalizeAndPrune(out.Summary), returnsummary.NormalizeAndPrune(narrow)) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, narrow)
	}
	fn, ok := out.Type.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Type)
	}
	if !returnsummary.Equal(returnsummary.NormalizeAndPrune(fn.Returns), returnsummary.NormalizeAndPrune(narrow)) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, narrow)
	}
}

func TestJoin_NarrowSummaryRepairsNeverArtifact(t *testing.T) {
	bad := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}
	good := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Unknown).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}

	out := Join(
		api.FunctionFact{Summary: bad, Type: typ.Func().Returns(bad...).Build()},
		api.FunctionFact{Narrow: good},
	)

	if !returnsummary.Equal(out.Summary, good) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, good)
	}
	if !returnsummary.Equal(out.Narrow, good) {
		t.Fatalf("narrow mismatch: got %v want %v", out.Narrow, good)
	}
	fn, ok := out.Type.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Type)
	}
	if !returnsummary.Equal(fn.Returns, good) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, good)
	}
}

func TestMergeType_MergesSameShapeReturnsCanonically(t *testing.T) {
	existing := typ.Func().
		Param("x", typ.String).
		Returns(typ.NewOptional(typ.Integer)).
		Build()
	candidate := typ.Func().
		Param("x", typ.String).
		Returns(typ.Integer).
		Build()

	merged := MergeType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if !typ.TypeEquals(fn.Returns[0], typ.Integer) {
		t.Fatalf("expected refined return integer, got %v", fn.Returns[0])
	}
}

func TestMergeType_WidensParamToCoverObservedCallsites(t *testing.T) {
	existing := typ.Func().
		Param("t", typ.NewArray(typ.Any)).
		Returns(typ.String).
		Build()
	candidate := typ.Func().
		Param("t", typ.NewMap(typ.String, typ.Any)).
		Returns(typ.String).
		Build()

	merged := MergeType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if len(fn.Params) != 1 {
		t.Fatalf("expected one param, got %+v", fn.Params)
	}
	if typ.TypeEquals(fn.Params[0].Type, typ.NewArray(typ.Any)) {
		t.Fatalf("expected param widening beyond array-only shape, got %v", fn.Params[0].Type)
	}
	wantMap := typ.NewMap(typ.String, typ.Any)
	if !subtype.IsSubtype(wantMap, fn.Params[0].Type) {
		t.Fatalf("expected merged param to admit map callsite evidence, got %v", fn.Params[0].Type)
	}
}

func TestMergeType_CollapsesMixedFunctionUnionVariants(t *testing.T) {
	base := typ.Func().
		Param("name", typ.Unknown).
		Returns(typ.NewRecord().Field("full_path", typ.String).SetOpen(true).Build()).
		Build()
	withChildren := typ.Func().
		Param("name", typ.Unknown).
		Returns(typ.NewRecord().
			Field("full_path", typ.String).
			Field("children", typ.NewArray(typ.Unknown)).
			SetOpen(true).
			Build()).
		Build()
	withTests := typ.Func().
		Param("name", typ.Unknown).
		Returns(typ.NewRecord().
			Field("full_path", typ.String).
			Field("tests", typ.NewArray(typ.Unknown)).
			SetOpen(true).
			Build()).
		Build()

	merged := MergeType(typ.NewUnion(typ.Nil, base, withChildren), withTests)
	if merged == nil {
		t.Fatal("expected merged type")
	}
	fn := unwrap.Function(merged)
	if fn == nil || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function variant, got %v", merged)
	}
	rec, ok := fn.Returns[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record return, got %T", fn.Returns[0])
	}
	for _, field := range []string{"full_path", "children", "tests"} {
		if rec.GetField(field) == nil {
			t.Fatalf("expected merged field %q in %v", field, rec)
		}
	}
	if merged.Kind() != kind.Optional {
		t.Fatalf("expected nil residual to be preserved as optional, got %v", merged)
	}
}

func TestNormalize_CanonicalizesStoredFunctionFact(t *testing.T) {
	fn := typ.Func().Returns(typ.Number).Build()
	got := Normalize(api.FunctionFact{
		Summary: []typ.Type{nil},
		Narrow:  []typ.Type{typ.Number},
		Type:    fn,
	})

	if !returnsummary.Equal(got.Summary, []typ.Type{typ.Nil}) {
		t.Fatalf("summary mismatch: got %v", got.Summary)
	}
	if !returnsummary.Equal(got.Narrow, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got.Narrow)
	}
	if !typ.TypeEquals(got.Type, fn) {
		t.Fatalf("func mismatch: got %v", got.Type)
	}
}

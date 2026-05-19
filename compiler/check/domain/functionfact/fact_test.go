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

func TestJoin_MergesExistingAndCandidate(t *testing.T) {
	existingFn := typ.Func().Returns(typ.Number).Build()
	candidateFn := typ.Func().Returns(typ.String).Build()
	existing := api.FunctionFact{
		Summary: []typ.Type{typ.Number},
		Narrow:  []typ.Type{typ.Number},
		Type:    existingFn,
	}
	candidate := api.FunctionFact{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Type:    candidateFn,
	}
	got := Join(existing, candidate)

	if !returnsummary.Equal(got.Summary, []typ.Type{typ.NewUnion(typ.Number, typ.String)}) {
		t.Fatalf("summary mismatch: got %v", got.Summary)
	}
	if !returnsummary.Equal(got.Narrow, []typ.Type{typ.NewUnion(typ.Number, typ.String)}) {
		t.Fatalf("narrow mismatch: got %v", got.Narrow)
	}
	if got.Type == nil {
		t.Fatal("expected merged function type")
	}
}

func TestJoin_DoesNotAlignFunctionToNarrowFieldRegression(t *testing.T) {
	withCapturedMethod := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Build()
	flowOnly := typ.NewRecord().
		Field("x", typ.Integer).
		Build()
	existingFunc := typ.Func().Returns(flowOnly).Build()

	out := Join(
		api.FunctionFact{Summary: []typ.Type{withCapturedMethod}, Narrow: []typ.Type{flowOnly}, Type: existingFunc},
		api.FunctionFact{Summary: []typ.Type{withCapturedMethod}, Narrow: []typ.Type{flowOnly}, Type: existingFunc},
	)

	if !returnsummary.Equal(out.Summary, []typ.Type{withCapturedMethod}) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, []typ.Type{withCapturedMethod})
	}
	fn, ok := out.Type.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Type)
	}
	if !returnsummary.Equal(fn.Returns, []typ.Type{withCapturedMethod}) {
		t.Fatalf("func returns should preserve captured method summary, got %v", fn.Returns)
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

func TestMergeType_PrefersConcreteParamOverTopObservation(t *testing.T) {
	existing := typ.Func().
		Param("x", typ.Any).
		Returns(typ.String).
		Build()
	candidate := typ.Func().
		Param("x", typ.String).
		Returns(typ.String).
		Build()

	merged := MergeType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		t.Fatalf("expected param refined to string, got %+v", fn.Params)
	}
}

func TestMergeType_KeepsBaselineOverNestedNilOnlyRegression(t *testing.T) {
	baselineReturn := typ.NewRecord().
		Field("full_path", typ.String).
		Field("parent", typ.Unknown).
		OptField("after_all", typ.Nil).
		SetOpen(true).
		Build()
	candidateReturn := typ.NewRecord().
		Field("full_path", typ.String).
		Field("parent", typ.Nil).
		Field("after_all", typ.Nil).
		SetOpen(true).
		Build()

	baseline := typ.Func().Param("name", typ.Unknown).Returns(baselineReturn).Build()
	candidate := typ.Func().Param("name", typ.Unknown).Returns(candidateReturn).Build()

	merged := MergeType(baseline, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function return, got %v", merged)
	}
	if !typ.TypeEquals(fn.Returns[0], baselineReturn) {
		t.Fatalf("expected baseline record to survive nil-only refinement, got %v", fn.Returns[0])
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

func TestMergeType_DoesNotDropNonFunctionUnionMembers(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.String).Build()
	existing := typ.NewUnion(fn, typ.Number)
	candidate := typ.Func().Param("x", typ.String).Returns(typ.String).Build()

	merged := MergeType(existing, candidate)
	u, ok := merged.(*typ.Union)
	if !ok {
		t.Fatalf("expected union to be preserved, got %T", merged)
	}
	hasNumber := false
	for _, m := range u.Members {
		if typ.TypeEquals(m, typ.Number) {
			hasNumber = true
			break
		}
	}
	if !hasNumber {
		t.Fatalf("expected merged union to retain non-function member, got %v", merged)
	}
}

func TestMergeType_CollapsesCompatibleFunctionVariants(t *testing.T) {
	base := typ.Func().
		OptParam("entries", typ.Any).
		Returns(typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown))).
		Build()
	refinedEntry := typ.NewRecord().Field("id", typ.String).Build()
	refined := typ.Func().
		OptParam("entries", typ.NewArray(refinedEntry)).
		Returns(typ.NewMap(typ.String, typ.NewArray(refinedEntry))).
		Build()

	merged := MergeType(base, refined)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected function after compatible-variant collapse, got %T", merged)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.NewArray(refinedEntry)) {
		t.Fatalf("expected refined param type to win, got %+v", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.NewMap(typ.String, typ.NewArray(refinedEntry))) {
		t.Fatalf("expected refined return map, got %v", fn.Returns)
	}
}

func TestMergeType_DoesNotCollapseParamToNilWhenOptionalInfoExists(t *testing.T) {
	existing := typ.Func().
		OptParam("tests", typ.Nil).
		Returns(typ.Integer).
		Build()
	candidate := typ.Func().
		OptParam("tests", typ.NewOptional(typ.NewArray(typ.Any))).
		Returns(typ.Integer).
		Build()

	merged := MergeType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected function, got %T", merged)
	}
	want := typ.NewOptional(typ.NewArray(typ.Any))
	if len(fn.Params) != 1 || !fn.Params[0].Optional || !typ.TypeEquals(fn.Params[0].Type, want) {
		t.Fatalf("expected optional param slot with type %v, got %+v", want, fn.Params)
	}
}

func TestMergeType_NilDoesNotDominateSoftOptionalParamShape(t *testing.T) {
	softArray := typ.NewOptional(typ.NewUnion(typ.NewArray(typ.Any), typ.NewRecord().SetOpen(true).Build()))
	preciseArray := typ.NewOptional(typ.NewArray(typ.String))

	merged := MergeType(
		typ.Func().OptParam("tests", typ.Nil).Returns(typ.Integer).Build(),
		typ.Func().OptParam("tests", softArray).Returns(typ.Integer).Build(),
	)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if !typ.TypeEquals(fn.Params[0].Type, softArray) {
		t.Fatalf("expected nil observation not to replace soft optional table shape, got %v", fn.Params[0].Type)
	}

	merged = MergeType(
		typ.Func().OptParam("tests", softArray).Returns(typ.Integer).Build(),
		typ.Func().OptParam("tests", preciseArray).Returns(typ.Integer).Build(),
	)
	fn, ok = merged.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if !typ.TypeEquals(fn.Params[0].Type, preciseArray) {
		t.Fatalf("expected precise optional array evidence to replace soft shape, got %v", fn.Params[0].Type)
	}
}

func TestMergeType_ReplacesStaleFalsyMapKeyWithTruthyRefinement(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	stale := typ.NewRecord().
		MapComponent(typ.NewUnion(typ.Boolean, typ.String), typ.NewArray(entry)).
		SetOpen(true).
		Build()
	current := typ.NewRecord().
		MapComponent(typ.String, typ.NewArray(entry)).
		SetOpen(true).
		Build()

	merged := MergeType(
		typ.Func().OptParam("t", stale).Returns(typ.NewArray(typ.NewUnion(typ.Boolean, typ.String))).Build(),
		typ.Func().OptParam("t", current).Returns(typ.NewArray(typ.String)).Build(),
	)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if !typ.TypeEquals(fn.Params[0].Type, current) {
		t.Fatalf("expected truthy-refined map key param %v, got %v", current, fn.Params[0].Type)
	}
}

func TestMergeType_DoesNotRegressToNarrowerNilReturn(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewOptional(typ.Integer)).
		Build()
	next := typ.Func().
		Returns(typ.Nil).
		Build()

	merged := MergeType(prev, next)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function return, got %T", merged)
	}
	if !typ.TypeEquals(fn.Returns[0], typ.NewOptional(typ.Integer)) {
		t.Fatalf("expected integer? return after merge, got %v", fn.Returns[0])
	}
}

func TestMergeType_PrefersWiderSupertypeOnSubtypeRelation(t *testing.T) {
	merged := MergeType(typ.Integer, typ.Number)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}

	merged = MergeType(typ.Number, typ.Integer)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}
}

func TestMergeType_IsCommutativeForIncomparableSignatures(t *testing.T) {
	coarse := typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build()
	refined := typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build()

	forward := MergeType(coarse, refined)
	reverse := MergeType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestMergeType_AliasInputsUseCanonicalJoin(t *testing.T) {
	coarse := typ.NewAlias("CoarseFn", typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build())
	refined := typ.NewAlias("RefinedFn", typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build())

	forward := MergeType(coarse, refined)
	reverse := MergeType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative alias merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestMergeType_MapVsOpenRecordUsesCanonicalJoin(t *testing.T) {
	coarse := typ.Func().
		Param("t", typ.NewRecord().SetOpen(true).Build()).
		Returns(typ.String).
		Build()
	refined := typ.Func().
		Param("t", typ.NewMap(typ.String, typ.NewArray(typ.String))).
		Returns(typ.String).
		Build()

	forward := MergeType(coarse, refined)
	reverse := MergeType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative map/open-record merge result, got forward=%v reverse=%v", forward, reverse)
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

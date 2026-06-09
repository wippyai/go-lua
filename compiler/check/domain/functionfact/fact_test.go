package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func projectFunctionForTest(t *testing.T, ff api.FunctionFact) *typ.Function {
	t.Helper()
	return projectFunctionForModeForTest(t, ff, api.SynthModeDeclared)
}

func projectFunctionForModeForTest(t *testing.T, ff api.FunctionFact, mode api.SynthMode) *typ.Function {
	t.Helper()
	fn := unwrap.Function(ProjectType(ff, ProjectionSibling, mode))
	if fn == nil {
		t.Fatalf("expected function projection, got %v", ProjectType(ff, ProjectionSibling, mode))
	}
	return fn
}

func TestJoin_InitialObservation(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()

	got := Join(api.FunctionFact{}, functionFactTest(
		factPreflowReturns(typ.String),
		factPostflowReturns(typ.String),
		factSignature(fn),
	))

	if !returnsummary.Equal(factPreflowTypesTest(got), []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got.Returns.Preflow)
	}
	if !returnsummary.Equal(factPostflowTypesTest(got), []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got.Returns.Postflow)
	}
	if !typ.TypeEquals(got.Public.Signature, fn) {
		t.Fatalf("signature mismatch: got %v", got.Public.Signature)
	}
}

func TestJoin_BodyPreconditionRemainsPublicContractEvenWithRefinement(t *testing.T) {
	refinement := constraint.NewRefinement([]constraint.Constraint{
		constraint.HasType{Path: constraint.ParamPath(1), Type: narrow.BuiltinTypeKey("string")},
	}, nil, nil)
	out := Join(
		functionFactTest(factCallParams(typ.String, typ.Any)),
		functionFactTest(
			factCallParams(typ.String, typ.String),
			factRefinement(refinement),
			factSignature(typ.Func().
				Param("label", typ.String).
				Param("msg", typ.String).
				Returns(typ.Unknown).
				Build()),
		),
	)

	if len(out.Call.Params) != 2 || !typ.TypeEquals(out.Call.Params[1].ProjectValue(), typ.String) {
		t.Fatalf("expected hard body precondition to remain public, got %v", out.Call.Params)
	}
}

func TestJoin_PublicContractEvidenceDominatesDynamicSeed(t *testing.T) {
	out := Join(
		functionFactTest(factCallParams(typ.Any)),
		functionFactTest(
			factCallParams(typ.String),
			factSignature(typ.Func().Param("value", typ.String).Returns(typ.Unknown).Build()),
		),
	)

	if len(out.Call.Params) != 1 || !typ.TypeEquals(out.Call.Params[0].ProjectValue(), typ.String) {
		t.Fatalf("expected public contract evidence to dominate dynamic seed, got %v", out.Call.Params)
	}
}

func TestMergeExpectedSignature_ContextRequiredParamDropsSeedNilability(t *testing.T) {
	seed := typ.Func().
		OptParam("db", typ.NewOptional(typ.String)).
		Returns(typ.Unknown).
		Build()
	expected := typ.Func().
		Param("db", typ.String).
		Returns(typ.String).
		Build()

	got := MergeExpectedSignature(seed, expected)
	if got == nil || len(got.Params) != 1 || got.Params[0].Optional || !typ.TypeEquals(got.Params[0].Type, typ.String) {
		t.Fatalf("MergeExpectedSignature() param = %#v, want required string", got)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.String) {
		t.Fatalf("MergeExpectedSignature() returns = %v, want string", got.Returns)
	}
}

func TestMergeExpectedSignature_ReturnsUseValueConvergenceLaw(t *testing.T) {
	base := typ.NewRecord().Field("x", typ.Number).Build()
	grown := typ.NewRecord().
		Field("x", typ.Number).
		Field("next", typ.NewRecord().Field("value", base).Build()).
		Build()
	seed := typ.Func().Returns(base).Build()
	expected := typ.Func().Returns(grown).Build()

	got := MergeExpectedSignature(seed, expected)
	if got == nil || len(got.Returns) != 1 {
		t.Fatalf("MergeExpectedSignature() returns = %v, want one return", got)
	}
	if _, ok := got.Returns[0].(*typ.Recursive); !ok {
		t.Fatalf("MergeExpectedSignature() return = %T %[1]v, want value-domain recursive convergence", got.Returns[0])
	}
}

func TestCanonicalPostflowSignature_UsesPublicParamsAndOmitsInferredReturns(t *testing.T) {
	observed := typ.Func().
		Param("value", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Integer).
		Build()
	publicSeed := typ.Func().
		Param("value", typ.String).
		Build()

	got := CanonicalPostflowSignature(observed, publicSeed, []typ.Type{typ.Integer}, false, false)
	if got == nil {
		t.Fatal("CanonicalPostflowSignature() = nil")
	}
	if len(got.Params) != 1 || !typ.TypeEquals(got.Params[0].Type, typ.String) {
		t.Fatalf("params = %v, want public string param", got.Params)
	}
	if got.Variadic != nil {
		t.Fatalf("variadic = %v, want synthetic variadic omitted", got.Variadic)
	}
	if len(got.Returns) != 0 {
		t.Fatalf("returns = %v, want inferred returns omitted from source signature", got.Returns)
	}
}

type panicHashType struct{}

func (panicHashType) Kind() kind.Kind { return kind.Unknown }
func (panicHashType) String() string  { return "panic-hash" }
func (panicHashType) Hash() uint64 {
	panic("undeclared inferred return was normalized into source signature")
}
func (panicHashType) Equals(typ.Type) bool { return false }

func TestCanonicalPostflowSignature_DoesNotHashUndeclaredInferredReturns(t *testing.T) {
	observed := typ.Func().
		Param("value", typ.String).
		Build()

	got := CanonicalPostflowSignature(observed, nil, []typ.Type{panicHashType{}}, false, false)
	if got == nil {
		t.Fatal("CanonicalPostflowSignature() = nil")
	}
	if len(got.Returns) != 0 {
		t.Fatalf("returns = %v, want undeclared inferred returns outside signature authority", got.Returns)
	}
}

func TestCanonicalPostflowSignature_PreservesDeclaredReturnsAndSourceVariadic(t *testing.T) {
	observed := typ.Func().
		Param("value", typ.Any).
		Variadic(typ.String).
		Returns(typ.Integer).
		Build()

	got := CanonicalPostflowSignature(observed, nil, []typ.Type{typ.String}, true, true)
	if got == nil {
		t.Fatal("CanonicalPostflowSignature() = nil")
	}
	if got.Variadic == nil || !typ.TypeEquals(got.Variadic, typ.String) {
		t.Fatalf("variadic = %v, want source string variadic", got.Variadic)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.Integer) {
		t.Fatalf("returns = %v, want declared integer return", got.Returns)
	}
}

func TestJoin_BodyParamsDoNotRewritePublicParams(t *testing.T) {
	bodyParam := typ.NewRecord().OptField("message", typ.String).Build()
	out := Join(
		functionFactTest(factCallParams(typ.Any)),
		functionFactTest(
			factBodyParams(bodyParam),
			factSignature(typ.Func().Param("info", bodyParam).Returns(typ.String).Build()),
		),
	)

	if len(out.Call.Params) != 1 || !typ.TypeEquals(out.Call.Params[0].ProjectValue(), typ.Any) {
		t.Fatalf("public params = %v, want any", out.Call.Params)
	}
	if len(out.Body.Params) != 1 || !typ.TypeEquals(out.Body.Params[0].ProjectValue(), bodyParam) {
		t.Fatalf("body params = %v, want %v", out.Body.Params, bodyParam)
	}
}

func TestJoin_NarrowSummaryDoesNotEraseDynamicReturnSlot(t *testing.T) {
	stream := typ.NewRecord().Field("candidates", typ.NewArray(typ.NewRecord().Build())).Build()
	out := Join(api.FunctionFact{}, functionFactTest(
		factPreflowReturns(typ.Unknown, typ.NewOptional(typ.NewRecord().Field("message", typ.String).Build())),
		factPostflowReturns(typ.NewOptional(stream), typ.Nil),
		factSignature(typ.Func().
			Param("method", typ.Any).
			Returns(typ.Unknown, typ.NewOptional(typ.NewRecord().Field("message", typ.String).Build())).
			Build()),
	))

	fn := projectFunctionForTest(t, out)
	if !typ.TypeEquals(fn.Returns[0], typ.Unknown) {
		t.Fatalf("dynamic return slot collapsed to %v, want unknown", fn.Returns[0])
	}
}

func TestNormalize_InferredPlaceholderAnyRepairsFromNarrowProof(t *testing.T) {
	out := Normalize(functionFactTest(
		factPreflowReturns(typ.Any),
		factPostflowReturns(typ.Integer),
		factSignature(typ.Func().Param("x", typ.Any).Build()),
	))

	if !returnsummary.Equal(factPreflowTypesTest(out), []typ.Type{typ.Integer}) {
		t.Fatalf("summary mismatch: got %v want integer", out.Returns.Preflow)
	}
}

func TestNormalize_DeclaredAnyReturnPreservesGradualContract(t *testing.T) {
	out := Normalize(functionFactTest(
		factPreflowReturns(typ.Any),
		factPostflowReturns(typ.Integer),
		factSignature(typ.Func().Param("x", typ.Any).Returns(typ.Any).Build()),
	))

	if !returnsummary.Equal(factPreflowTypesTest(out), []typ.Type{typ.Any}) {
		t.Fatalf("summary mismatch: got %v want any", out.Returns.Preflow)
	}
}

func TestNormalize_NilOnlyNarrowDoesNotCreateSummary(t *testing.T) {
	out := Normalize(functionFactTest(factPostflowReturns(typ.Nil)))
	if len(out.Returns.Preflow) != 0 {
		t.Fatalf("summary mismatch: got %v want empty", out.Returns.Preflow)
	}
	if !returnsummary.Equal(factPostflowTypesTest(out), []typ.Type{typ.Nil}) {
		t.Fatalf("narrow mismatch: got %v want nil", out.Returns.Postflow)
	}
}

func TestNormalize_MixedArityNarrowDoesNotTruncateSummaryProduct(t *testing.T) {
	dbType := typ.NewRecord().Field("query", typ.Func().Returns(typ.Any).Build()).Build()
	summary := []typ.Type{typ.NewOptional(dbType), typ.NewOptional(typ.LuaError)}
	out := Normalize(functionFactTest(
		factPreflowReturns(summary...),
		factPostflowReturns(dbType),
		factSignature(typ.Func().Returns(typ.Unknown).Build()),
	))

	if !returnsummary.Equal(factPreflowTypesTest(out), summary) {
		t.Fatalf("summary mismatch: got %v want %v", out.Returns.Preflow, summary)
	}
	if !returnsummary.Equal(factPostflowTypesTest(out), []typ.Type{dbType}) {
		t.Fatalf("narrow mismatch: got %v want %v", out.Returns.Postflow, []typ.Type{dbType})
	}
}

func TestWidenForConvergence_MixedArityNarrowDoesNotTruncateSummaryProduct(t *testing.T) {
	dbType := typ.NewRecord().Field("query", typ.Func().Returns(typ.Any).Build()).Build()
	prevSummary := []typ.Type{typ.NewOptional(dbType), typ.NewOptional(typ.LuaError)}
	out := WidenForConvergence(
		functionFactTest(factReturnProjection(prevSummary, prevSummary)),
		functionFactTest(factPreflowReturns(dbType), factPostflowReturns(dbType)),
	)

	if len(out.Returns.Preflow) != 2 {
		t.Fatalf("summary arity mismatch: got %v want two slots", out.Returns.Preflow)
	}
	if !unwrap.IsOptionalLike(out.Returns.Preflow[0].ProjectValue()) {
		t.Fatalf("first slot should remain nilable across return paths, got %v", out.Returns.Preflow[0].ProjectValue())
	}
	if !unwrap.IsOptionalLike(out.Returns.Preflow[1].ProjectValue()) {
		t.Fatalf("second slot should remain nil-padded, got %v", out.Returns.Preflow[1].ProjectValue())
	}
}

func TestWidenForConvergence_ValueErrorProductAdmitsLaterSuccessBranch(t *testing.T) {
	value := typ.NewRecord().Field("y", typ.Integer).Build()
	err := typ.LiteralString("missing")
	next := []typ.Type{typ.NewOptional(value), typ.NewOptional(err)}

	out := WidenForConvergence(
		functionFactTest(factPreflowReturns(typ.Nil, err), factPostflowReturns(typ.Nil, err)),
		functionFactTest(
			factReturnProjection(next, next),
			factSignature(typ.Func().Param("name", typ.Any).Returns(next...).Build()),
		),
	)

	if !returnsummary.Equal(factPreflowTypesTest(out), next) {
		t.Fatalf("summary = %v, want %v", out.Returns.Preflow, next)
	}
	if !returnsummary.Equal(factPostflowTypesTest(out), next) {
		t.Fatalf("narrow = %v, want %v", out.Returns.Postflow, next)
	}
}

func TestWidenForConvergence_EquivalentAliasAndStructuralNarrowConverges(t *testing.T) {
	eventStruct := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("message")).Field("id", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("tool")).Field("id", typ.String).Build(),
	)
	eventAlias := typ.NewAlias("Event", eventStruct)
	aliasReturn := typ.NewOptional(eventAlias)
	structReturn := typ.NewOptional(eventStruct)

	out := WidenForConvergence(
		functionFactTest(factPreflowReturns(aliasReturn), factPostflowReturns(structReturn)),
		functionFactTest(factPreflowReturns(structReturn), factPostflowReturns(aliasReturn)),
	)

	if !returnsummary.Equal(factPreflowTypesTest(out), factPostflowTypesTest(out)) {
		t.Fatalf("summary/narrow equivalent representatives diverged:\nsummary=%v\nnarrow=%v", out.Returns.Preflow, out.Returns.Postflow)
	}
	if len(out.Returns.Preflow) != 1 || !returnSlotHasAliasSurface(out.Returns.Preflow[0].ProjectValue()) {
		t.Fatalf("expected alias surface to be the canonical representative, got %v", out.Returns.Preflow)
	}

	next := WidenForConvergence(out, functionFactTest(factPreflowReturns(structReturn), factPostflowReturns(aliasReturn)))
	if !returnsummary.Equal(factPreflowTypesTest(next), factPreflowTypesTest(out)) || !returnsummary.Equal(factPostflowTypesTest(next), factPostflowTypesTest(out)) {
		t.Fatalf("equivalent alias/struct repair must be idempotent:\nout=%v/%v\nnext=%v/%v", out.Returns.Preflow, out.Returns.Postflow, next.Returns.Preflow, next.Returns.Postflow)
	}
}

func TestWidenForConvergence_RecursiveSummaryNarrowRepairIsCoinductive(t *testing.T) {
	summaryNode := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("id", typ.String).
			Field("children", typ.NewArray(self)).
			MapComponent(typ.String, typ.NewOptional(self)).
			Build()
	})
	narrowNode := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("id", typ.String).
			Field("children", typ.NewArray(self)).
			MapComponent(typ.String, typ.NewOptional(self)).
			Build()
	})
	aliasReturn := typ.NewOptional(typ.NewAlias("Suite", summaryNode))
	structReturn := typ.NewOptional(narrowNode)

	out := WidenForConvergence(
		functionFactTest(factPreflowReturns(aliasReturn), factPostflowReturns(structReturn)),
		functionFactTest(factPreflowReturns(structReturn), factPostflowReturns(aliasReturn)),
	)

	if !returnsummary.Equal(factPreflowTypesTest(out), factPostflowTypesTest(out)) {
		t.Fatalf("recursive summary/narrow repair diverged:\nsummary=%v\nnarrow=%v", out.Returns.Preflow, out.Returns.Postflow)
	}
	if len(out.Returns.Preflow) != 1 || !returnSlotHasAliasSurface(out.Returns.Preflow[0].ProjectValue()) {
		t.Fatalf("expected alias surface to remain canonical, got %v", out.Returns.Preflow)
	}
}

func returnSlotHasAliasSurface(ret typ.Type) bool {
	switch v := typ.UnwrapAnnotated(ret).(type) {
	case *typ.Alias:
		return true
	case *typ.Optional:
		return returnSlotHasAliasSurface(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if returnSlotHasAliasSurface(member) {
				return true
			}
		}
	}
	return false
}

func TestNormalize_NarrowRequiredFieldRepairsOptionalPresence(t *testing.T) {
	summary := typ.NewUnion(
		typ.NewRecord().
			Field("success", typ.True).
			Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
			Build(),
		typ.NewRecord().
			Field("success", typ.False).
			Field("error", typ.LiteralString("missing")).
			Build(),
	)
	narrow := typ.NewUnion(
		typ.NewRecord().
			Field("success", typ.True).
			Field("result", typ.NewRecord().Field("data", typ.Unknown).Build()).
			Build(),
		typ.NewRecord().
			Field("success", typ.False).
			Field("error", typ.LiteralString("missing")).
			Build(),
	)
	want := typ.NewUnion(
		typ.NewRecord().
			Field("success", typ.True).
			Field("result", typ.NewRecord().Field("data", typ.Unknown).Build()).
			Build(),
		typ.NewRecord().
			Field("success", typ.False).
			Field("error", typ.LiteralString("missing")).
			Build(),
	)

	out := Normalize(functionFactTest(factPreflowReturns(summary), factPostflowReturns(narrow)))
	if !returnsummary.Equal(factPreflowTypesTest(out), []typ.Type{want}) {
		t.Fatalf("summary mismatch: got %v want %v", out.Returns.Preflow, []typ.Type{want})
	}
}

func TestWidenForConvergence_PreservesBodyStructuralParamPrecision(t *testing.T) {
	publicParam := typ.NewOptional(typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.String).
			Field("content", typ.String).
			Field("function_call_id", typ.String).
			Build(),
		typ.NewRecord().
			Field("role", typ.String).
			Field("content", typ.String).
			Build(),
	))
	bodyContract := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.String).
			Field("content", typ.String).
			Field("function_call_id", typ.String).
			Build(),
		typ.NewRecord().
			Field("role", typ.String).
			Field("content", typ.String).
			Build(),
	)
	entryParam := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.LiteralString("function_result")).
			Field("content", typ.LiteralString("ok")).
			Field("function_call_id", typ.LiteralString("tool")).
			Build(),
		typ.NewRecord().
			Field("role", typ.LiteralString("developer")).
			Field("content", typ.LiteralString("merge")).
			Build(),
	)
	ret := typ.NewArray(typ.NewRecord().Field("role", typ.LiteralString("user")).Build())

	out := WidenForConvergence(
		functionFactTest(
			factCallParams(publicParam),
			factPreflowReturns(ret),
			factPostflowReturns(ret),
			factSignature(typ.Func().Param("messages", publicParam).Returns(ret).Build()),
		),
		functionFactTest(
			factCallParams(publicParam),
			factBodyParams(bodyContract),
			factEntryParams(entryParam),
			factPreflowReturns(ret),
			factPostflowReturns(ret),
			factSignature(typ.Func().Param("messages", publicParam).Returns(ret).Build()),
		),
	)

	bodyFn := unwrap.Function(ProjectType(out, ProjectionBody, api.SynthModeDeclared))
	if bodyFn == nil || !typ.TypeEquals(bodyFn.Params[0].Type, entryParam) {
		t.Fatalf("expected body-effective function type to preserve entry %v, got %v", entryParam, bodyFn)
	}
	if len(out.Call.Params) != 1 || !typ.TypeEquals(out.Call.Params[0].ProjectValue(), publicParam) {
		t.Fatalf("expected public call-boundary params to remain %v, got %v", publicParam, out.Call.Params)
	}
	if len(out.Body.Params) != 1 || !typ.TypeEquals(out.Body.Params[0].ProjectValue(), bodyContract) {
		t.Fatalf("expected body contract to remain %v, got %v", bodyContract, out.Body.Params)
	}
	if len(out.Entry.Params) != 1 || !typ.TypeEquals(out.Entry.Params[0].ProjectValue(), entryParam) {
		t.Fatalf("expected entry params to remain %v, got %v", entryParam, out.Entry.Params)
	}
}

func TestWidenForConvergence_SelfEmbeddingTupleParamTerminates(t *testing.T) {
	record := typ.NewRecord().Field("id", typ.String).Build()
	tuple := typ.NewTuple(record)
	nested := typ.NewTuple(tuple)

	out := WidenForConvergence(
		functionFactTest(factSignature(typ.Func().Param("specs", tuple).Returns(typ.Unknown).Build())),
		functionFactTest(factSignature(typ.Func().Param("specs", nested).Returns(typ.Unknown).Build())),
	)

	fn := projectFunctionForTest(t, out)
	got := fn.Params[0].Type
	// Soundness: the widened parameter must be a terminating upper bound that
	// covers BOTH observed shapes — the shallow tuple and the once-nested tuple.
	gotAV := product.FromType(got)
	coversTuple := product.Domain.LessOrEq(product.FromType(tuple), gotAV)
	coversNested := product.Domain.LessOrEq(product.FromType(nested), gotAV)
	t.Logf("widened self-embedding tuple param = %v (coversTuple=%v coversNested=%v)", got, coversTuple, coversNested)
	if !coversTuple || !coversNested {
		t.Fatalf("widened tuple param %v does not cover both (record) and ((record)): coversTuple=%v coversNested=%v", got, coversTuple, coversNested)
	}
}

func TestWidenForConvergence_CurrentNarrowRepairsStaleInferredMapKey(t *testing.T) {
	stale := typ.NewMap(typ.Any, typ.Any)
	solved := typ.NewMap(typ.String, typ.Any)

	out := WidenForConvergence(
		functionFactTest(factPreflowReturns(stale), factPostflowReturns(stale), factSignature(typ.Func().Build())),
		functionFactTest(factPreflowReturns(solved), factPostflowReturns(solved), factSignature(typ.Func().Build())),
	)

	if !returnsummary.Equal(factPostflowTypesTest(out), []typ.Type{solved}) {
		t.Fatalf("narrow mismatch: got %v want %v", out.Returns.Postflow, []typ.Type{solved})
	}
	if !returnsummary.Equal(factPreflowTypesTest(out), []typ.Type{solved}) {
		t.Fatalf("summary mismatch: got %v want %v", out.Returns.Preflow, []typ.Type{solved})
	}
}

func TestWidenForConvergence_DeclaredDynamicMapReturnPreserved(t *testing.T) {
	declared := typ.NewMap(typ.Any, typ.Any)
	solved := typ.NewMap(typ.String, typ.Any)

	out := WidenForConvergence(
		functionFactTest(factPreflowReturns(declared), factPostflowReturns(declared), factSignature(typ.Func().Returns(declared).Build())),
		functionFactTest(factPreflowReturns(solved), factPostflowReturns(solved), factSignature(typ.Func().Returns(declared).Build())),
	)

	if !returnsummary.Equal(factPreflowTypesTest(out), []typ.Type{declared}) {
		t.Fatalf("declared summary mismatch: got %v want %v", out.Returns.Preflow, []typ.Type{declared})
	}
	fn := projectFunctionForModeForTest(t, out, api.SynthModeFlow)
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], declared) {
		t.Fatalf("declared projection return = %v, want %v", fn.Returns, []typ.Type{declared})
	}
}

func TestProjectionExport_UnannotatedDynamicAnyReturnFieldsBecomeUnknown(t *testing.T) {
	stale := typ.NewOptional(typ.NewRecord().
		Field("max_tokens", typ.Any).
		Field("output_tokens", typ.Any).
		Build())
	observed := typ.NewRecord().
		Field("max_tokens", typ.Any).
		Field("output_tokens", typ.Any).
		Field("dimensions", typ.Number).
		Build()

	out := WidenForConvergence(
		functionFactTest(factPreflowReturns(stale), factPostflowReturns(stale), factSignature(typ.Func().Build())),
		functionFactTest(factPreflowReturns(observed), factPostflowReturns(observed), factSignature(typ.Func().Build())),
	)

	sibling := unwrap.Function(ProjectType(out, ProjectionSibling, api.SynthModeFlow))
	if sibling == nil || len(sibling.Returns) != 1 {
		t.Fatalf("sibling projection = %v, want one return", sibling)
	}
	siblingRec, ok := unwrap.Optional(sibling.Returns[0]).(*typ.Record)
	if !ok {
		t.Fatalf("sibling return = %v, want record", sibling.Returns)
	}
	if field := siblingRec.GetField("max_tokens"); field == nil || !typ.TypeEquals(field.Type, typ.Any) {
		t.Fatalf("sibling max_tokens = %v, want any", field)
	}

	exported := unwrap.Function(ProjectType(out, ProjectionExport, api.SynthModeFlow))
	if exported == nil || len(exported.Returns) != 1 {
		t.Fatalf("export projection = %v, want one return", exported)
	}
	ret := unwrap.Optional(exported.Returns[0])
	rec, ok := ret.(*typ.Record)
	if !ok {
		t.Fatalf("summary return = %T %[1]v, want record", ret)
	}
	for _, name := range []string{"max_tokens", "output_tokens"} {
		field := rec.GetField(name)
		if field == nil || !typ.TypeEquals(field.Type, typ.Unknown) {
			t.Fatalf("field %s = %v, want unknown", name, field)
		}
	}
}

func TestJoin_NarrowSummaryReplacesOpenTopPlaceholder(t *testing.T) {
	openTop := typ.NewRecord().SetOpen(true).Build()
	existingFunc := typ.Func().Build()
	candidateFunc := typ.Func().Build()
	narrow := []typ.Type{typ.NewArray(typ.Unknown)}

	out := Join(
		functionFactTest(factPreflowReturns(openTop), factSignature(existingFunc)),
		functionFactTest(factPreflowReturns(openTop), factPostflowReturns(narrow...), factSignature(candidateFunc)),
	)

	if !returnsummary.Equal(returnsummary.NormalizeAndPrune(factPreflowTypesTest(out)), returnsummary.NormalizeAndPrune(narrow)) {
		t.Fatalf("summary mismatch: got %v want %v", out.Returns.Preflow, narrow)
	}
	fn := projectFunctionForTest(t, out)
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
		functionFactTest(factPreflowReturns(bad...), factSignature(typ.Func().Build())),
		functionFactTest(factPostflowReturns(good...)),
	)

	if !returnsummary.Equal(factPreflowTypesTest(out), good) {
		t.Fatalf("summary mismatch: got %v want %v", out.Returns.Preflow, good)
	}
	if !returnsummary.Equal(factPostflowTypesTest(out), good) {
		t.Fatalf("narrow mismatch: got %v want %v", out.Returns.Postflow, good)
	}
	fn := projectFunctionForTest(t, out)
	if !returnsummary.Equal(fn.Returns, good) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, good)
	}
}

func TestJoin_MergesExistingAndCandidate(t *testing.T) {
	existingFn := typ.Func().Returns(typ.Number).Build()
	candidateFn := typ.Func().Returns(typ.String).Build()
	existing := functionFactTest(factPreflowReturns(typ.Number), factPostflowReturns(typ.Number), factSignature(existingFn))
	candidate := functionFactTest(factPreflowReturns(typ.String), factPostflowReturns(typ.String), factSignature(candidateFn))
	got := Join(existing, candidate)

	if !returnsummary.Equal(factPreflowTypesTest(got), []typ.Type{typ.NewUnion(typ.Number, typ.String)}) {
		t.Fatalf("summary mismatch: got %v", got.Returns.Preflow)
	}
	if !returnsummary.Equal(factPostflowTypesTest(got), []typ.Type{typ.NewUnion(typ.Number, typ.String)}) {
		t.Fatalf("narrow mismatch: got %v", got.Returns.Postflow)
	}
	if got.Public.Signature == nil {
		t.Fatal("expected merged function signature")
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
	existingFunc := typ.Func().Build()

	out := Join(
		functionFactTest(factPreflowReturns(withCapturedMethod), factPostflowReturns(flowOnly), factSignature(existingFunc)),
		functionFactTest(factPreflowReturns(withCapturedMethod), factPostflowReturns(flowOnly), factSignature(existingFunc)),
	)

	if !returnsummary.Equal(factPreflowTypesTest(out), []typ.Type{withCapturedMethod}) {
		t.Fatalf("summary mismatch: got %v want %v", out.Returns.Preflow, []typ.Type{withCapturedMethod})
	}
	fn := projectFunctionForTest(t, out)
	if !returnsummary.Equal(fn.Returns, []typ.Type{withCapturedMethod}) {
		t.Fatalf("func returns should preserve captured method summary, got %v", fn.Returns)
	}
}

func TestBestNarrowUnionMember_AlignsRecursiveEvidenceFamily(t *testing.T) {
	summarySuite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	narrowSuite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	other := typ.NewRecursive("Registry", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("items", typ.NewArray(self)).
			Build()
	})

	members := []typ.Type{other, narrowSuite}
	state := newRepairTypeState()
	got := state.bestNarrowUnionMember(summarySuite, members, state.newRepairUnionMemberIndex(members))
	if got != narrowSuite {
		t.Fatalf("bestNarrowUnionMember() = %v, want recursive Suite family", got)
	}
}

func TestBestNarrowUnionMember_NoFamilyMatchPreservesSummary(t *testing.T) {
	summary := typ.NewRecord().Field("role", typ.LiteralString("developer")).Build()
	narrowMember := typ.NewRecord().Field("role", typ.LiteralString("user")).Build()

	members := []typ.Type{narrowMember}
	state := newRepairTypeState()
	got := state.bestNarrowUnionMember(summary, members, state.newRepairUnionMemberIndex(members))
	if got != summary {
		t.Fatalf("bestNarrowUnionMember() = %v, want original summary branch", got)
	}
}

func TestRepairUnionFamilyComparisonTerminatesOnRecursiveKey(t *testing.T) {
	left := typ.NewRecursive("Key", func(self typ.Type) typ.Type {
		return typ.NewOptional(self)
	})
	right := typ.NewRecursive("Key", func(self typ.Type) typ.Type {
		return typ.NewOptional(self)
	})

	state := newRepairTypeState()
	if !state.sameRepairUnionFamily(left, right) {
		t.Fatal("equivalent recursive repair family should compare without unbounded descent")
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

func TestMergeType_ReplacesUnsolvedFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()

	merged := MergeType(seed, solved)
	if !typ.TypeEquals(merged, solved) {
		t.Fatalf("MergeType(seed, solved) = %v, want %v", merged, solved)
	}
	merged = WidenTypeForConvergence(seed, solved)
	if !typ.TypeEquals(merged, solved) {
		t.Fatalf("WidenTypeForConvergence(seed, solved) = %v, want %v", merged, solved)
	}
}

func TestWidenForConvergence_ReplacesReturnFieldFunctionSeed(t *testing.T) {
	seed := typ.Func().Build()
	solved := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	weak := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", seed).
		Build()
	strong := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", solved).
		Build()

	got := WidenForConvergence(
		functionFactTest(factPreflowReturns(weak), factPostflowReturns(weak)),
		functionFactTest(factPreflowReturns(strong), factPostflowReturns(strong)),
	)

	if !returnsummary.Equal(factPreflowTypesTest(got), []typ.Type{strong}) {
		t.Fatalf("summary mismatch: got %v want %v", got.Returns.Preflow, []typ.Type{strong})
	}
	if !returnsummary.Equal(factPostflowTypesTest(got), []typ.Type{strong}) {
		t.Fatalf("narrow mismatch: got %v want %v", got.Returns.Postflow, []typ.Type{strong})
	}
}

func TestMergeType_MergesCallbackEnvOverlaySpec(t *testing.T) {
	coarseSpec := contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
		"up": typ.Unknown,
	}))
	preciseUp := typ.Func().Param("fn", typ.Func().Param("db", typ.String).Build()).Build()
	preciseSpec := contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
		"up": preciseUp,
	}))
	coarse := typ.Func().Param("fn", typ.Func().Build()).Spec(coarseSpec).Build()
	precise := typ.Func().Param("fn", typ.Func().Build()).Spec(preciseSpec).Build()

	merged, ok := MergeType(coarse, precise).(*typ.Function)
	if !ok {
		t.Fatalf("MergeType() = %T, want function", merged)
	}
	spec := contract.ExtractSpec(merged)
	if spec == nil {
		t.Fatal("merged spec missing")
	}
	cb := spec.GetCallback(0)
	if cb == nil || !typ.TypeEquals(cb.EnvOverlay["up"], preciseUp) {
		t.Fatalf("merged callback overlay = %v, want precise up", cb)
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

func TestRepairSummaryWithNarrowHasNoDepthCap(t *testing.T) {
	depth := typ.DefaultRecursionDepth + 4
	summary := nestedFieldType(depth, typ.Any)
	narrow := nestedFieldType(depth, typ.String)

	got := repairSummaryWithNarrow([]typ.Type{summary}, []typ.Type{narrow})
	if len(got) != 1 {
		t.Fatalf("expected one repaired slot, got %d", len(got))
	}
	leaf := nestedFieldLeaf(t, got[0], depth)
	if !typ.TypeEquals(leaf, typ.Any) {
		t.Fatalf("deep repair leaf = %v, want any preserved beyond default depth", leaf)
	}
}

func TestRepairSummaryWithNarrowPreservesUnchangedRecordNode(t *testing.T) {
	rec := typ.NewRecord().
		Field("name", typ.String).
		Field("count", typ.Integer).
		Build()

	got := repairSummaryWithNarrow([]typ.Type{rec}, []typ.Type{rec})
	if len(got) != 1 {
		t.Fatalf("expected one repaired slot, got %d", len(got))
	}
	if got[0] != rec {
		t.Fatalf("unchanged repair rebuilt record node")
	}
}

func TestRepairSummaryWithNarrowRefinesMetatableSlot(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	summary := typ.NewRecord().Metatable(typ.Unknown).Build()
	narrow := typ.NewRecord().Metatable(metatable).Build()

	got := repairSummaryWithNarrow([]typ.Type{summary}, []typ.Type{narrow})
	if len(got) != 1 {
		t.Fatalf("expected one repaired slot, got %d", len(got))
	}
	if mt, ok := querycore.Method(got[0], "ready"); !ok {
		t.Fatalf("repaired metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got[0])
	}
}

func TestWidenForConvergence_RefinesUnknownReturnMetatableEvidence(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	metatable := typ.NewRecord().Field("__index", prototype).Build()
	weak := typ.NewRecord().Metatable(typ.Unknown).Build()
	strong := typ.NewRecord().Metatable(metatable).Build()

	got := WidenForConvergence(
		functionFactTest(factPreflowReturns(weak), factPostflowReturns(weak)),
		functionFactTest(factPreflowReturns(strong), factPostflowReturns(strong)),
	)
	if mt, ok := querycore.Method(got.Returns.Preflow[0].ProjectValue(), "ready"); !ok {
		t.Fatalf("widened summary metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got.Returns.Preflow[0].ProjectValue())
	}
	if mt, ok := querycore.Method(got.Returns.Postflow[0].ProjectValue(), "ready"); !ok {
		t.Fatalf("widened narrow metatable method ready = %v ok=%v, want inherited method on %v", mt, ok, got.Returns.Postflow[0].ProjectValue())
	}
}

func TestRepairSummaryWithNarrowPreservesUnchangedRecursiveRecordNode(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	rec := typ.NewRecord().
		Field("root", node).
		Field("items", typ.NewArray(node)).
		Build()

	got := repairSummaryWithNarrow([]typ.Type{rec}, []typ.Type{rec})
	if len(got) != 1 {
		t.Fatalf("expected one repaired slot, got %d", len(got))
	}
	if got[0] != rec {
		t.Fatalf("unchanged recursive repair rebuilt record node")
	}
}

func TestRepairSummaryWithNarrowPreservesSemanticallyEqualUnionProducts(t *testing.T) {
	summaryRecord := typ.NewRecord().Field("id", typ.String).Build()
	summaryArray := typ.NewArray(summaryRecord)
	summaryMap := typ.NewMap(typ.String, typ.NewOptional(summaryRecord))
	summary := typ.NewUnion(summaryArray, summaryMap)

	narrowRecord := typ.NewRecord().Field("id", typ.String).Build()
	narrowArray := typ.NewArray(narrowRecord)
	narrowMap := typ.NewMap(typ.String, typ.NewOptional(narrowRecord))
	narrow := typ.NewUnion(narrowArray, narrowMap)

	got := repairSummaryWithNarrow([]typ.Type{summary}, []typ.Type{narrow})
	if len(got) != 1 {
		t.Fatalf("expected one repaired slot, got %d", len(got))
	}
	if got[0] != summary {
		t.Fatalf("semantically equal repair rebuilt union: got %v, want original %v", got[0], summary)
	}
}

func TestRepairSummaryWithNarrowFoldsSelfEmbeddingBeforeDescent(t *testing.T) {
	summary := typ.NewRecord().
		Field("name", typ.String).
		Build()
	narrow := typ.NewRecord().
		Field("name", typ.String).
		Field("parent", summary).
		Build()

	got := repairSummaryWithNarrow([]typ.Type{summary}, []typ.Type{narrow})
	if len(got) != 1 {
		t.Fatalf("expected one repaired slot, got %d", len(got))
	}
	if _, ok := got[0].(*typ.Recursive); !ok {
		t.Fatalf("repair result = %T, want recursive upper bound", got[0])
	}
}

func TestRepairSummaryWithNarrowFoldsMapUnionSelfEmbeddingBeforeDescent(t *testing.T) {
	summary := typ.NewMap(typ.String, typ.Any)
	narrow := typ.NewMap(typ.String, typ.NewUnion(typ.String, summary))

	got := repairSummaryWithNarrow([]typ.Type{summary}, []typ.Type{narrow})
	if len(got) != 1 {
		t.Fatalf("expected one repaired slot, got %d", len(got))
	}
	if _, ok := got[0].(*typ.Recursive); !ok {
		t.Fatalf("repair result = %T, want recursive upper bound", got[0])
	}
}

func nestedFieldType(depth int, leaf typ.Type) typ.Type {
	out := leaf
	for i := 0; i < depth; i++ {
		out = typ.NewRecord().Field("next", out).Build()
	}
	return out
}

func nestedFieldLeaf(t *testing.T, ty typ.Type, depth int) typ.Type {
	t.Helper()
	out := ty
	for i := 0; i < depth; i++ {
		rec, ok := out.(*typ.Record)
		if !ok {
			t.Fatalf("depth %d: got %T, want record", i, out)
		}
		field := rec.GetField("next")
		if field == nil {
			t.Fatalf("depth %d: missing next field in %v", i, rec)
		}
		out = field.Type
	}
	return out
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

func TestMergeReturnsForSameSignature_GenericFunctions(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("T", nil).
		Returns(typ.Integer).
		Build()

	mergedType, ok := MergeReturnsForSameSignature(prev, next)
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

func TestMergeReturnsForSameSignature_GenericTypeParamsMustMatch(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("U", nil).
		Returns(typ.Integer).
		Build()

	_, ok := MergeReturnsForSameSignature(prev, next)
	if ok {
		t.Fatal("expected mismatched generic params not to merge")
	}
}

func TestMergeReturnsForSameSignature_NormalizesLeakedTypeParams(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewTypeParam("T", nil)).
		Build()
	next := typ.Func().
		Returns(typ.Integer).
		Build()

	mergedType, ok := MergeReturnsForSameSignature(prev, next)
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

func TestNormalize_CanonicalizesStoredFunctionFact(t *testing.T) {
	fn := typ.Func().Returns(typ.Number).Build()
	got := Normalize(functionFactTest(
		factPreflowReturns(nil),
		factPostflowReturns(typ.Number),
		factSignature(fn),
	))

	if !returnsummary.Equal(factPreflowTypesTest(got), []typ.Type{typ.Nil}) {
		t.Fatalf("summary mismatch: got %v", got.Returns.Preflow)
	}
	if !returnsummary.Equal(factPostflowTypesTest(got), []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got.Returns.Postflow)
	}
	if !typ.TypeEquals(got.Public.Signature, fn) {
		t.Fatalf("signature mismatch: got %v", got.Public.Signature)
	}
}

func TestMergeType_JoinsTupleAndArrayParamsAsSequence(t *testing.T) {
	node := typ.NewRecord().Field("node_id", typ.String).Build()
	left := typ.Func().Param("nodes", typ.NewTuple(node)).Build()
	right := typ.Func().Param("nodes", typ.NewArray(node)).Build()

	merged, ok := MergeType(left, right).(*typ.Function)
	if !ok || len(merged.Params) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	want := typ.NewArray(node)
	if !typ.TypeEquals(merged.Params[0].Type, want) {
		t.Fatalf("expected sequence parameter %v, got %v", want, merged.Params[0].Type)
	}
}

func TestMergeType_RecordParamFieldsUseFunctionFactJoin(t *testing.T) {
	leftNode := typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Unknown).
		Build()
	rightNode := typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Any).
		Build()
	left := typ.Func().Param("node", leftNode).Build()
	right := typ.Func().Param("node", rightNode).Build()

	merged, ok := MergeType(left, right).(*typ.Function)
	if !ok || len(merged.Params) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	want := typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Any).
		Build()
	if !typ.TypeEquals(merged.Params[0].Type, want) {
		t.Fatalf("expected parameter-domain record join %v, got %v", want, merged.Params[0].Type)
	}
}

func TestMergeType_DisjointPartialRecordParamsBecomeOptionalFields(t *testing.T) {
	leftUsage := typ.NewRecord().Field("promptTokenCount", typ.Integer).Build()
	rightUsage := typ.NewRecord().Field("candidatesTokenCount", typ.Integer).Build()
	left := typ.Func().Param("usage", leftUsage).Build()
	right := typ.Func().Param("usage", rightUsage).Build()

	merged, ok := MergeType(left, right).(*typ.Function)
	if !ok || len(merged.Params) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	want := typ.NewRecord().
		OptField("candidatesTokenCount", typ.Integer).
		OptField("promptTokenCount", typ.Integer).
		Build()
	if !typ.TypeEquals(merged.Params[0].Type, want) {
		t.Fatalf("expected optional-field parameter shape %v, got %v", want, merged.Params[0].Type)
	}
}

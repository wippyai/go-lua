package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFactsDomain_LaneOperatorsAreIdempotent(t *testing.T) {
	fnSym := cfg.SymbolID(1)
	capturedSym := cfg.SymbolID(2)
	classSym := cfg.SymbolID(3)
	callback := typ.Func().Param("self", typ.Unknown).Returns(typ.Boolean).Build()
	fn := typ.Func().Param("name", typ.String).Returns(typ.String).Build()
	functionFacts := api.FunctionFacts{
		fnSym: {
			Call:    api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.String})},
			Returns: api.FunctionReturnProjection{Preflow: product.LiftVector([]typ.Type{typ.String}), Postflow: product.LiftVector([]typ.Type{typ.String})},
			Public:  api.FunctionPublicProjection{Signature: fn},
		},
	}
	capturedTypes := api.CapturedTypes{
		capturedSym: product.FromType(typ.NewRecord().Field("name", typ.String).Build()),
	}
	capturedFields := api.CapturedFieldAssigns{
		fnSym: {
			capturedSym: {
				fieldKey("callback"): product.FromType(typ.NewOptional(callback)),
			},
		},
	}
	constructorFields := api.ConstructorFields{
		classSym: {
			fieldKey("name"): product.FromType(typ.String),
		},
	}

	normalizedFunctions := WidenFunctionFacts(nil, functionFacts)
	if !FunctionFactsEqual(normalizedFunctions, WidenFunctionFacts(normalizedFunctions, normalizedFunctions)) {
		t.Fatalf("Widen must be idempotent for function facts")
	}
	if !FunctionFactsEqual(normalizedFunctions, JoinFunctionFacts(normalizedFunctions, normalizedFunctions)) {
		t.Fatalf("Join must be idempotent for function facts")
	}
	normalizedCapturedTypes := WidenCapturedTypes(nil, capturedTypes)
	if !symbolTypeMapEqual(normalizedCapturedTypes, WidenCapturedTypes(normalizedCapturedTypes, normalizedCapturedTypes)) {
		t.Fatalf("Widen must be idempotent for captured types")
	}
	if !symbolTypeMapEqual(normalizedCapturedTypes, JoinCapturedTypes(normalizedCapturedTypes, normalizedCapturedTypes)) {
		t.Fatalf("Join must be idempotent for captured types")
	}
	normalizedCapturedFields := WidenCapturedFieldAssigns(nil, capturedFields)
	if !CapturedFieldAssignsEqual(normalizedCapturedFields, WidenCapturedFieldAssigns(normalizedCapturedFields, normalizedCapturedFields)) {
		t.Fatalf("Widen must be idempotent for captured fields")
	}
	if !CapturedFieldAssignsEqual(normalizedCapturedFields, JoinCapturedFieldAssigns(normalizedCapturedFields, normalizedCapturedFields)) {
		t.Fatalf("Join must be idempotent for captured fields")
	}
	normalizedConstructors := WidenConstructorFields(nil, constructorFields)
	if !ConstructorFieldsEqual(normalizedConstructors, WidenConstructorFields(normalizedConstructors, normalizedConstructors)) {
		t.Fatalf("Widen must be idempotent for constructor fields")
	}
	if !ConstructorFieldsEqual(normalizedConstructors, JoinConstructorFields(normalizedConstructors, normalizedConstructors)) {
		t.Fatalf("Join must be idempotent for constructor fields")
	}

	view := functionfact.FactsProjection(normalizedFunctions)
	if got := view.ReturnSummary(fnSym); !returnsummary.Equal(got, []typ.Type{typ.String}) {
		t.Fatalf("summary must come from FunctionFacts projection, got %v", got)
	}
	if got := view.NarrowSummary(fnSym); !returnsummary.Equal(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow summary must come from FunctionFacts projection, got %v", got)
	}
	if got := view.Type(fnSym, functionfact.ProjectionSibling, api.SynthModeDeclared); got == nil {
		t.Fatalf("function type must come from FunctionFacts projection")
	}
}

func TestFactsDomain_WidenFunctionParamsIsVarianceAware(t *testing.T) {
	sym := cfg.SymbolID(1)
	prev := api.FunctionFacts{
		sym: {Public: api.FunctionPublicProjection{Signature: typ.Func().Param("path", typ.Any).Returns(typ.NewArray(typ.Unknown)).Build()}},
	}
	next := api.FunctionFacts{
		sym: {Public: api.FunctionPublicProjection{Signature: typ.Func().Param("path", typ.String).Returns(typ.NewArray(typ.Unknown)).Build()}},
	}

	widened := WidenFunctionFacts(prev, next)
	widenedAgain := WidenFunctionFacts(widened, next)
	if !FunctionFactsEqual(widened, widenedAgain) {
		t.Fatalf("Widen must be idempotent for function param facts")
	}

	fn := unwrapFunctionForDomainTest(t, functionfact.FactsProjection(widened).Type(sym, functionfact.ProjectionSibling, api.SynthModeDeclared))
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.Any) {
		t.Fatalf("expected widening to preserve broad parameter upper bound, got %v", fn)
	}
}

func TestFactsDomain_PreservesArityAndNilabilityAsSeparateParamAxes(t *testing.T) {
	sym := cfg.SymbolID(1)
	context := typ.NewRecord().
		MapComponent(typ.String, typ.Any).
		SetOpen(true).
		Build()
	raw := api.FunctionFacts{
		sym: {Public: api.FunctionPublicProjection{Signature: typ.Func().OptParam("context", typ.NewOptional(context)).Build()}},
	}

	widened := WidenFunctionFacts(nil, raw)
	fn := unwrapFunctionForDomainTest(t, functionfact.FactsProjection(widened).Type(sym, functionfact.ProjectionSibling, api.SynthModeDeclared))
	if len(fn.Params) != 1 || !fn.Params[0].Optional {
		t.Fatalf("expected optional parameter slot, got %v", fn)
	}
	want := typ.NewOptional(context)
	if !typ.TypeEquals(fn.Params[0].Type, want) {
		t.Fatalf("expected explicit nilability to remain in the value type, got %v", fn.Params[0].Type)
	}
	if !FunctionFactsEqual(widened, WidenFunctionFacts(widened, raw)) {
		t.Fatalf("expected optional parameter product-domain representation to be idempotent")
	}
}

func TestFactsDomain_WidenPreservesCapturedCallbackUnionMembers(t *testing.T) {
	sym := cfg.SymbolID(9)
	withPending := typ.NewUnion(
		typ.LiteralString("pass"),
		typ.LiteralString("pending"),
		typ.LiteralString("fail"),
		typ.LiteralString("skip"),
	)
	withoutPending := typ.NewUnion(
		typ.LiteralString("pass"),
		typ.LiteralString("fail"),
		typ.LiteralString("skip"),
	)
	prevFn := typ.Func().
		Param("suite", typ.Any).
		Param("test_case", typ.Any).
		Returns(typ.NewRecord().Field("status", withPending).Field("suite", typ.Unknown).Build()).
		Build()
	nextFn := typ.Func().
		Param("suite", typ.Any).
		Param("test_case", typ.Any).
		Returns(typ.NewRecord().Field("status", withoutPending).Field("suite", typ.Unknown).Build()).
		Build()

	widened := WidenCapturedTypes(
		api.CapturedTypes{sym: product.FromType(prevFn)},
		api.CapturedTypes{sym: product.FromType(nextFn)},
	)
	widenedAgain := WidenCapturedTypes(widened, api.CapturedTypes{sym: product.FromType(nextFn)})
	if !symbolTypeMapEqual(widened, widenedAgain) {
		t.Fatalf("Widen must be idempotent for captured callback union members")
	}

	fn := unwrapFunctionForDomainTest(t, widened[sym].ProjectValue())
	rec, ok := fn.Returns[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected callback record return, got %T", fn.Returns[0])
	}
	status := rec.GetField("status")
	if status == nil || !typ.TypeEquals(status.Type, withPending) {
		t.Fatalf("expected status union to preserve pending member, got %v", status)
	}
}

func unwrapFunctionForDomainTest(t *testing.T, got typ.Type) *typ.Function {
	t.Helper()
	fn, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("expected function type, got %T %v", got, got)
	}
	return fn
}

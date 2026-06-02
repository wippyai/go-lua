package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFactsDomain_ProductOperatorsAreIdempotentAcrossAllDomains(t *testing.T) {
	fnSym := cfg.SymbolID(1)
	capturedSym := cfg.SymbolID(2)
	classSym := cfg.SymbolID(3)
	lit := &ast.FunctionExpr{}
	callback := typ.Func().Param("self", typ.Unknown).Returns(typ.Boolean).Build()
	fn := typ.Func().Param("name", typ.String).Returns(typ.String).Build()
	raw := api.Facts{
		FunctionFacts: api.FunctionFacts{
			fnSym: {Params: product.LiftVector([]typ.Type{typ.String}), Summary: product.LiftVector([]typ.Type{typ.String}), Narrow: product.LiftVector([]typ.Type{typ.String}), Signature: fn},
		},
		LiteralSigs: api.LiteralSigs{
			lit: typ.Func().Param("name", typ.String).Returns(typ.String).Build(),
		},
		CapturedTypes: api.CapturedTypes{
			capturedSym: product.FromType(typ.NewRecord().Field("name", typ.String).Build()),
		},
		CapturedFields: api.CapturedFieldAssigns{
			fnSym: {
				capturedSym: {
					fieldKey("callback"): product.FromType(typ.NewOptional(callback)),
				},
			},
		},
		ConstructorFields: api.ConstructorFields{
			classSym: {
				fieldKey("name"): product.FromType(typ.String),
			},
		},
	}

	normalized := WidenFacts(api.Facts{}, raw)
	if !FactsEqual(normalized, WidenFacts(normalized, normalized)) {
		t.Fatalf("Widen must be idempotent across the product domain")
	}
	if !FactsEqual(normalized, JoinFacts(normalized, normalized)) {
		t.Fatalf("Join must be idempotent across the product domain")
	}

	if got := functionfact.ReturnSummary(normalized.FunctionFacts, fnSym); !returnsummary.Equal(got, []typ.Type{typ.String}) {
		t.Fatalf("summary must come from canonical FunctionFacts, got %v", got)
	}
	if got := functionfact.NarrowSummary(normalized.FunctionFacts, fnSym); !returnsummary.Equal(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow summary must come from canonical FunctionFacts, got %v", got)
	}
	if got := functionfact.SiblingTypeProjection(normalized.FunctionFacts, fnSym, api.PhaseScopeCompute); got == nil {
		t.Fatalf("function type must come from canonical FunctionFacts")
	}
}

func TestFactsDomain_WidenIdempotentForLiteralUnknownVsConcreteReturn(t *testing.T) {
	lit := &ast.FunctionExpr{}
	prev := api.Facts{
		LiteralSigs: api.LiteralSigs{
			lit: typ.Func().Param("name", typ.Unknown).Returns(typ.Unknown, typ.NewOptional(typ.String)).Build(),
		},
	}
	next := api.Facts{
		LiteralSigs: api.LiteralSigs{
			lit: typ.Func().Param("name", typ.Unknown).Returns(
				typ.NewOptional(typ.NewRecord().
					Field("id", typ.String).
					Field("priority", typ.Integer).
					SetOpen(true).
					Build()),
				typ.NewOptional(typ.String),
			).Build(),
		},
	}

	widened := WidenFacts(prev, next)
	widenedAgain := WidenFacts(widened, next)
	if !FactsEqual(widened, widenedAgain) {
		t.Fatalf("Widen must be idempotent for literal signatures:\nfirst=%#v\nsecond=%#v", widened, widenedAgain)
	}

	got := widened.LiteralSigs[lit]
	if got == nil || len(got.Returns) != 2 || !typ.TypeEquals(got.Returns[0], typ.Unknown) {
		t.Fatalf("expected unresolved literal return to remain the stable upper bound, got %v", got)
	}
}

func TestFactsDomain_WidenFunctionParamsIsVarianceAware(t *testing.T) {
	sym := cfg.SymbolID(1)
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Signature: typ.Func().Param("path", typ.Any).Returns(typ.NewArray(typ.Unknown)).Build()},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Signature: typ.Func().Param("path", typ.String).Returns(typ.NewArray(typ.Unknown)).Build()},
		},
	}

	widened := WidenFacts(prev, next)
	widenedAgain := WidenFacts(widened, next)
	if !FactsEqual(widened, widenedAgain) {
		t.Fatalf("Widen must be idempotent for function param facts")
	}

	fn := unwrapFunctionForDomainTest(t, functionfact.SiblingTypeProjection(widened.FunctionFacts, sym, api.PhaseScopeCompute))
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
	raw := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Signature: typ.Func().OptParam("context", typ.NewOptional(context)).Build()},
		},
	}

	widened := WidenFacts(api.Facts{}, raw)
	fn := unwrapFunctionForDomainTest(t, functionfact.SiblingTypeProjection(widened.FunctionFacts, sym, api.PhaseScopeCompute))
	if len(fn.Params) != 1 || !fn.Params[0].Optional {
		t.Fatalf("expected optional parameter slot, got %v", fn)
	}
	want := typ.NewOptional(context)
	if !typ.TypeEquals(fn.Params[0].Type, want) {
		t.Fatalf("expected explicit nilability to remain in the value type, got %v", fn.Params[0].Type)
	}
	if !FactsEqual(widened, WidenFacts(widened, raw)) {
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

	widened := WidenFacts(
		api.Facts{CapturedTypes: api.CapturedTypes{sym: product.FromType(prevFn)}},
		api.Facts{CapturedTypes: api.CapturedTypes{sym: product.FromType(nextFn)}},
	)
	widenedAgain := WidenFacts(widened, api.Facts{CapturedTypes: api.CapturedTypes{sym: product.FromType(nextFn)}})
	if !FactsEqual(widened, widenedAgain) {
		t.Fatalf("Widen must be idempotent for captured callback union members")
	}

	fn := unwrapFunctionForDomainTest(t, widened.CapturedTypes[sym].ProjectValue())
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

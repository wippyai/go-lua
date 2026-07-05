package sourceprovenance

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestTopLevelProducerLooksThroughAssertionWrappers(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "foo"}}
	wrapped := &ast.NonNilAssertExpr{
		Expr: &ast.CastExpr{
			Expr: call,
			Type: &ast.PrimitiveTypeExpr{Name: "number"},
		},
	}

	got, ok := Call(wrapped)
	if !ok || got != call {
		t.Fatalf("Call(wrapped) = %p/%v, want inner call %p/true", got, ok, call)
	}
	if !CanProduceMultipleValues(wrapped) {
		t.Fatal("wrapped call did not produce multiple values")
	}
}

func TestAdjustRetUsesInnerProducer(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "foo"}, AdjustRet: true}
	wrappedCall := &ast.CastExpr{Expr: call, Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	if !AdjustRet(wrappedCall) {
		t.Fatal("wrapped adjusted call did not report AdjustRet")
	}

	vararg := &ast.Comma3Expr{AdjustRet: true}
	wrappedVararg := &ast.NonNilAssertExpr{Expr: vararg}
	if !AdjustRet(wrappedVararg) {
		t.Fatal("wrapped adjusted vararg did not report AdjustRet")
	}
}

func TestAssertionInnerStopsAtNonAssertion(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	wrapped := &ast.CastExpr{Expr: expr, Type: &ast.PrimitiveTypeExpr{Name: "any"}}

	if got := AssertionInner(wrapped); got != expr {
		t.Fatalf("AssertionInner = %T %p, want ident %p", got, got, expr)
	}
}

func TestProofInnerStopsAtAnyCast(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	wrapped := &ast.NonNilAssertExpr{
		Expr: &ast.CastExpr{
			Expr: expr,
			Type: &ast.PrimitiveTypeExpr{Name: "any"},
		},
	}

	if got, ok := ProofInner(wrapped); ok || got != wrapped.Expr {
		t.Fatalf("ProofInner(any cast) = %T/%v, want cast/false", got, ok)
	}

	typed := &ast.CastExpr{Expr: expr, Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	if got, ok := ProofInner(typed); !ok || got != expr {
		t.Fatalf("ProofInner(number cast) = %T/%v, want ident/true", got, ok)
	}
}

func TestProofInnerStopsAtUnknownCast(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	wrapped := &ast.CastExpr{
		Expr: expr,
		Type: &ast.PrimitiveTypeExpr{Name: "unknown"},
	}

	if got, ok := ProofInner(wrapped); ok || got != wrapped {
		t.Fatalf("ProofInner(unknown cast) = %T/%v, want cast/false", got, ok)
	}
}

func TestProofInnerIsFunctionLooksThroughProofWrappers(t *testing.T) {
	fn := &ast.FunctionExpr{}
	wrapped := &ast.NonNilAssertExpr{
		Expr: &ast.CastExpr{
			Expr: fn,
			Type: &ast.PrimitiveTypeExpr{Name: "fun"},
		},
	}
	if !ProofInnerIsFunction(wrapped) {
		t.Fatal("ProofInnerIsFunction did not look through proof-transparent wrappers")
	}
}

func TestProofInnerIsFunctionStopsAtProofBoundary(t *testing.T) {
	fn := &ast.FunctionExpr{}
	wrapped := &ast.CastExpr{
		Expr: fn,
		Type: &ast.PrimitiveTypeExpr{Name: "any"},
	}
	if ProofInnerIsFunction(wrapped) {
		t.Fatal("ProofInnerIsFunction crossed an any proof boundary")
	}
}

func TestProofIdentUsesProofTransparentWrappers(t *testing.T) {
	ident := &ast.IdentExpr{Value: "x"}
	wrapped := &ast.NonNilAssertExpr{
		Expr: &ast.CastExpr{
			Expr: ident,
			Type: &ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	if got, ok := ProofIdent(wrapped); !ok || got != ident {
		t.Fatalf("ProofIdent(transparent wrappers) = %T/%v, want ident", got, ok)
	}
	boundary := &ast.CastExpr{
		Expr: ident,
		Type: &ast.PrimitiveTypeExpr{Name: "any"},
	}
	if got, ok := ProofIdent(boundary); ok || got != nil {
		t.Fatalf("ProofIdent(any boundary) = %T/%v, want nil/false", got, ok)
	}
}

func TestConcreteRuntimeCastSourceClassifiesValidationCasts(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	source := ASTSource{
		Kind: SourceExpression,
		Expr: &ast.NonNilAssertExpr{
			Expr: &ast.CastExpr{
				Expr:   expr,
				Type:   &ast.PrimitiveTypeExpr{Name: "number"},
				Syntax: ast.CastSyntaxColonColon,
			},
		},
	}
	if !ConcreteRuntimeCastSource(source) {
		t.Fatal("ConcreteRuntimeCastSource rejected concrete runtime cast")
	}

	source.Expr = &ast.CastExpr{
		Expr:   expr,
		Type:   &ast.PrimitiveTypeExpr{Name: "any"},
		Syntax: ast.CastSyntaxColonColon,
	}
	if ConcreteRuntimeCastSource(source) {
		t.Fatal("ConcreteRuntimeCastSource accepted top-like any cast")
	}

	source.Kind = SourceCall
	if ConcreteRuntimeCastSource(source) {
		t.Fatal("ConcreteRuntimeCastSource accepted non-expression source")
	}
}

func TestTypedNilProducersAreAbsent(t *testing.T) {
	var call *ast.FuncCallExpr
	var callExpr ast.Expr = call
	if got := TopLevelProducer(callExpr); got.Kind != ProducerNone || got.Expr != nil || got.Call != nil {
		t.Fatalf("typed nil call producer = %#v, want none", got)
	}
	if CanProduceMultipleValues(callExpr) {
		t.Fatal("typed nil call should not produce multiple values")
	}
	if AdjustRet(callExpr) {
		t.Fatal("typed nil call should not report adjusted returns")
	}

	var cast *ast.CastExpr
	var castExpr ast.Expr = cast
	if got := AssertionInner(castExpr); got != nil {
		t.Fatalf("AssertionInner(typed nil cast) = %#v, want nil", got)
	}
	if got, ok := ProofInner(castExpr); !ok || got != nil {
		t.Fatalf("ProofInner(typed nil cast) = %#v/%v, want nil/true", got, ok)
	}
}

func TestBrokenAssertionWrappersHaveNoProducer(t *testing.T) {
	wrapped := &ast.CastExpr{
		Expr: &ast.NonNilAssertExpr{},
		Type: &ast.PrimitiveTypeExpr{Name: "number"},
	}
	if got := AssertionInner(wrapped); got != nil {
		t.Fatalf("AssertionInner(broken wrapper) = %#v, want nil", got)
	}
	if !missingAssertionInner(wrapped) {
		t.Fatal("broken wrapper did not report missing assertion inner")
	}
	if got := TopLevelProducer(wrapped); got.Kind != ProducerNone || got.Expr != nil {
		t.Fatalf("broken wrapper producer = %#v, want none", got)
	}
	if CanProduceMultipleValues(wrapped) {
		t.Fatal("broken wrapper should not produce multiple values")
	}
	if AdjustRet(wrapped) {
		t.Fatal("broken wrapper should not report adjusted returns")
	}
}

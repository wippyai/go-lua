package callarg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestForArgsAdaptsIndexedExpression(t *testing.T) {
	args := []ast.Expr{&ast.NumberExpr{Value: "1"}}
	called := false
	reSynth := ForArgs(args, func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		called = true
		if idx != 0 || arg != args[0] {
			t.Fatalf("unexpected arg idx=%d expr=%T", idx, arg)
		}
		if expected != typ.String {
			t.Fatalf("got expected %v, want string", expected)
		}
		return typ.String
	})

	if got := reSynth(0, typ.String); got != typ.String {
		t.Fatalf("got %v, want string", got)
	}
	if !called {
		t.Fatal("expected adapter to call re-synthesizer")
	}
	if got := reSynth(1, typ.String); got != nil {
		t.Fatalf("out-of-range arg got %v, want nil", got)
	}
}

func TestInitialInferenceTypesUsesShallowDirectFunctionLiteral(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"value"}}}
	args := []ast.Expr{fn, &ast.StringExpr{Value: "seed"}}

	got, hasCallback := InitialInferenceTypes(
		args,
		func(arg ast.Expr) typ.Type {
			if arg == args[1] {
				return typ.String
			}
			t.Fatalf("typeOf called for callback arg %T", arg)
			return nil
		},
		nil,
	)

	if !hasCallback {
		t.Fatal("expected callback argument to be detected")
	}
	callback, ok := got[0].(*typ.Function)
	if !ok || callback == nil || len(callback.Params) != 1 || len(callback.Returns) != 1 {
		t.Fatalf("callback arg = %v, want shallow unary function", got[0])
	}
	if !typ.TypeEquals(callback.Params[0].Type, typ.Any) || !typ.TypeEquals(callback.Returns[0], typ.Any) {
		t.Fatalf("callback arg = %v, want any -> any", callback)
	}
	if got[1] != typ.String {
		t.Fatalf("non-callback arg = %v, want string", got[1])
	}
}

func TestInitialInferenceTypesUsesShallowNamedFunctionLiteral(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"value"}}}
	cb := &ast.IdentExpr{Value: "cb"}

	got, hasCallback := InitialInferenceTypes(
		[]ast.Expr{cb},
		func(arg ast.Expr) typ.Type {
			t.Fatalf("typeOf called for named callback arg %T", arg)
			return nil
		},
		func(arg ast.Expr) *ast.FunctionExpr {
			if arg == cb {
				return fn
			}
			return nil
		},
	)

	if !hasCallback {
		t.Fatal("expected named callback argument to be detected")
	}
	callback, ok := got[0].(*typ.Function)
	if !ok || callback == nil || len(callback.Params) != 1 || len(callback.Returns) != 1 {
		t.Fatalf("callback arg = %v, want shallow unary function", got[0])
	}
}

func TestFull_Function(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		return typ.String
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.FunctionExpr{}, typ.Func().Build())

	if !called {
		t.Fatal("expected callback to be called")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFull_Table(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		return typ.String
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.TableExpr{}, typ.NewRecord().Build())

	if !called {
		t.Fatal("expected callback to be called")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFull_Other(t *testing.T) {
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		return typ.String
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.NumberExpr{}, typ.Integer)

	if result != nil {
		t.Fatal("expected nil for expression that does not benefit from contextual re-synthesis")
	}
}

func TestFull_Cast(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		if _, ok := arg.(*ast.CastExpr); !ok {
			t.Fatalf("got %T, want CastExpr", arg)
		}
		return typ.String
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.CastExpr{}, typ.String)

	if !called {
		t.Fatal("expected callback to be called for cast expression")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFull_Logical(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		if _, ok := arg.(*ast.LogicalOpExpr); !ok {
			t.Fatalf("got %T, want LogicalOpExpr", arg)
		}
		return typ.String
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.LogicalOpExpr{}, typ.String)

	if !called {
		t.Fatal("expected callback to be called for logical expression")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFull_Identifier(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		return typ.String
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.IdentExpr{Value: "cb"}, typ.Func().Build())

	if !called {
		t.Fatal("expected callback to be called for identifier")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFull_IdentifierDoesNotUseExpectedAsProofForUnknown(t *testing.T) {
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		if expected != nil {
			t.Fatalf("identifier synthesis got expected %v, want nil", expected)
		}
		return typ.Unknown
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.IdentExpr{Value: "resource_id"}, typ.String)

	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

func TestFullWithExpectedProofs_IdentifierAsksExpectedAwareObserver(t *testing.T) {
	expectedType := typ.String
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		if expected != expectedType {
			t.Fatalf("identifier synthesis expected = %v, want %v", expected, expectedType)
		}
		return expectedType
	}

	reSynth := FullWithExpectedProofs(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.IdentExpr{Value: "self"}, expectedType)

	if result != expectedType {
		t.Fatalf("got %v, want %v", result, expectedType)
	}
}

func TestFull_IdentifierDoesNotUseRecursiveExpectedAsProof(t *testing.T) {
	inferred := typ.Func().Param("value", typ.String).Build()
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", self).Build()
	})
	expected := typ.NewRecord().
		Field("node", node).
		Field("handler", inferred).
		Build()
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		if expected != nil {
			t.Fatalf("identifier synthesis got expected %v, want nil", expected)
		}
		return inferred
	}

	reSynth := Full(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.IdentExpr{Value: "handler"}, expected)

	if result != inferred {
		t.Fatalf("got %v, want inferred recursive-heavy value left unchanged", result)
	}
}

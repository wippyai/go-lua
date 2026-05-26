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

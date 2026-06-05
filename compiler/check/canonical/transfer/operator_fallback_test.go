package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEvalBinaryStructuralFallbackUsesCoreOperatorLaw(t *testing.T) {
	const sym = cfg.SymbolID(9101)
	ident := &ast.IdentExpr{Value: "num"}
	in := input.BuildFromFunction(&ast.FunctionExpr{}, nil, nil)
	if in.Graph == nil {
		t.Fatal("test graph did not build")
	}
	in.Graph.Bindings().Bind(ident, sym)
	in.Graph.Bindings().SetName(sym, "num")

	tr := New(in, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Integer),
		},
	}

	got, ok := tr.evalBinary(&out, "+", ident, &ast.NumberExpr{Value: "1"}, nil)
	if !ok {
		t.Fatal("evalBinary returned no value")
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.Integer) {
		t.Fatalf("evalBinary(integer + integer literal) = %v, want integer", got.ProjectValue())
	}
}

func TestEvalUnaryStructuralFallbackUsesCoreOperatorLaw(t *testing.T) {
	const sym = cfg.SymbolID(9102)
	ident := &ast.IdentExpr{Value: "num"}
	in := input.BuildFromFunction(&ast.FunctionExpr{}, nil, nil)
	if in.Graph == nil {
		t.Fatal("test graph did not build")
	}
	in.Graph.Bindings().Bind(ident, sym)
	in.Graph.Bindings().SetName(sym, "num")

	tr := New(in, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Integer),
		},
	}

	got, ok := tr.evalUnary(&out, "-", ident, nil)
	if !ok {
		t.Fatal("evalUnary returned no value")
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.Integer) {
		t.Fatalf("evalUnary(-integer) = %v, want integer", got.ProjectValue())
	}
}

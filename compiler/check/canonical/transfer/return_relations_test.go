package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

func TestCallReturnRelationsUsesProductArgEvidence(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"arg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	arg := &ast.IdentExpr{Value: "arg"}
	in.Graph.Bindings().Bind(arg, in.Scope.ParamSymbols[0])
	in.Graph.Bindings().SetName(in.Scope.ParamSymbols[0], "arg")

	rel := flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}
	typer := &productReturnRelationsTestTyper{
		rels: flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{rel}),
	}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	got := tr.callReturnRelations(&out, &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "callee"},
		Args: []ast.Expr{arg},
	}, nil)

	if !got.HasErrorReturn(rel) {
		t.Fatalf("return relations = %#v, want %#v", got.ErrorReturns(), rel)
	}
	if len(typer.args) != 1 || typer.args[0].IsZero() || !typer.args[0].IsGradualTop() {
		t.Fatalf("product return-relation args = %#v, want one gradual-top product value", typer.args)
	}
}

type productReturnRelationsTestTyper struct {
	captureEffectTyper
	rels flow.ReturnRelations
	args []product.AbstractValue
}

func (t *productReturnRelationsTestTyper) ReturnRelationsFromValues(
	_ *ast.FuncCallExpr,
	ctx ProductCallContext,
) flow.ReturnRelations {
	t.args = append([]product.AbstractValue(nil), ctx.ArgValues...)
	return t.rels
}

var _ productReturnRelationProvider = (*productReturnRelationsTestTyper)(nil)

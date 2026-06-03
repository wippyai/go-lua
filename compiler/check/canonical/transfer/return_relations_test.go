package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
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

func TestAssignCallPostconditionsMaterializeLengthParamLowerBound(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"arg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	argSym := in.Scope.ParamSymbols[0]
	targetSym := cfg.SymbolID(7001)
	arg := &ast.IdentExpr{Value: "arg"}
	in.Graph.Bindings().Bind(arg, argSym)
	in.Graph.Bindings().SetName(argSym, "arg")

	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "keys"},
		Args: []ast.Expr{arg},
	}
	callInfo := &cfg.CallInfo{Call: call, Callee: call.Func, Args: call.Args}
	typer := &productReturnRelationsTestTyper{
		rels: flow.ReturnRelationsOfLengthParams([]flow.ReturnLengthParamRelation{{
			ReturnIndex: 0,
			ParamIndex:  0,
		}}),
	}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top(),
	}
	out.Num.ApplyLenGeConst(flow.SymbolPathKey(argSym, nil), 2)
	info := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "ks",
			Symbol: targetSym,
		}},
		Sources:     []ast.Expr{call},
		SourceCalls: []*cfg.CallInfo{callInfo},
	}

	effects := tr.buildAssignCallPostconditions(&out, info, nil)
	tr.applyAssignCallPostconditions(&out, effects)

	targetKey := flow.SymbolPathKey(targetSym, nil)
	if lower, _, ok := out.Num.LenBoundsFor(targetKey); !ok || lower != 2 {
		t.Fatalf("target length lower = %d/%v, want 2", lower, ok)
	}
	if !out.Rel.HasTargetLengthParam(targetSym, targetKey, 0) {
		t.Fatalf("point relations = %#v, want wrapper-preserving target length >= param 0", out.Rel)
	}
}

func TestAssignCallPostconditionsUsePreWriteArgumentLength(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"arg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	argSym := in.Scope.ParamSymbols[0]
	arg := &ast.IdentExpr{Value: "arg"}
	in.Graph.Bindings().Bind(arg, argSym)
	in.Graph.Bindings().SetName(argSym, "arg")

	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "keys"},
		Args: []ast.Expr{arg},
	}
	callInfo := &cfg.CallInfo{Call: call, Callee: call.Func, Args: call.Args}
	typer := &postconditionAssignTestTyper{
		rels: flow.ReturnRelationsOfLengthParams([]flow.ReturnLengthParamRelation{{
			ReturnIndex: 0,
			ParamIndex:  0,
		}}),
		returns: []product.AbstractValue{product.FromType(typ.NewArray(typ.String))},
	}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(argSym): product.FromType(typ.NewArray(typ.String)),
		},
		Num: numeric.NewState(),
		Rel: flow.PointRelationsDomain.Top(),
	}
	out.Num.ApplyLenGeConst(flow.SymbolPathKey(argSym, nil), 3)
	info := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "arg",
			Symbol: argSym,
		}},
		Sources:     []ast.Expr{call},
		SourceCalls: []*cfg.CallInfo{callInfo},
	}

	tr.applyAssign(&out, 0, info, nil)

	targetKey := flow.SymbolPathKey(argSym, nil)
	if lower, _, ok := out.Num.LenBoundsFor(targetKey); !ok || lower != 3 {
		t.Fatalf("rewritten target length lower = %d/%v, want pre-call lower 3", lower, ok)
	}
	if !out.Rel.HasTargetLengthParam(argSym, targetKey, 0) {
		t.Fatalf("point relations = %#v, want rewritten target length >= original param 0", out.Rel)
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

type postconditionAssignTestTyper struct {
	captureEffectTyper
	rels    flow.ReturnRelations
	returns []product.AbstractValue
}

func (t *postconditionAssignTestTyper) ReturnRelationsFromValues(
	_ *ast.FuncCallExpr,
	_ ProductCallContext,
) flow.ReturnRelations {
	return t.rels
}

func (t *postconditionAssignTestTyper) CallReturnValues(
	_ *ast.FuncCallExpr,
	_ ProductCallContext,
) ([]product.AbstractValue, bool) {
	return t.returns, true
}

var _ productReturnRelationProvider = (*postconditionAssignTestTyper)(nil)
var _ ProductCallTyper = (*postconditionAssignTestTyper)(nil)

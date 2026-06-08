package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/constraint"
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

	effects := tr.buildAssignCallPostconditions(&out, 0, info, nil)
	tr.applyAssignCallPostconditions(&out, effects)

	targetLocal, ok := flow.LocalAddressOfPath(constraint.NewPath(targetSym, "ks"))
	if !ok {
		t.Fatal("local target address was not produced")
	}
	targetKey := flow.SymbolPathKey(targetSym, nil)
	if lower, _, ok := out.Num.LenBoundsFor(targetKey); !ok || lower != 2 {
		t.Fatalf("target length lower = %d/%v, want 2", lower, ok)
	}
	if !out.Rel.HasTargetLengthParamLocal(targetLocal, 0) {
		t.Fatalf("point relations = %#v, want wrapper-preserving target length >= param 0", out.Rel)
	}
}

func TestAssignCallPostconditionsMaterializeReturnKeyBoundaryFact(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"self"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	selfSym := in.Scope.ParamSymbols[0]
	idSym := cfg.SymbolID(7002)
	self := &ast.IdentExpr{Value: "self"}
	in.Graph.Bindings().Bind(self, selfSym)
	in.Graph.Bindings().SetName(selfSym, "self")

	call := &ast.FuncCallExpr{
		Receiver: self,
		Method:   "create_node",
	}
	callInfo := &cfg.CallInfo{Call: call, Method: call.Method, Receiver: self}
	boundaryFacts := flow.BoundaryFactsOf([]flow.BoundaryKeyPresenceFact{{
		Table: flow.BoundaryPath{
			Kind:     flow.BoundaryPathParam,
			Index:    0,
			Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}},
		},
		Key: flow.BoundaryPath{Kind: flow.BoundaryPathReturn, Index: 0},
	}}, nil, nil, nil, nil, nil)
	typer := &productReturnRelationsTestTyper{
		facts: boundaryFacts,
	}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{KeyPresence: flow.KeyPresenceFactsDomain.Top()}
	info := &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "id",
			Symbol: idSym,
		}},
		Sources:     []ast.Expr{call},
		SourceCalls: []*cfg.CallInfo{callInfo},
	}

	effects := tr.buildAssignCallPostconditions(&out, 0, info, nil)
	tr.applyAssignCallPostconditions(&out, effects)

	tablePath := constraint.NewPath(selfSym, "self").Field("nodes")
	keyPath := constraint.NewPath(idSym, "id")
	if !testKeyPresenceHas(t, out.KeyPresence, tablePath, keyPath) {
		t.Fatalf("return-key postcondition did not seed KeyPresence: %s", out.KeyPresence.Format())
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

	targetLocal, ok := flow.LocalAddressOfPath(constraint.NewPath(argSym, "arg"))
	if !ok {
		t.Fatal("local target address was not produced")
	}
	targetKey := flow.SymbolPathKey(argSym, nil)
	if lower, _, ok := out.Num.LenBoundsFor(targetKey); !ok || lower != 3 {
		t.Fatalf("rewritten target length lower = %d/%v, want pre-call lower 3", lower, ok)
	}
	if !out.Rel.HasTargetLengthParamLocal(targetLocal, 0) {
		t.Fatalf("point relations = %#v, want rewritten target length >= original param 0", out.Rel)
	}
}

type productReturnRelationsTestTyper struct {
	captureEffectTyper
	rels  flow.ReturnRelations
	facts flow.BoundaryFacts
	args  []product.AbstractValue
}

func (t *productReturnRelationsTestTyper) ProductCallFromValues(
	_ *ast.FuncCallExpr,
	ctx ProductCallContext,
) ProductCallResult {
	t.args = append([]product.AbstractValue(nil), ctx.ArgValues...)
	result := EmptyProductCallResult()
	result.ReturnRelations = t.rels
	if t.facts.HasProof() {
		result.Effects.BoundaryFacts = t.facts
	}
	return result
}

var _ ProductCallProvider = (*productReturnRelationsTestTyper)(nil)

type postconditionAssignTestTyper struct {
	captureEffectTyper
	rels    flow.ReturnRelations
	returns []product.AbstractValue
}

func (t *postconditionAssignTestTyper) ProductCallFromValues(
	_ *ast.FuncCallExpr,
	_ ProductCallContext,
) ProductCallResult {
	return ProductCallResult{
		ReturnValues:    t.returns,
		HasReturnValues: true,
		ReturnRelations: t.rels,
		Effects:         EmptyCallEffects(),
	}
}

var _ ProductCallProvider = (*postconditionAssignTestTyper)(nil)

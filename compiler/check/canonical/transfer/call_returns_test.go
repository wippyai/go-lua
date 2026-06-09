package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEvalCallPreservesProductReturnEvidence(t *testing.T) {
	typer := &productReturnTestTyper{returns: []product.AbstractValue{product.GradualAny()}}
	tr := New(input.Inputs{}, Config{CallTyper: typer})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	returns, ok := tr.evalCall(&out, &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "callee"}}, nil)
	if !ok || len(returns) != 1 {
		t.Fatalf("evalCall returned %d/%v, want one product return", len(returns), ok)
	}
	if !returns[0].IsGradualTop() {
		t.Fatalf("return value lost gradual-top evidence: %v", returns[0].ProjectValue())
	}
}

func TestProductCallContextUsesTransferExpressionEvaluator(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.NumberExpr{Value: "42"}}}

	ctx := tr.ProductCallContext(&out, call)
	if len(ctx.ArgValues) != 1 || ctx.ArgValues[0].IsZero() {
		t.Fatalf("ProductCallContext ArgValues = %v, want evaluated numeric argument", ctx.ArgValues)
	}
	if len(ctx.RuntimeArgValues) != 1 || ctx.RuntimeArgValues[0].IsZero() {
		t.Fatalf("ProductCallContext RuntimeArgValues = %v, want evaluated numeric argument", ctx.RuntimeArgValues)
	}
	if av, ok := ctx.RuntimeArgValueAt(-1); !ok || av.IsZero() {
		t.Fatalf("RuntimeArgValueAt(-1) = %v/%v, want evaluated numeric argument", av, ok)
	}
	if ctx.ExprValue == nil {
		t.Fatal("ProductCallContext missing ExprValue resolver")
	}
	if av, ok := ctx.ExprValue(call.Args[0]); !ok || av.IsZero() {
		t.Fatalf("ExprValue(arg) = %v/%v, want evaluated product value", av, ok)
	}
}

func TestTransferUnreachableCallDoesNotEmitArgumentDemand(t *testing.T) {
	page := &ast.IdentExpr{Value: "page"}
	arg := &ast.AttrGetExpr{
		Object:    page,
		Key:       &ast.StringExpr{Value: "data_func"},
		KeySyntax: ast.AttrKeyDot,
	}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "takes_string"},
		Args: []ast.Expr{arg},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"page"}},
		Stmts:   []ast.Stmt{&ast.FuncCallStmt{Expr: call}},
	}
	in := input.BuildFromFunction(fn, nil, nil, "takes_string")
	tr := New(in, Config{CallTyper: deadCallDemandTyper{demand: typ.String}})

	var callPoint cfg.Point
	in.Graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info != nil && info.Call == call {
			callPoint = p
		}
	})
	if callPoint == 0 {
		t.Fatal("test call point not found")
	}

	demands := paramevidence.Contracts{}
	out := tr.Transfer(in.Graph, callPoint, flow.PointStateDomain.Bottom(), nil, func(idx int, c paramevidence.ParamContract) {
		demands = paramevidence.JoinDemand(demands, idx, c)
	})

	if len(demands) != 0 {
		t.Fatalf("unreachable call emitted demands: %v", demands)
	}
	if !flow.PointStateDomain.Equal(out, flow.PointStateDomain.Bottom()) {
		t.Fatalf("unreachable call out = %#v, want point-state Bottom", out)
	}
}

func TestProductCallContextExprValueDoesNotEvaluateCalls(t *testing.T) {
	typer := &nonReentrantProductTyper{t: t}
	tr := New(input.Inputs{}, Config{CallTyper: typer})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "callee"}}

	returns, ok := tr.evalCall(&out, call, nil)
	if !ok || len(returns) != 1 || !typ.TypeEquals(returns[0].ProjectValue(), typ.String) {
		t.Fatalf("evalCall returns = %v/%v, want string return", returns, ok)
	}
	if typer.calls != 1 {
		t.Fatalf("CallReturnValues called %d times; provider ExprValue re-entered evalCall", typer.calls)
	}
	if !typer.sawCallProjectionMiss {
		t.Fatalf("provider ExprValue(call) resolved a call expression; want projection-only miss")
	}
}

func TestProjectExprValueDynamicAnyMethodChain(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"reader"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	readerSym := in.Scope.ParamSymbols[0]
	reader := &ast.IdentExpr{Value: "reader"}
	in.Graph.Bindings().Bind(reader, readerSym)

	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(readerSym): product.FromType(typ.Any),
	}}
	inner := &ast.FuncCallExpr{Receiver: reader, Method: "with", Args: []ast.Expr{&ast.NumberExpr{Value: "1"}}}
	outer := &ast.FuncCallExpr{Receiver: inner, Method: "all"}

	av, ok := tr.projectExprValue(&out, outer)
	if !ok || !typ.IsAny(av.ProjectValue()) {
		t.Fatalf("projectExprValue(dynamic chain) = %v/%v, want any", av.ProjectValue(), ok)
	}
}

func TestProductCallContextRuntimeArgsIncludeMethodReceiver(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"recv", "arg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	recvSym := in.Scope.ParamSymbols[0]
	argSym := in.Scope.ParamSymbols[1]
	recv := &ast.IdentExpr{Value: "recv"}
	arg := &ast.IdentExpr{Value: "arg"}
	in.Graph.Bindings().Bind(recv, recvSym)
	in.Graph.Bindings().Bind(arg, argSym)

	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(recvSym): product.FromType(typ.String),
		flow.SymbolValueKey(argSym):  product.FromType(typ.Number),
	}}
	ctx := tr.ProductCallContext(&out, &ast.FuncCallExpr{Receiver: recv, Method: "send", Args: []ast.Expr{arg}})

	if len(ctx.ArgValues) != 1 {
		t.Fatalf("ArgValues length = %d, want positional args only", len(ctx.ArgValues))
	}
	if len(ctx.RuntimeArgValues) != 2 {
		t.Fatalf("RuntimeArgValues length = %d, want receiver plus positional arg", len(ctx.RuntimeArgValues))
	}
	if av, ok := ctx.RuntimeArgValueAt(0); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("RuntimeArgValueAt(0) = %v/%v, want receiver string", av.ProjectValue(), ok)
	}
	if av, ok := ctx.RuntimeArgValueAt(1); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("RuntimeArgValueAt(1) = %v/%v, want argument number", av.ProjectValue(), ok)
	}
	if av, ok := ctx.RuntimeArgValueAt(-1); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("RuntimeArgValueAt(-1) = %v/%v, want tail argument number", av.ProjectValue(), ok)
	}
}

func TestProductCallContextMethodReceiverSelfTypeRejectsOpenSurface(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"recv"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	recvSym := in.Scope.ParamSymbols[0]
	recv := &ast.IdentExpr{Value: "recv"}
	in.Graph.Bindings().Bind(recv, recvSym)

	tp := typ.NewTypeParam("T", nil)
	openReceiver := typ.NewRecord().
		Field("go", typ.Func().Build()).
		Field("value", tp).
		Build()
	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(recvSym): product.FromType(openReceiver),
	}}
	ctx := tr.ProductCallContext(&out, &ast.FuncCallExpr{Receiver: recv, Method: "go"})

	if ctx.SelfType != nil {
		t.Fatalf("ProductCallContext SelfType = %v, want nil for open receiver", ctx.SelfType)
	}
	if av, ok := ctx.RuntimeArgValueAt(0); !ok || !typ.TypeEquals(av.ProjectValue(), openReceiver) {
		t.Fatalf("RuntimeArgValueAt(0) = %v/%v, want receiver value preserved", av.ProjectValue(), ok)
	}
}

func TestProductCallContextMethodReceiverUsesBranchNarrowedCopiedLocal(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"store"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	storeSym := in.Scope.ParamSymbols[0]
	store := &ast.IdentExpr{Value: "store"}
	in.Graph.Bindings().Bind(store, storeSym)

	svc := typ.NewRecord().
		Field("go", typ.Func().Build()).
		Build()
	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(storeSym): product.FromType(typ.NewOptional(svc)),
	}}
	narrowed := tr.narrowByCondCheck(out, &cfg.BranchInfo{
		CondSymbol: storeSym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition:  store,
	}, true, false)
	if av, ok := tr.symbolValue(&narrowed, storeSym); !ok || !typ.TypeEquals(av.ProjectValue(), svc) {
		t.Fatalf("narrowed store = %v/%v, want non-optional receiver", av.ProjectValue(), ok)
	}

	call := &ast.FuncCallExpr{Receiver: store, Method: "go"}
	ctx := tr.ProductCallContext(&narrowed, call)
	if ctx.SelfType == nil || !typ.TypeEquals(ctx.SelfType, svc) {
		t.Fatalf("ProductCallContext SelfType = %v, want non-optional receiver %v", ctx.SelfType, svc)
	}
	if av, ok := ctx.RuntimeArgValueAt(0); !ok || !typ.TypeEquals(av.ProjectValue(), svc) {
		t.Fatalf("RuntimeArgValueAt(0) = %v/%v, want non-optional receiver", av.ProjectValue(), ok)
	}
}

func TestProductCallContextMethodReceiverUsesBranchNarrowedAliasLocal(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"store"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	storeSym := in.Scope.ParamSymbols[0]
	store := &ast.IdentExpr{Value: "store"}
	in.Graph.Bindings().Bind(store, storeSym)

	svc := typ.NewAlias("Svc", typ.NewRecord().
		Field("go", typ.Func().Build()).
		Build())
	tr := New(in, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(storeSym): product.FromType(typ.NewOptional(svc)),
	}}
	narrowed := tr.narrowByCondCheck(out, &cfg.BranchInfo{
		CondSymbol: storeSym,
		CondCheck:  cfg.CondCheck{Kind: cfg.CheckTruthy},
		Condition:  store,
	}, true, false)
	if av, ok := tr.symbolValue(&narrowed, storeSym); !ok || !av.DefinitelyPresent() {
		t.Fatalf("narrowed store = %v/%v present=%v, want present alias receiver", av.ProjectValue(), ok, av.DefinitelyPresent())
	}

	call := &ast.FuncCallExpr{Receiver: store, Method: "go"}
	ctx := tr.ProductCallContext(&narrowed, call)
	if ctx.SelfType == nil {
		t.Fatalf("ProductCallContext SelfType is nil for narrowed alias receiver")
	}
	if _, optional := typ.SplitNilableFieldType(ctx.SelfType); optional {
		t.Fatalf("ProductCallContext SelfType = %v, want non-optional alias receiver", ctx.SelfType)
	}
	if av, ok := ctx.RuntimeArgValueAt(0); !ok || !av.DefinitelyPresent() {
		t.Fatalf("RuntimeArgValueAt(0) = %v/%v present=%v, want present alias receiver", av.ProjectValue(), ok, av.DefinitelyPresent())
	}
}

func TestTransferTypeCastNarrowNormalizesConstKeyPath(t *testing.T) {
	obj := &ast.IdentExpr{Value: "obj"}
	key := &ast.IdentExpr{Value: "key"}
	access := &ast.AttrGetExpr{Object: obj, Key: key, KeySyntax: ast.AttrKeyIndex}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "Point"},
		Args: []ast.Expr{access},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"obj"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"key"},
				Exprs: []ast.Expr{&ast.StringExpr{Value: "p-q"}},
			},
			&ast.FuncCallStmt{Expr: call},
		},
	}
	in := input.BuildFromFunction(fn, nil, nil, "Point")
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build parameter/call")
	}
	objSym := in.Scope.ParamSymbols[0]
	var callPoint cfg.Point
	in.Graph.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info.Call == call {
			callPoint = p
		}
	})
	if callPoint == 0 {
		t.Fatal("call point not found")
	}
	pointType := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	tr := New(in, Config{CallTyper: constTypeCastTyper{target: pointType}})
	incoming := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(objSym): product.FromType(typ.NewRecord().
				StaticStringIndex("p-q", typ.Any).
				Build()),
		},
	}

	out := tr.Transfer(in.Graph, callPoint, incoming, nil, nil)

	segs := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "p-q"}}
	fact, ok := testStaticMemberValue(t, out.StaticMembers, objSym, segs)
	if !ok || !typ.TypeEquals(fact.ProjectValue(), pointType) {
		t.Fatalf("const-key type-cast fact = %v/%v; want %v", fact.ProjectValue(), ok, pointType)
	}
}

func TestEvalCallPassesUnannotatedParamCalleeAsProductGradualTop(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"fn"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	callee := &ast.IdentExpr{Value: "fn"}
	sym := in.Scope.ParamSymbols[0]
	in.Graph.Bindings().Bind(callee, sym)
	in.Graph.Bindings().SetName(sym, "fn")

	typer := &productReturnTestTyper{returns: []product.AbstractValue{product.GradualAny()}}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	if _, ok := tr.evalCall(&out, &ast.FuncCallExpr{Func: callee}, nil); !ok {
		t.Fatal("evalCall did not resolve through product call typer")
	}
	if typer.callee.IsZero() || !typer.callee.IsGradualTop() {
		t.Fatalf("callee product value = %v, want gradual-top evidence", typer.callee.ProjectValue())
	}
}

func TestEvalCallProductStrictAnyReturnStaysStrictAny(t *testing.T) {
	tr := New(input.Inputs{}, Config{CallTyper: strictAnyReturnTyper{}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	returns, ok := tr.evalCall(&out, &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "callee"}}, nil)
	if !ok || len(returns) != 1 {
		t.Fatalf("evalCall returned %d/%v, want one product return", len(returns), ok)
	}
	if returns[0].IsGradualTop() {
		t.Fatal("strict any return was incorrectly promoted to gradual-top evidence")
	}
	if !typ.IsAny(returns[0].ProjectValue()) {
		t.Fatalf("product return = %v, want strict any", returns[0].ProjectValue())
	}
}

func TestEvalCallPendingProductInputWithoutReturnEvidenceStaysUnknown(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"db"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	db := &ast.IdentExpr{Value: "db"}
	sym := in.Scope.ParamSymbols[0]
	in.Graph.Bindings().Bind(db, sym)
	in.Graph.Bindings().SetName(sym, "db")

	typer := &pendingBlocksTypeFallbackTyper{}
	tr := New(in, Config{CallTyper: typer})
	call := &ast.FuncCallExpr{Receiver: db, Method: "execute"}
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	if returns, ok := tr.evalCall(&out, call, nil); ok || len(returns) != 0 {
		t.Fatalf("pending product input evalCall = %v/%v, want no contribution", returns, ok)
	}
	if typer.productCalls != 1 {
		t.Fatalf("product return calls after pending input = %d, want 1", typer.productCalls)
	}

	out.Env[flow.SymbolValueKey(sym)] = product.FromType(typ.String)
	returns, ok := tr.evalCall(&out, call, nil)
	if ok || len(returns) != 0 {
		t.Fatalf("concrete receiver without product evidence = %v/%v, want no contribution", returns, ok)
	}
	if typer.productCalls != 2 {
		t.Fatalf("product return calls after concrete receiver = %d, want 2", typer.productCalls)
	}
}

type productReturnTestTyper struct {
	captureEffectTyper
	returns []product.AbstractValue
	args    []product.AbstractValue
	callee  product.AbstractValue
}

func productReturnResultForTest(returns ...product.AbstractValue) ProductCallResult {
	return ProductCallResult{
		ReturnValues:    append([]product.AbstractValue(nil), returns...),
		HasReturnValues: true,
		Boundary:        EmptyBoundaryOutcome(),
	}
}

func (p *productReturnTestTyper) ProductCallFromValues(
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
) ProductCallResult {
	p.args = append([]product.AbstractValue(nil), ctx.ArgValues...)
	if call != nil && ctx.ExprValue != nil {
		p.callee, _ = ctx.ExprValue(call.Func)
	}
	return productReturnResultForTest(p.returns...)
}

type deadCallDemandTyper struct {
	captureEffectTyper
	demand typ.Type
}

func (d deadCallDemandTyper) ProductCallFromValues(call *ast.FuncCallExpr, _ ProductCallContext) ProductCallResult {
	result := EmptyProductCallResult()
	if call == nil || len(call.Args) == 0 || d.demand == nil {
		return result
	}
	out := make([]callobligation.Obligation, len(call.Args))
	for i := range out {
		out[i] = callobligation.Body(d.demand)
	}
	result.Boundary.ArgDemands = out
	return result
}

type strictAnyReturnTyper struct {
	captureEffectTyper
}

func (strictAnyReturnTyper) ProductCallFromValues(*ast.FuncCallExpr, ProductCallContext) ProductCallResult {
	return productReturnResultForTest(product.FromType(typ.Any))
}

type pendingBlocksTypeFallbackTyper struct {
	captureEffectTyper
	productCalls int
}

func (p *pendingBlocksTypeFallbackTyper) ProductCallFromValues(*ast.FuncCallExpr, ProductCallContext) ProductCallResult {
	p.productCalls++
	return EmptyProductCallResult()
}

var _ CallTyper = (*productReturnTestTyper)(nil)
var _ ProductCallProvider = (*productReturnTestTyper)(nil)
var _ CallTyper = strictAnyReturnTyper{}
var _ ProductCallProvider = strictAnyReturnTyper{}
var _ CallTyper = (*pendingBlocksTypeFallbackTyper)(nil)
var _ ProductCallProvider = (*pendingBlocksTypeFallbackTyper)(nil)

type constTypeCastTyper struct {
	captureEffectTyper
	target typ.Type
}

func (c constTypeCastTyper) TypeCastTarget(*ast.FuncCallExpr, func(ast.Expr) typ.Type) (typ.Type, bool) {
	return c.target, c.target != nil
}

type nonReentrantProductTyper struct {
	captureEffectTyper
	t                     *testing.T
	calls                 int
	sawCallProjectionMiss bool
}

func (p *nonReentrantProductTyper) ProductCallFromValues(call *ast.FuncCallExpr, ctx ProductCallContext) ProductCallResult {
	p.calls++
	if p.calls > 1 {
		p.t.Fatalf("provider ExprValue re-entered evalCall for the same call")
	}
	if ctx.ExprValue == nil {
		p.t.Fatal("ProductCallContext missing ExprValue resolver")
	}
	if av, ok := ctx.ExprValue(call); ok || !av.IsZero() {
		p.t.Fatalf("ExprValue(call) = %v/%v, want projection miss", av, ok)
	}
	p.sawCallProjectionMiss = true
	return productReturnResultForTest(product.FromType(typ.String))
}

var _ ProductCallProvider = (*nonReentrantProductTyper)(nil)

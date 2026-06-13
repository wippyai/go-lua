package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerCallSitesPreserveAllSemanticContextsAndProducerStaysNarrow(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	local := localAssign([]string{"a"}, makeCall)
	printCall := &ast.FuncCallExpr{Func: ident("print")}
	printStmt := &ast.FuncCallStmt{Expr: printCall}
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	ifStmt := &ast.IfStmt{Condition: readyCall}
	iterCall := &ast.FuncCallExpr{Func: ident("iter")}
	genericFor := &ast.GenericForStmt{Names: []string{"item"}, Exprs: []ast.Expr{iterCall}}
	tailCall := &ast.FuncCallExpr{Func: ident("tail")}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{tailCall}}
	stmts := []ast.Stmt{local, printStmt, ifStmt, genericFor, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "print", "ready", "iter", "tail"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())

	localPoints := requireStmtPoints(t, built, local, 2)
	localSite, ok := facts.CallSite(localPoints[0])
	if !ok {
		t.Fatalf("missing assignment-source call site")
	}
	if localSite.Context() != factflow.CallSiteContextAssignmentSource || localSite.ExprIndex() != 0 || localSite.CalleeSymbol() == 0 {
		t.Fatalf("assignment-source call site = context %v expr index %d callee %d", localSite.Context(), localSite.ExprIndex(), localSite.CalleeSymbol())
	}
	if expr, ok := localSite.Expr(); !ok || expr == 0 {
		t.Fatalf("assignment-source call-site expr = %d/%v", expr, ok)
	}
	localTargets := localSite.ResultTargets()
	if len(localTargets) != 1 || localTargets[0].Kind() != factflow.CallResultTargetLocalAssignment || localTargets[0].Index() != 0 || localTargets[0].ResultIndex() != 0 {
		t.Fatalf("assignment-source call-site targets = %#v", localTargets)
	}
	if _, ok := facts.Call(localPoints[0]); !ok {
		t.Fatalf("assignment-source call point %d missing producer", localPoints[0])
	}

	statementPoint := requireStmtPoints(t, built, printStmt, 1)[0]
	statementSite, ok := facts.CallSite(statementPoint)
	if !ok {
		t.Fatalf("missing statement call site")
	}
	if statementSite.Context() != factflow.CallSiteContextStatement || !statementSite.Final() || !statementSite.Adjusted() || statementSite.Expanded() {
		t.Fatalf("statement call site flags/context = %v final=%v adjusted=%v expanded=%v", statementSite.Context(), statementSite.Final(), statementSite.Adjusted(), statementSite.Expanded())
	}
	if len(statementSite.ResultTargets()) != 0 {
		t.Fatalf("statement call-site targets = %#v, want none", statementSite.ResultTargets())
	}
	if _, ok := facts.Call(statementPoint); ok {
		t.Fatalf("statement call point %d lowered as call producer", statementPoint)
	}

	branchPoints := requireStmtPoints(t, built, ifStmt, 2)
	conditionSite, ok := facts.CallSite(branchPoints[0])
	if !ok {
		t.Fatalf("missing condition call site")
	}
	if conditionSite.Context() != factflow.CallSiteContextCondition || conditionSite.ExprIndex() != 0 || !conditionSite.Final() || !conditionSite.Adjusted() {
		t.Fatalf("condition call site = context %v expr index %d final=%v adjusted=%v", conditionSite.Context(), conditionSite.ExprIndex(), conditionSite.Final(), conditionSite.Adjusted())
	}
	if _, ok := facts.Call(branchPoints[0]); ok {
		t.Fatalf("condition call point %d lowered as call producer", branchPoints[0])
	}

	genericPoints := requireStmtPoints(t, built, genericFor, 3)
	iteratorSite, ok := facts.CallSite(genericPoints[0])
	if !ok {
		t.Fatalf("missing iterator call site")
	}
	if iteratorSite.Context() != factflow.CallSiteContextIteratorSource || iteratorSite.ExprIndex() != 0 {
		t.Fatalf("iterator call site = context %v expr index %d", iteratorSite.Context(), iteratorSite.ExprIndex())
	}
	if _, ok := facts.Call(genericPoints[0]); ok {
		t.Fatalf("iterator call point %d lowered as call producer", genericPoints[0])
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	returnSite, ok := facts.CallSite(returnPoints[0])
	if !ok {
		t.Fatalf("missing return-source call site")
	}
	if returnSite.Context() != factflow.CallSiteContextReturnSource || returnSite.ExprIndex() != 0 || !returnSite.OpenTail() {
		t.Fatalf("return-source call site = context %v expr index %d open tail=%v", returnSite.Context(), returnSite.ExprIndex(), returnSite.OpenTail())
	}
	returnTargets := returnSite.ResultTargets()
	if len(returnTargets) != 1 || returnTargets[0].Kind() != factflow.CallResultTargetReturn || returnTargets[0].Index() != 0 || returnTargets[0].ResultIndex() != 0 {
		t.Fatalf("return-source call-site targets = %#v", returnTargets)
	}
	if _, ok := facts.Call(returnPoints[0]); !ok {
		t.Fatalf("return-source call point %d missing producer", returnPoints[0])
	}
}

func TestLowerCallSitePreservesPortableCallShapeAndArgumentOverlays(t *testing.T) {
	obj := ident("obj")
	arg := ident("arg")
	other := ident("other")
	castArg := &ast.CastExpr{
		Expr:   arg,
		Type:   primitiveType("number"),
		Syntax: ast.CastSyntaxAs,
	}
	call := &ast.FuncCallExpr{
		Receiver: obj,
		Method:   "run",
		Args:     []ast.Expr{castArg, other},
		TypeArgs: []ast.TypeExpr{primitiveType("string"), primitiveType("number")},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"obj", "arg", "other"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	point := requireStmtPoints(t, built, stmt, 1)[0]
	site, ok := facts.CallSite(point)
	if !ok {
		t.Fatalf("missing call site at point %d", point)
	}
	if site.Context() != factflow.CallSiteContextStatement || site.MethodName() != "run" {
		t.Fatalf("call site context/method = %v/%q", site.Context(), site.MethodName())
	}
	receiverPath := path.NewPath(mustIdentSymbol(t, bindings, obj), "obj")
	methodPath := receiverPath.Field("run")
	if got, ok := site.ReceiverPath(); !ok || !got.Equal(receiverPath) {
		t.Fatalf("receiver path = %#v/%v, want %#v/true", got, ok, receiverPath)
	}
	if got, ok := site.MethodPath(); !ok || !got.Equal(methodPath) {
		t.Fatalf("method path = %#v/%v, want %#v/true", got, ok, methodPath)
	}
	if got := site.CalleePath(); !got.Equal(methodPath) {
		t.Fatalf("callee path = %#v, want %#v", got, methodPath)
	}

	args := site.ArgumentSources()
	if len(args) != 2 {
		t.Fatalf("argument sources = %#v, want two args", args)
	}
	if args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr || args[0].ExprRef == 0 || args[0].ExprIndex != 0 || args[0].TargetIndex != 0 || args[0].Final {
		t.Fatalf("first arg source = %#v", args[0])
	}
	if args[1].Kind != factflow.ValueSourceExpression || !args[1].HasExpr || args[1].ExprRef == 0 || args[1].ExprIndex != 1 || args[1].TargetIndex != 1 || !args[1].Final {
		t.Fatalf("second arg source = %#v", args[1])
	}
	assertLoweredAssertion(t, facts, args[0], assertion.Type(), factflow.ValueSourceExpression)

	typeArgs := site.TypeArgs()
	if len(typeArgs) != 2 || typeArgs[0] == 0 || typeArgs[1] == 0 || typeArgs[0] == typeArgs[1] {
		t.Fatalf("type args = %#v, want two distinct opaque refs", typeArgs)
	}
	if targets := site.ResultTargets(); len(targets) != 0 {
		t.Fatalf("statement method call targets = %#v, want none", targets)
	}
	if _, ok := facts.Call(point); ok {
		t.Fatalf("statement method call point %d lowered as call producer", point)
	}

	producerType := reflect.TypeOf(factflow.CallProducer{})
	for _, method := range []string{"ReceiverPath", "MethodPath", "MethodName", "ArgumentSources", "TypeArgs"} {
		if _, ok := producerType.MethodByName(method); ok {
			t.Fatalf("CallProducer unexpectedly exposes broad call-shape method %s", method)
		}
	}
}

func TestLowerCallSiteUsesSemanticArgumentSources(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	wrapped := &ast.CastExpr{
		Expr:   inner,
		Type:   primitiveType("number"),
		Syntax: ast.CastSyntaxAs,
	}
	outer := &ast.FuncCallExpr{
		Func: ident("f"),
		Args: []ast.Expr{wrapped},
	}
	innerPoint := cfg.Point(42)
	l := lowerer{
		exprs: make(map[any]factflow.ExprRef),
	}
	semanticSource := sourceprovenance.SourceForExpr(wrapped, 0, 0, 0, true, false, func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		return innerPoint, exprIndex == 0 && call == inner
	})
	site := l.callSite(semantics.CallFact{
		Context:         semantics.CallContextStatement,
		Call:            outer,
		Args:            []ast.Expr{ident("not_semantic_source")},
		ArgumentSources: []sourceprovenance.ASTSource{semanticSource},
	})
	args := site.ArgumentSources()
	if len(args) != 1 {
		t.Fatalf("argument sources = %#v, want one arg", args)
	}
	arg := args[0]
	if arg.Kind != factflow.ValueSourceCall || !arg.HasCallPoint || arg.CallPoint != innerPoint {
		t.Fatalf("nested call arg source = %#v, want call point %d", arg, innerPoint)
	}
}

func TestLowerNestedExpressionProducerCallIsReadableSlotZero(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	outer := &ast.FuncCallExpr{Func: ident("f"), Args: []ast.Expr{inner}}
	stmt := &ast.FuncCallStmt{Expr: outer}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f", "g"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	points := requireStmtPoints(t, built, stmt, 2)
	innerSite, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing inner call site")
	}
	if innerSite.Context() != factflow.CallSiteContextExpressionProducer {
		t.Fatalf("inner call context = %v, want expression producer", innerSite.Context())
	}
	targets := innerSite.ResultTargets()
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetExpression || targets[0].ResultIndex() != 0 {
		t.Fatalf("inner call targets = %#v", targets)
	}
	producer, ok := facts.Call(points[0])
	if !ok {
		t.Fatalf("missing nested call producer")
	}
	if producerTargets := producer.ResultTargets(); len(producerTargets) != 1 || producerTargets[0].Kind() != factflow.CallResultTargetExpression || producerTargets[0].ResultIndex() != 0 {
		t.Fatalf("nested producer targets = %#v", producerTargets)
	}
	outerSite, ok := facts.CallSite(points[1])
	if !ok {
		t.Fatalf("missing outer call site")
	}
	args := outerSite.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceCall || args[0].CallPoint != points[0] || !args[0].HasCallPoint || args[0].ResultIndex != 0 {
		t.Fatalf("outer argument sources = %#v, want inner call source", args)
	}
	if _, ok := facts.Call(points[1]); ok {
		t.Fatalf("outer statement call unexpectedly lowered as producer")
	}
}

func TestLowerCallSiteMapsUnknownContextExplicitly(t *testing.T) {
	l := lowerer{exprs: make(map[any]factflow.ExprRef)}
	site := l.callSite(semantics.CallFact{
		Context: semantics.CallContextUnknown,
		Call:    &ast.FuncCallExpr{Func: ident("mystery")},
	})
	if site.Context() != factflow.CallSiteContextUnknown {
		t.Fatalf("unknown semantic call context lowered as %v", site.Context())
	}
	if expr, ok := site.Expr(); !ok || expr == 0 {
		t.Fatalf("unknown call-site expr = %d/%v, want explicit expr ref", expr, ok)
	}
}

func TestLowerMemberOrdinaryCallTargetStaysCallSiteOnly(t *testing.T) {
	decl := localAssign([]string{"t"}, &ast.TableExpr{})
	targetRoot := ident("t")
	call := &ast.FuncCallExpr{Func: ident("f")}
	write := assign([]ast.Expr{dot(targetRoot, "x")}, call)
	stmts := []ast.Stmt{decl, write}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	points := requireStmtPoints(t, built, write, 2)
	producer, ok := facts.Call(points[0])
	if !ok {
		t.Fatalf("missing assignment call producer at point %d", points[0])
	}
	if producer.CalleeSymbol() == 0 {
		t.Fatalf("producer callee symbol missing")
	}
	if got := producer.ResultTargets(); len(got) != 0 {
		t.Fatalf("member ordinary target leaked into producer targets: %#v", got)
	}

	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	if site.Context() != factflow.CallSiteContextAssignmentSource {
		t.Fatalf("call-site context = %v, want assignment source", site.Context())
	}
	targets := site.ResultTargets()
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetOrdinaryAssignment || targets[0].Index() != 0 || targets[0].ResultIndex() != 0 {
		t.Fatalf("call-site targets = %#v", targets)
	}
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, targetRoot), "t").Field("x")
	if !targets[0].TargetPath().Equal(wantPath) {
		t.Fatalf("call-site target path = %v, want %v", targets[0].TargetPath(), wantPath)
	}
}

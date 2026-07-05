package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})

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
	if _, ok := callproducer.FromFacts(facts, localPoints[0]); !ok {
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
	if _, ok := callproducer.FromFacts(facts, statementPoint); ok {
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
	if conditionSite.ConditionNegated() {
		t.Fatalf("condition call site unexpectedly negated")
	}
	if _, ok := callproducer.FromFacts(facts, branchPoints[0]); ok {
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
	if _, ok := callproducer.FromFacts(facts, genericPoints[0]); ok {
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
	if _, ok := callproducer.FromFacts(facts, returnPoints[0]); !ok {
		t.Fatalf("return-source call point %d missing producer", returnPoints[0])
	}
}

func TestLowerMemberCallLocalAssignmentUsesCallResultSource(t *testing.T) {
	makeCall := &ast.FuncCallExpr{
		Func: dot(ident("builder"), "build"),
	}
	local := localAssign([]string{"batch"}, makeCall)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"builder"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	points := requireStmtPoints(t, built, local, 2)
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing member assignment call site")
	}
	if site.Context() != factflow.CallSiteContextAssignmentSource {
		t.Fatalf("member call context = %v, want assignment source", site.Context())
	}
	if _, ok := callproducer.FromFacts(facts, points[0]); !ok {
		t.Fatalf("member assignment call point %d missing producer", points[0])
	}
	assign, ok := facts.RootAssignment(points[1])
	if !ok {
		t.Fatalf("missing local assignment at point %d", points[1])
	}
	source := assign.Source()
	if source.Kind != factflow.ValueSourceCall || source.CallPoint != points[0] || !source.HasCallPoint || source.ResultIndex != 0 {
		t.Fatalf("member assignment source = %#v, want call result 0 from point %d", source, points[0])
	}
}

func TestLowerCallSiteResultTargetPathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = nil
    local other = nil
    value = make()
end
`, "make")
	assignStmt, ok := fn.Stmts[2].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want assignment", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, assignStmt, 2)
	callPoint := points[0]
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	makeSym, ok := bindings.GlobalSymbol("make")
	if !ok {
		t.Fatal("missing make global symbol")
	}
	makePath := path.NewPath(makeSym, "make")

	body := wir.NewBody("synthetic-call-target-owner")
	resultTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Call:    wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(makePath))}},
		Results: body.AppendOperands([]wir.Operand{resultTemp}),
	})
	body.SetPointRange(callPoint, start, start+1)
	body.SetCallResultTarget(callPoint, 0, otherPath)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	targets := site.ResultTargets()
	if len(targets) != 1 {
		t.Fatalf("call result targets = %#v, want one", targets)
	}
	if got := targets[0].TargetPath(); !got.Equal(otherPath) || got.Equal(valuePath) {
		t.Fatalf("call result target path = %v, want WIR path %v not semantic path %v", got, otherPath, valuePath)
	}
}

func TestLowerCallResultTargetPathAccessorComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = nil
    local other = nil
    value = make()
end
`, "make")
	assignStmt, ok := fn.Stmts[2].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want assignment", fn.Stmts[2])
	}
	callPoint := requireStmtPoints(t, built, assignStmt, 2)[0]
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	makeSym, ok := bindings.GlobalSymbol("make")
	if !ok {
		t.Fatal("missing make global symbol")
	}
	makePath := path.NewPath(makeSym, "make")
	fact, ok := result.Call(callPoint)
	if !ok {
		t.Fatalf("missing call fact at point %d", callPoint)
	}

	body := wir.NewBody("target-accessor")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: callPoint,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(makePath))}},
	})
	body.SetPointRange(callPoint, start, start+1)
	body.SetCallResultTarget(callPoint, 0, otherPath)

	got, ok := (&lowerer{wir: body}).callResultTargetPath(callPoint, fact, 0)
	if !ok || !got.Equal(otherPath) || got.Equal(valuePath) {
		t.Fatalf("call result target path = %v/%v, want WIR path %v not semantic path %v", got, ok, otherPath, valuePath)
	}
}

func TestLowerNegatedConditionCallSiteCarriesPolarity(t *testing.T) {
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	ifStmt := &ast.IfStmt{Condition: &ast.UnaryNotOpExpr{Expr: readyCall}}
	stmts := []ast.Stmt{ifStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ready"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	points := requireStmtPoints(t, built, ifStmt, 2)
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing condition call site")
	}
	if site.Context() != factflow.CallSiteContextCondition || !site.ConditionNegated() {
		t.Fatalf("condition call site = context %v negated=%v, want negated condition", site.Context(), site.ConditionNegated())
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

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
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
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourceExpression || !receiverSource.HasExpr || receiverSource.ExprRef == 0 {
		t.Fatalf("receiver source = %#v/%v, want lowered expression source", receiverSource, ok)
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
	assertLoweredAssertion(t, facts, args[0], concreteCastAssertionForType(typ.Number), factflow.ValueSourceExpression)

	typeArgs := site.TypeArgs()
	if len(typeArgs) != 2 || typeArgs[0] == 0 || typeArgs[1] == 0 || typeArgs[0] == typeArgs[1] {
		t.Fatalf("type args = %#v, want two distinct opaque refs", typeArgs)
	}
	if targets := site.ResultTargets(); len(targets) != 0 {
		t.Fatalf("statement method call targets = %#v, want none", targets)
	}
	if _, ok := callproducer.FromFacts(facts, point); ok {
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
		ArgumentSpans: []semantics.SourceSpan{
			{StartLine: 7, StartCol: 2, EndLine: 7, EndCol: 18},
		},
		ArgumentLabels: []string{"wrapped_call"},
	})
	args := site.ArgumentSources()
	if len(args) != 1 {
		t.Fatalf("argument sources = %#v, want one arg", args)
	}
	arg := args[0]
	if arg.Kind != factflow.ValueSourceCall || !arg.HasCallPoint || arg.CallPoint != innerPoint {
		t.Fatalf("nested call arg source = %#v, want call point %d", arg, innerPoint)
	}
	if span, ok := site.ArgumentSpanAt(0); !ok || span.StartLine != 7 || span.EndCol != 18 {
		t.Fatalf("argument span = %#v/%v, want lowered semantic span", span, ok)
	}
	if label, ok := site.ArgumentLabelAt(0); !ok || label != "wrapped_call" {
		t.Fatalf("argument label = %q/%v, want wrapped_call/true", label, ok)
	}
}

func TestLowerCallSiteUsesWIRArgumentSourcesForLiteralAndCallOperands(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value, "ok", 3, nil, true, produce())
end
`, "send", "produce")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 2)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	site, ok := facts.CallSite(points[1])
	if !ok {
		t.Fatalf("missing outer call site at point %d", points[1])
	}
	args := site.ArgumentSources()
	if len(args) != 6 {
		t.Fatalf("argument sources = %#v, want six", args)
	}
	wantParamKey := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value").Key()
	if args[0].Kind != factflow.ValueSourcePath || args[0].PathKey != wantParamKey || args[0].HasExpr {
		t.Fatalf("param argument source = %#v, want WIR path source %q", args[0], wantParamKey)
	}
	if args[1].Kind != factflow.ValueSourceLiteral || args[1].LiteralKind != factflow.ValueSourceLiteralString || args[1].String != "ok" || args[1].HasExpr {
		t.Fatalf("string argument source = %#v, want WIR string literal", args[1])
	}
	if args[2].Kind != factflow.ValueSourceLiteral || args[2].LiteralKind != factflow.ValueSourceLiteralInteger || args[2].Int != 3 || args[2].HasExpr {
		t.Fatalf("integer argument source = %#v, want WIR integer literal", args[2])
	}
	if args[3].Kind != factflow.ValueSourceNil {
		t.Fatalf("nil argument source = %#v", args[3])
	}
	if args[4].Kind != factflow.ValueSourceLiteral || args[4].LiteralKind != factflow.ValueSourceLiteralBool || !args[4].Bool || args[4].HasExpr {
		t.Fatalf("bool argument source = %#v, want WIR bool literal", args[4])
	}
	if args[5].Kind != factflow.ValueSourceCall || !args[5].HasCallPoint || args[5].CallPoint != points[0] ||
		!args[5].Final || !args[5].Expanded || args[5].Adjusted || args[5].OpenTail {
		t.Fatalf("tail call argument source = %#v, want expanded non-open call from point %d", args[5], points[0])
	}
}

func TestLowerCallSiteUsesWIRArgumentSourceForLocalRootPath(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = "x"
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[1].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[1])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok {
		t.Fatalf("missing first argument source")
	}
	want := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value").Key()
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr || arg.PathKey != "" {
		t.Fatalf("local argument source = %#v, want WIR expression-backed path source", arg)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok || got.Key() != want {
		t.Fatalf("local argument expression path = %v/%v, want %q", got, ok, want)
	}
}

func TestLowerCallSiteLocalArgumentPathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = "x"
    local other = "y"
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	valueStmt := fn.Stmts[0].(*ast.LocalAssignStmt)
	otherStmt := fn.Stmts[1].(*ast.LocalAssignStmt)
	valuePath := path.NewPath(mustLocalAt(t, bindings, valueStmt, 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, otherStmt, 0), "other")
	body := wir.NewBody("synthetic-call")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok || arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("argument source = %#v/%v, want expression-backed WIR path", arg, ok)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok || !got.Equal(otherPath) || got.Equal(valuePath) {
		t.Fatalf("argument expression path = %v/%v, want WIR path %v not semantic path %v", got, ok, otherPath, valuePath)
	}
}

func TestLowerCallSiteSegmentedArgumentPathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = { name = "x" }
    local other = { name = "y" }
    send(value.name)
end
`, "send")
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value").Field("name")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other").Field("name")
	body := wir.NewBody("synthetic-segmented-call-arg")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok || arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("argument source = %#v/%v, want expression-backed WIR path", arg, ok)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok || !got.Equal(otherPath) || got.Equal(valuePath) {
		t.Fatalf("argument expression path = %v/%v, want WIR path %v not semantic path %v", got, ok, otherPath, valuePath)
	}
}

func TestLowerDirectRootCallShapeComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local callee = function() end
    local other = function() end
    callee()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	calleePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "callee")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-direct-callee")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	if got := site.CalleeSymbol(); got != otherPath.Symbol || got == calleePath.Symbol {
		t.Fatalf("callee symbol = %d, want WIR symbol %d not semantic symbol %d", got, otherPath.Symbol, calleePath.Symbol)
	}
	if got := site.CalleePath(); !got.Equal(otherPath) || got.Equal(calleePath) {
		t.Fatalf("callee path = %v, want WIR path %v not semantic path %v", got, otherPath, calleePath)
	}
	if site.CalleeMemberAccess() {
		t.Fatalf("direct root call unexpectedly marked as member access")
	}
}

func TestLowerDirectMemberCallShapeComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { run = function() end }
    local other = { run = function() end }
    obj.run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	objPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "obj")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-direct-member-callee")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath.Field("run")))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	if got := site.CalleeSymbol(); got != otherPath.Symbol || got == objPath.Symbol {
		t.Fatalf("callee symbol = %d, want WIR symbol %d not semantic symbol %d", got, otherPath.Symbol, objPath.Symbol)
	}
	if got := site.CalleePath(); !got.Equal(otherPath.Field("run")) || got.Equal(objPath.Field("run")) {
		t.Fatalf("callee path = %v, want WIR path %v not semantic path %v", got, otherPath.Field("run"), objPath.Field("run"))
	}
	receiver, member, ok := site.CalleeMemberAccessPath()
	if !ok || !receiver.Equal(otherPath) || receiver.Equal(objPath) || member.Name != "run" {
		t.Fatalf("member access path = %v/%#v/%v, want WIR receiver %v .run not semantic receiver %v", receiver, member, ok, otherPath, objPath)
	}
}

func TestLowerMethodCallReceiverSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { run = function(self) end }
    local other = { run = function(self) end }
    obj:run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	objPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "obj")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-method-receiver")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "run"}),
			Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourcePath || receiverSource.PathKey != otherPath.Key() {
		t.Fatalf("receiver source = %#v/%v, want WIR path source %v", receiverSource, ok, otherPath)
	}
	receiverPath, ok := site.ReceiverPath()
	if !ok || !receiverPath.Equal(otherPath) || receiverPath.Equal(objPath) {
		t.Fatalf("receiver path = %v/%v, want WIR path %v not semantic path %v", receiverPath, ok, otherPath, objPath)
	}
	methodPath, ok := site.MethodPath()
	if !ok || !methodPath.Equal(otherPath.Field("run")) || methodPath.Equal(objPath.Field("run")) {
		t.Fatalf("method path = %v/%v, want WIR path %v not semantic path %v", methodPath, ok, otherPath.Field("run"), objPath.Field("run"))
	}
	if calleePath := site.CalleePath(); !calleePath.Equal(otherPath.Field("run")) || calleePath.Equal(objPath.Field("run")) {
		t.Fatalf("callee path = %v, want WIR path %v not semantic path %v", calleePath, otherPath.Field("run"), objPath.Field("run"))
	}
}

func TestLowerMethodCallSegmentedReceiverSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { child = { run = function(self) end } }
    local other = { child = { run = function(self) end } }
    obj.child:run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	objPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "obj").Field("child")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other").Field("child")
	body := wir.NewBody("synthetic-segmented-method-receiver")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "run"}),
			Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourceExpression || !receiverSource.HasExpr {
		t.Fatalf("receiver source = %#v/%v, want expression-backed WIR path", receiverSource, ok)
	}
	gotPath, ok := facts.ExpressionPath(receiverSource.ExprRef)
	if !ok || !gotPath.Equal(otherPath) || gotPath.Equal(objPath) {
		t.Fatalf("receiver source path = %v/%v, want WIR path %v not semantic path %v", gotPath, ok, otherPath, objPath)
	}
	receiverPath, ok := site.ReceiverPath()
	if !ok || !receiverPath.Equal(otherPath) || receiverPath.Equal(objPath) {
		t.Fatalf("receiver path = %v/%v, want WIR path %v not semantic path %v", receiverPath, ok, otherPath, objPath)
	}
}

func TestLowerMethodCallReceiverSourceDoesNotRequireSemanticReceiverSource(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { run = function(self) end }
    local other = { run = function(self) end }
    obj:run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	fact, ok := result.Call(points[0])
	if !ok {
		t.Fatalf("missing semantic call fact at point %d", points[0])
	}
	fact.HasReceiverSource = false
	fact.ReceiverSource = sourceprovenance.ASTSource{}

	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-method-receiver-no-sidecar")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "run"}),
			Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	l := lowerer{
		bindings:        bindings,
		wir:             body,
		exprs:           make(map[any]factflow.ExprRef),
		expressionPaths: make(map[factflow.ExprRef]path.Path),
	}
	site := l.callSiteWithArgumentSourcesAt(points[0], fact, nil)
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourcePath || receiverSource.PathKey != otherPath.Key() {
		t.Fatalf("receiver source = %#v/%v, want WIR path source %v without semantic receiver source", receiverSource, ok, otherPath)
	}
}

func TestLowerCallSiteDoesNotFallbackWhenWIRCallInstructionMissing(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: wir.NewBody("empty")})

	if _, ok := facts.CallSite(points[0]); ok {
		t.Fatalf("WIR mode call at point %d fell back to semantic sidecar", points[0])
	}
}

func TestLowerCallSiteUsesUnknownForUnsupportedDirectWIRArgument(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-unsupported-call-arg")
	unsupported := wir.Operand{Kind: wir.OperandType, Ref: uint32(body.InternType(typ.String))}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{unsupported}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	args := site.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceUnknown || args[0].TargetIndex != 0 {
		t.Fatalf("WIR call argument sources = %#v, want one unknown source for unsupported direct operand", args)
	}
}

func TestLowerCallSiteUsesUnknownForMissingTempWIRArgument(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-missing-temp-call-arg")
	unsupported := wir.Operand{Kind: wir.OperandTemp, Ref: 99}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{unsupported}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	args := site.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceUnknown || args[0].TargetIndex != 0 {
		t.Fatalf("WIR call argument sources = %#v, want one unknown source for missing temp operand", args)
	}
}

func TestLowerCallSiteClosureArgumentFunctionComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local other = function() end
    send(function() end)
end
`, "send")
	stmt, ok := fn.Stmts[1].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[1])
	}
	callExpr, ok := stmt.Expr.(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("call expr = %T, want function call", stmt.Expr)
	}
	callArg, ok := callExpr.Args[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("argument = %T, want function expression", callExpr.Args[0])
	}
	actualSym, ok := bindings.FunctionSymbol(callArg)
	if !ok || actualSym == 0 {
		t.Fatalf("actual argument function symbol = %d/%v", actualSym, ok)
	}
	otherAssign, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	otherFn, ok := otherAssign.Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("other expr = %T, want function expression", otherAssign.Exprs[0])
	}
	otherSym, ok := bindings.FunctionSymbol(otherFn)
	if !ok || otherSym == 0 || otherSym == actualSym {
		t.Fatalf("other function symbol = %d/%v, actual=%d", otherSym, ok, actualSym)
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-closure-call-arg")
	closureTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	proto := body.AddProto(wir.FuncProto{Name: "other", Symbol: otherSym})
	start := body.Emit(wir.Instruction{
		Op:    wir.OpClosure,
		Point: points[0],
		Dst:   closureTemp,
		Func:  proto,
	})
	body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{closureTemp}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok || arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("argument source = %#v/%v, want WIR closure expression source", arg, ok)
	}
	gotSym, ok := facts.ExpressionFunction(arg.ExprRef)
	if !ok || gotSym != otherSym || gotSym == actualSym {
		t.Fatalf("expression function = %d/%v, want WIR proto symbol %d not semantic argument symbol %d", gotSym, ok, otherSym, actualSym)
	}
}

func TestLowerCallSiteDoesNotFallbackToSemanticCalleeWhenWIRDirectCalleeUnsupported(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	valuePath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value")
	body := wir.NewBody("synthetic-unsupported-call-callee")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandTemp, Ref: 7}},
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(valuePath))}}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	if site.CalleeSymbol() != 0 || !site.CalleePath().IsEmpty() || site.CalleeMemberAccess() {
		t.Fatalf("WIR unsupported callee kept semantic callee identity: symbol=%d path=%s member=%v", site.CalleeSymbol(), site.CalleePath(), site.CalleeMemberAccess())
	}
}

func TestLowerMethodCallNameComesFromWIRForExpressionReceiver(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(obj: any): ()
    obj:run()
end
`)
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-expression-receiver-method")
	receiverTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 9}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "stop"}),
			Receiver: receiverTemp,
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	if !site.CalleeMemberAccess() || site.MethodName() != "stop" {
		t.Fatalf("method call shape = member:%v method:%q, want WIR method stop", site.CalleeMemberAccess(), site.MethodName())
	}
	if _, ok := site.ReceiverPath(); ok {
		t.Fatalf("expression receiver unexpectedly kept semantic receiver path")
	}
	if _, ok := site.MethodPath(); ok {
		t.Fatalf("expression receiver unexpectedly kept semantic method path")
	}
}

func TestLowerCallSitePreservesMemberAccessEvidence(t *testing.T) {
	l := lowerer{exprs: make(map[any]factflow.ExprRef)}
	site := l.callSite(semantics.CallFact{
		Context:            semantics.CallContextStatement,
		Call:               &ast.FuncCallExpr{Func: ident("make")},
		CalleePath:         path.Path{Root: "api"}.Field("make"),
		HasCalleePath:      true,
		CalleeMemberAccess: true,
	})
	if !site.CalleeMemberAccess() {
		t.Fatalf("lowered call site dropped member-access evidence")
	}
}

func TestLowerIteratorCallSitePreservesArgumentPath(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local state = {
    active_sessions = {},
}

for id, session_info in pairs(state.active_sessions) do
end
`, "pairs")
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})

	genericFor, ok := stmts[1].(*ast.GenericForStmt)
	if !ok {
		t.Fatalf("statement 1 = %T, want *ast.GenericForStmt", stmts[1])
	}
	points := built.StmtPoints.PointsFor(genericFor)
	if len(points) == 0 {
		t.Fatalf("generic-for points = %v, want iterator call point", points)
	}
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing iterator call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok {
		t.Fatalf("missing iterator call argument source")
	}
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("iterator argument source = %#v, want expression source", arg)
	}
	stateSym := mustLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	want := path.NewPath(stateSym, "state").Field("active_sessions")
	semanticFact, ok := result.Call(points[0])
	if !ok || len(semanticFact.ArgumentSources) != 1 {
		t.Fatalf("semantic iterator call fact = %#v/%v", semanticFact, ok)
	}
	if got, ok := pathexpr.Resolve(semanticFact.ArgumentSources[0].Expr, bindings); !ok || !got.Equal(want) {
		t.Fatalf("semantic iterator argument path = %v/%v, want %v", got, ok, want)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok {
		t.Fatalf("missing expression path for iterator argument ref %d", arg.ExprRef)
	}
	if !got.Equal(want) {
		t.Fatalf("iterator argument path = %v, want %v", got, want)
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
	producer, ok := callproducer.FromFacts(facts, points[0])
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
	if _, ok := callproducer.FromFacts(facts, points[1]); ok {
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
	producer, ok := callproducer.FromFacts(facts, points[0])
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

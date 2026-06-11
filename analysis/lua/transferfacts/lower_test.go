package transferfacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/valuesource"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func lowerFacts(t *testing.T, result *semantics.Result, graph cfg.Graph, reg *axis.Registry) factflow.Facts {
	t.Helper()
	if reg == nil {
		t.Fatal("lowerFacts requires a registry")
	}
	return Lower(result, graph, Config{Registry: reg})
}

func TestLowerPanicsWithoutRegistry(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, "local x = 1")
	_ = stmts
	_ = bindings
	_ = built

	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Config.Registry is required") {
			t.Fatal("Lower did not panic")
		}
	}()

	_ = Lower(result, built.Graph, Config{})
}

func TestLowerPanicsWithoutRegistryOnEmptyInputs(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Config.Registry is required") {
			t.Fatal("Lower did not panic")
		}
	}()

	_ = Lower(nil, nil, Config{})
}

func TestLowerDoesNotLowerDeclarationOrControlSidecars(t *testing.T) {
	typeDef := &ast.TypeDefStmt{Name: "User", Type: primitiveType("number")}
	interfaceDef := &ast.InterfaceDefStmt{Name: "Serializable"}
	funcDef := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: ident("build")},
		Func: &ast.FunctionExpr{ParList: &ast.ParList{}},
	}
	numericFor := &ast.NumberForStmt{
		Name:  "i",
		Init:  number("1"),
		Limit: number("3"),
		Step:  number("1"),
	}
	genericFor := &ast.GenericForStmt{
		Names: []string{"item"},
		Exprs: []ast.Expr{ident("items")},
	}
	label := &ast.LabelStmt{Name: "done"}
	gotoStmt := &ast.GotoStmt{Label: "done"}
	stmts := []ast.Stmt{typeDef, interfaceDef, funcDef, numericFor, genericFor, label, gotoStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"build", "items"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	for _, point := range requireStmtPoints(t, built, typeDef, 1) {
		if fact, ok := result.TypeDefinition(point); !ok || fact.Kind != cfgfacts.TypeDefinitionAlias {
			t.Fatalf("missing type definition sidecar at point %d: %#v/%v", point, fact, ok)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, interfaceDef, 1) {
		if fact, ok := result.TypeDefinition(point); !ok || fact.Kind != cfgfacts.TypeDefinitionInterface {
			t.Fatalf("missing interface definition sidecar at point %d: %#v/%v", point, fact, ok)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, funcDef, 1) {
		if _, ok := result.FunctionDefinition(point); !ok {
			t.Fatalf("missing function definition sidecar at point %d", point)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, numericFor, 2) {
		if _, ok := result.NumericFor(point); !ok {
			t.Fatalf("missing numeric-for sidecar at point %d", point)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, genericFor, 2) {
		if _, ok := result.GenericFor(point); !ok {
			t.Fatalf("missing generic-for sidecar at point %d", point)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, label, 1) {
		if _, ok := result.Label(point); !ok {
			t.Fatalf("missing label sidecar at point %d", point)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, gotoStmt, 1) {
		if _, ok := result.Goto(point); !ok {
			t.Fatalf("missing goto sidecar at point %d", point)
		}
		assertNoPointFact(t, facts, point)
	}
}

func TestLowerAssignmentsReturnsAndCallsPreserveValueListMetadata(t *testing.T) {
	makeIdent := ident("make")
	makeCall := &ast.FuncCallExpr{Func: makeIdent}
	packIdent := ident("pack")
	packCall := &ast.FuncCallExpr{Func: packIdent}
	local := localAssign([]string{"a", "b", "c"}, makeCall, packCall)

	aWrite := ident("a")
	putIdent := ident("put")
	putCall := &ast.FuncCallExpr{Func: putIdent}
	write := assign([]ast.Expr{aWrite}, putCall)

	aRead := ident("a")
	tailIdent := ident("tail")
	tailCall := &ast.FuncCallExpr{Func: tailIdent}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{aRead, tailCall}}

	stmts := []ast.Stmt{local, write, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "pack", "put", "tail"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	localPoints := requireStmtPoints(t, built, local, 5)
	makeProducer, ok := facts.Call(localPoints[0])
	if !ok {
		t.Fatalf("missing make call producer")
	}
	if makeProducer.Context() != factflow.CallProducerContextAssignment || makeProducer.ExprIndex() != 0 || makeProducer.Final() || makeProducer.Expanded() || !makeProducer.Adjusted() || makeProducer.OpenTail() {
		t.Fatalf("make producer flags/context are wrong")
	}
	makeRef, ok := makeProducer.Expr()
	if !ok || makeRef == 0 {
		t.Fatalf("make producer expr ref = %d/%v", makeRef, ok)
	}
	makeTargets := makeProducer.ResultTargets()
	if len(makeTargets) != 1 || makeTargets[0].Kind() != factflow.CallResultTargetLocalAssignment || makeTargets[0].Index() != 0 {
		t.Fatalf("make targets = %#v", makeTargets)
	}

	aFact, ok := facts.LocalAssignment(localPoints[2])
	if !ok {
		t.Fatalf("missing local a fact")
	}
	aSource := aFact.Source()
	if aFact.TargetSymbol() != mustLocalAt(t, bindings, local, 0) || !aFact.TargetPath().Equal(path.NewPath(aFact.TargetSymbol(), "a")) {
		t.Fatalf("local a target = %d %v", aFact.TargetSymbol(), aFact.TargetPath())
	}
	if aSource.Kind != factflow.ValueSourceCall || aSource.ExprRef != makeRef || !aSource.HasExpr || aSource.ExprIndex != 0 || aSource.TargetIndex != 0 || aSource.ResultIndex != 0 || aSource.CallPoint != localPoints[0] || !aSource.HasCallPoint || !aSource.Adjusted {
		t.Fatalf("local a source = %#v, make ref %d", aSource, makeRef)
	}

	packProducer, ok := facts.Call(localPoints[1])
	if !ok {
		t.Fatalf("missing pack call producer")
	}
	packRef, ok := packProducer.Expr()
	if !ok || packRef == 0 || packRef == makeRef {
		t.Fatalf("pack producer expr ref = %d/%v, make ref %d", packRef, ok, makeRef)
	}
	if !packProducer.Final() || !packProducer.Expanded() || packProducer.Adjusted() || packProducer.OpenTail() {
		t.Fatalf("pack producer flags are wrong")
	}
	packTargets := packProducer.ResultTargets()
	if len(packTargets) != 2 || packTargets[0].Index() != 1 || packTargets[1].Index() != 2 {
		t.Fatalf("pack targets = %#v", packTargets)
	}
	bFact, ok := facts.LocalAssignment(localPoints[3])
	if !ok {
		t.Fatalf("missing local b fact")
	}
	cFact, ok := facts.LocalAssignment(localPoints[4])
	if !ok {
		t.Fatalf("missing local c fact")
	}
	if got := bFact.Source(); got.ExprRef != packRef || got.ResultIndex != 0 || !got.Expanded || got.OpenTail {
		t.Fatalf("local b source = %#v, pack ref %d", got, packRef)
	}
	if got := cFact.Source(); got.ExprRef != packRef || got.ResultIndex != 1 || !got.Expanded || got.OpenTail {
		t.Fatalf("local c source = %#v, pack ref %d", got, packRef)
	}

	writePoints := requireStmtPoints(t, built, write, 2)
	putProducer, ok := facts.Call(writePoints[0])
	if !ok {
		t.Fatalf("missing put call producer")
	}
	putRef, ok := putProducer.Expr()
	if !ok || putRef == 0 {
		t.Fatalf("put producer expr ref = %d/%v", putRef, ok)
	}
	ordinary, ok := facts.OrdinaryAssignment(writePoints[1])
	if !ok {
		t.Fatalf("missing ordinary root assignment")
	}
	if ordinary.TargetSymbol() != mustIdentSymbol(t, bindings, aWrite) {
		t.Fatalf("ordinary target = %d, want %d", ordinary.TargetSymbol(), mustIdentSymbol(t, bindings, aWrite))
	}
	if got := ordinary.Source(); got.Kind != factflow.ValueSourceCall || got.ExprRef != putRef || !got.HasExpr || got.CallPoint != writePoints[0] || !got.HasCallPoint {
		t.Fatalf("ordinary source = %#v, put ref %d", got, putRef)
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	tailProducer, ok := facts.Call(returnPoints[0])
	if !ok {
		t.Fatalf("missing tail call producer")
	}
	if tailProducer.Context() != factflow.CallProducerContextReturn || !tailProducer.Final() || !tailProducer.Expanded() || !tailProducer.OpenTail() {
		t.Fatalf("tail producer flags/context are wrong")
	}
	tailRef, ok := tailProducer.Expr()
	if !ok || tailRef == 0 {
		t.Fatalf("tail producer expr ref = %d/%v", tailRef, ok)
	}
	tailTargets := tailProducer.ResultTargets()
	if len(tailTargets) != 1 || tailTargets[0].Kind() != factflow.CallResultTargetReturn || tailTargets[0].Index() != 1 {
		t.Fatalf("tail targets = %#v", tailTargets)
	}
	returnFact, ok := facts.Return(returnPoints[1])
	if !ok {
		t.Fatalf("missing return fact")
	}
	sources := returnFact.Sources()
	if len(sources) != 2 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr || sources[1].Kind != factflow.ValueSourceCall {
		t.Fatalf("return sources = %#v", sources)
	}
	if sources[1].ExprRef != tailRef || !sources[1].Expanded || !sources[1].OpenTail || sources[1].CallPoint != returnPoints[0] || !sources[1].HasCallPoint {
		t.Fatalf("return tail source = %#v, tail ref %d", sources[1], tailRef)
	}
}

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

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())

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
	if len(localTargets) != 1 || localTargets[0].Kind() != factflow.CallResultTargetLocalAssignment || localTargets[0].Index() != 0 {
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
	if len(returnTargets) != 1 || returnTargets[0].Kind() != factflow.CallResultTargetReturn || returnTargets[0].Index() != 0 {
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

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
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

func TestLowerCallSiteArgumentNestedCallSourceHasCallPoint(t *testing.T) {
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
		exprs:      make(map[any]factflow.ExprRef),
		callPoints: map[*ast.FuncCallExpr]cfg.Point{inner: innerPoint},
	}
	site := l.callSite(semantics.CallFact{
		Context: semantics.CallContextStatement,
		Call:    outer,
		Args:    []ast.Expr{wrapped},
	})
	args := site.ArgumentSources()
	if len(args) != 1 {
		t.Fatalf("argument sources = %#v, want one arg", args)
	}
	arg := args[0]
	if arg.Kind != factflow.ValueSourceCall || !arg.HasCallPoint || arg.CallPoint != innerPoint {
		t.Fatalf("nested call arg source = %#v, want call point %d", arg, innerPoint)
	}

	wrappedSource := l.argumentValueSource(wrapped, 0, true)
	if wrappedSource.Kind != factflow.ValueSourceCall || !wrappedSource.HasCallPoint || wrappedSource.CallPoint != innerPoint {
		t.Fatalf("wrapped nested call arg source = %#v, want call point %d", wrappedSource, innerPoint)
	}
	wrappedSemanticSource := l.argumentSemanticValueSource(wrapped, 0, true)
	if wrappedSemanticSource.Kind != valuesource.Call || !wrappedSemanticSource.HasCallPoint || wrappedSemanticSource.CallPoint != innerPoint {
		t.Fatalf("wrapped nested call semantic source = %#v, want call point %d", wrappedSemanticSource, innerPoint)
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

func TestLowerOrdinaryAssignmentsSplitsRootAndStaticPathWrites(t *testing.T) {
	local := localAssign([]string{"t", "k", "x"}, number("0"), stringLit("key"), number("0"))
	dotWrite := assign([]ast.Expr{dot(ident("t"), "x")}, number("1"))
	indexWrite := assign([]ast.Expr{stringIndex(ident("t"), "x")}, number("2"))
	dynamicWrite := assign([]ast.Expr{dynamicIndex(ident("t"), ident("k"))}, number("3"))
	rootWrite := assign([]ast.Expr{ident("x")}, number("4"))
	stmts := []ast.Stmt{local, dotWrite, indexWrite, dynamicWrite, rootWrite}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	tSym := mustLocalAt(t, bindings, local, 0)

	dotPoint := requireStmtPoints(t, built, dotWrite, 1)[0]
	dotFact, ok := facts.PathAssignment(dotPoint)
	if !ok {
		t.Fatalf("missing dot path assignment")
	}
	if !dotFact.TargetPath().Equal(path.NewPath(tSym, "t").Field("x")) {
		t.Fatalf("dot path assignment target = %v", dotFact.TargetPath())
	}
	if _, ok := facts.OrdinaryAssignment(dotPoint); ok {
		t.Fatalf("dot path assignment also lowered as root assignment")
	}

	indexPoint := requireStmtPoints(t, built, indexWrite, 1)[0]
	indexFact, ok := facts.PathAssignment(indexPoint)
	if !ok {
		t.Fatalf("missing static index path assignment")
	}
	if !indexFact.TargetPath().Equal(path.NewPath(tSym, "t").IndexStr("x")) {
		t.Fatalf("static index path assignment target = %v", indexFact.TargetPath())
	}

	dynamicPoint := requireStmtPoints(t, built, dynamicWrite, 1)[0]
	if _, ok := facts.PathAssignment(dynamicPoint); ok {
		t.Fatalf("dynamic index lowered as path assignment")
	}
	if _, ok := facts.OrdinaryAssignment(dynamicPoint); ok {
		t.Fatalf("dynamic index lowered as ordinary root assignment")
	}

	rootPoint := requireStmtPoints(t, built, rootWrite, 1)[0]
	rootFact, ok := facts.OrdinaryAssignment(rootPoint)
	if !ok {
		t.Fatalf("missing root ordinary assignment")
	}
	if rootFact.TargetSymbol() != mustLocalAt(t, bindings, local, 2) {
		t.Fatalf("root target = %d, want x symbol", rootFact.TargetSymbol())
	}
	if _, ok := facts.PathAssignment(rootPoint); ok {
		t.Fatalf("root ordinary assignment also lowered as path assignment")
	}
}

func TestLowerObjectLiteralSidecarUsesAssignmentExprRef(t *testing.T) {
	leafValue := number("1")
	stringValue := number("2")
	dynamicValue := number("3")
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("leaf"), KeySyntax: ast.AttrKeyDot, Value: leafValue},
		{Key: stringLit("key"), KeySyntax: ast.AttrKeyIndex, Value: stringValue},
		{Key: ident("dynamic"), KeySyntax: ast.AttrKeyIndex, Value: dynamicValue},
	}}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	point := requireStmtPoints(t, built, local, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	source := localFact.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr || source.ExprRef == 0 {
		t.Fatalf("local source = %#v, want expression source with expr ref", source)
	}
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar for assignment expr ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 2 {
		t.Fatalf("literal entries = %#v, want two static entries", entries)
	}
	assertLoweredObjectEntry(t, entries[0], fieldSuffix("leaf"), factflow.ValueSourceExpression)
	assertLoweredObjectEntry(t, entries[1], stringSuffix("key"), factflow.ValueSourceExpression)
	if entries[0].Source().ExprRef == source.ExprRef || entries[1].Source().ExprRef == source.ExprRef {
		t.Fatalf("entry source expr refs reused table expr ref: source=%#v entries=%#v", source, entries)
	}
}

func TestLowerWrappedObjectLiteralKeepsAssertionOverlayAndEntries(t *testing.T) {
	leafValue := number("1")
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("leaf"), KeySyntax: ast.AttrKeyDot, Value: leafValue},
	}}
	cast := &ast.CastExpr{
		Expr:   table,
		Type:   primitiveType("any"),
		Syntax: ast.CastSyntaxAs,
	}
	local := localAssign([]string{"t"}, cast)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	point := requireStmtPoints(t, built, local, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	source := localFact.Source()
	assertLoweredAssertion(t, facts, source, assertion.Any(), factflow.ValueSourceExpression)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar for wrapped assignment expr ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 1 {
		t.Fatalf("literal entries = %#v, want one static entry", entries)
	}
	assertLoweredObjectEntry(t, entries[0], fieldSuffix("leaf"), factflow.ValueSourceExpression)
}

func TestLowerIdentifierNilTruthyFalsyBranches(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	nilRead := ident("x")
	nilStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "==", Lhs: nilRead, Rhs: &ast.NilExpr{}}}
	notNilRead := ident("x")
	notNilStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "~=", Lhs: notNilRead, Rhs: &ast.NilExpr{}}}
	truthyRead := ident("x")
	truthyStmt := &ast.IfStmt{Condition: truthyRead}
	falsyRead := ident("x")
	falsyStmt := &ast.IfStmt{Condition: &ast.UnaryNotOpExpr{Expr: falsyRead}}
	stmts := []ast.Stmt{decl, nilStmt, notNilStmt, truthyStmt, falsyStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	xPath := path.NewPath(mustIdentSymbol(t, bindings, nilRead), "x")
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, nilStmt, 1)[0], xPath, presence.Absent(), true, presence.Present(), true)
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, notNilStmt, 1)[0], xPath, presence.Present(), true, presence.Absent(), true)
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, truthyStmt, 1)[0], xPath, presence.Present(), true, presence.Bottom(), false)
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, falsyStmt, 1)[0], xPath, presence.Bottom(), false, presence.Present(), true)
}

func TestLowerMemberPathBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"t"}, &ast.TableExpr{})
	rootRead := ident("t")
	memberStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "~=", Lhs: dot(rootRead, "child"), Rhs: &ast.NilExpr{}}}
	stmts := []ast.Stmt{decl, memberStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, rootRead), "t").Field("child")
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, memberStmt, 1)[0], wantPath, presence.Present(), true, presence.Absent(), true)
}

func TestLowerTypeGuardTableEqualityBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(xRead),
		Rhs:      stringLit("table"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Table),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Table),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerTypeGuardFunctionInequalityBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      typeCall(xRead),
		Rhs:      stringLit("function"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Function),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Function),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerTypeGuardNilBranchRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	eqRead := ident("x")
	eqStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(eqRead),
		Rhs:      stringLit("nil"),
	}}
	notRead := ident("x")
	notStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      typeCall(notRead),
		Rhs:      stringLit("nil"),
	}}
	stmts := []ast.Stmt{decl, eqStmt, notStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	xPath := path.NewPath(mustIdentSymbol(t, bindings, eqRead), "x")
	nilValue := valueRefinementExpectation{
		presence:       presence.Absent(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Singleton(runtimekind.Nil),
		hasRuntimeKind: true,
	}
	notNilValue := valueRefinementExpectation{
		presence:       presence.Present(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Top().Without(runtimekind.Nil),
		hasRuntimeKind: true,
	}
	assertLoweredBranchValueRefinement(t, facts, requireStmtPoints(t, built, eqStmt, 1)[0], xPath, nilValue, notNilValue)
	assertLoweredBranchValueRefinement(t, facts, requireStmtPoints(t, built, notStmt, 1)[0], xPath, notNilValue, nilValue)
}

func TestLowerTypeGuardRuntimeTypeNames(t *testing.T) {
	l := lowerer{registry: product.DefaultRegistry()}
	target := path.NewPath(symbol.ID(1), "x")
	tests := []struct {
		typeName string
		tag      runtimekind.Tag
	}{
		{"nil", runtimekind.Nil},
		{"boolean", runtimekind.Boolean},
		{"number", runtimekind.Number},
		{"string", runtimekind.String},
		{"table", runtimekind.Table},
		{"function", runtimekind.Function},
		{"thread", runtimekind.Thread},
		{"userdata", runtimekind.Userdata},
	}

	for _, tt := range tests {
		refinement, ok := l.typeBranchRefinement(target, branchcond.CheckTypeEqual, tt.typeName)
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) returned false", tt.typeName)
		}
		trueValue, ok := refinement.TrueValue()
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) missing true-edge refinement", tt.typeName)
		}
		falseValue, ok := refinement.FalseValue()
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) missing false-edge refinement", tt.typeName)
		}

		truePresence := presence.Present()
		falsePresence := presence.Top()
		falseHasPresence := false
		if tt.tag == runtimekind.Nil {
			truePresence = presence.Absent()
			falsePresence = presence.Present()
			falseHasPresence = true
		}
		assertValueRefinement(t, tt.typeName+" true edge", trueValue, valueRefinementExpectation{
			presence:       truePresence,
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(tt.tag),
			hasRuntimeKind: true,
		})
		assertValueRefinement(t, tt.typeName+" false edge", falseValue, valueRefinementExpectation{
			presence:       falsePresence,
			hasPresence:    falseHasPresence,
			runtimeKind:    runtimekind.Top().Without(tt.tag),
			hasRuntimeKind: true,
		})
	}
}

func TestLowerTypeGuardReversedOperandsBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      stringLit("table"),
		Rhs:      typeCall(xRead),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Table),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Table),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerSkipsUnknownTypeGuardBranchRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(ident("x")),
		Rhs:      stringLit("mystery"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	if _, ok := facts.BranchRefinement(point); ok {
		t.Fatalf("unknown type guard branch point %d lowered as branch refinement", point)
	}
}

func TestLowerNestedObjectLiteralEntriesUnderAssignmentExprRef(t *testing.T) {
	rootLeaf := number("1")
	nestedLeaf := number("2")
	dynamicValue := number("3")
	nested := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("b"), KeySyntax: ast.AttrKeyDot, Value: nestedLeaf},
		{Key: ident("dynamic"), KeySyntax: ast.AttrKeyIndex, Value: dynamicValue},
	}}
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("x"), KeySyntax: ast.AttrKeyDot, Value: rootLeaf},
		{Key: stringLit("a"), KeySyntax: ast.AttrKeyDot, Value: nested},
	}}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	point := requireStmtPoints(t, built, local, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	source := localFact.Source()
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar for assignment expr ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 3 {
		t.Fatalf("literal entries = %#v, want root, nested root, and nested leaf", entries)
	}
	assertLoweredObjectEntry(t, entries[0], fieldSuffix("x"), factflow.ValueSourceExpression)
	assertLoweredObjectEntry(t, entries[1], fieldSuffix("a"), factflow.ValueSourceExpression)
	assertLoweredObjectEntry(t, entries[2], fieldChainSuffix("a", "b"), factflow.ValueSourceExpression)
}

func TestLowerClaimsToSidecarsWithoutProofRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeRead := ident("x")
	anyRead := ident("x")
	nonNilRead := ident("x")
	typeCast := &ast.CastExpr{Expr: typeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
	anyCast := &ast.CastExpr{Expr: anyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxColonColon}
	nonNil := &ast.NonNilAssertExpr{Expr: nonNilRead}
	local := localAssign([]string{"a", "b", "c"}, typeCast, anyCast, nonNil)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	points := requireStmtPoints(t, built, local, 3)
	typeSource := mustLocalSource(t, facts, points[0])
	anySource := mustLocalSource(t, facts, points[1])
	nonNilSource := mustLocalSource(t, facts, points[2])

	assertLoweredAssertion(t, facts, typeSource, assertion.Type(), factflow.ValueSourceExpression)
	assertLoweredAssertion(t, facts, anySource, assertion.Any(), factflow.ValueSourceExpression)
	assertLoweredAssertion(t, facts, nonNilSource, assertion.NonNil(), factflow.ValueSourceExpression)
	if _, ok := facts.BranchRefinement(points[2]); ok {
		t.Fatalf("x! assignment produced branch/presence refinement")
	}
}

func TestLowerClaimsPreserveCastSyntaxVariantsWithoutProofRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	asTypeRead := ident("x")
	colonTypeRead := ident("x")
	asAnyRead := ident("x")
	colonAnyRead := ident("x")
	asTypeCast := &ast.CastExpr{Expr: asTypeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
	colonTypeCast := &ast.CastExpr{Expr: colonTypeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxColonColon}
	asAnyCast := &ast.CastExpr{Expr: asAnyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxAs}
	colonAnyCast := &ast.CastExpr{Expr: colonAnyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxColonColon}
	local := localAssign([]string{"a", "b", "c", "d"}, asTypeCast, colonTypeCast, asAnyCast, colonAnyCast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	points := requireStmtPoints(t, built, local, 4)
	cases := []struct {
		name  string
		point cfg.Point
		want  assertion.Value
	}{
		{name: "as type", point: points[0], want: assertion.Type()},
		{name: "colon type", point: points[1], want: assertion.Type()},
		{name: "as any", point: points[2], want: assertion.Any()},
		{name: "colon any", point: points[3], want: assertion.Any()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := mustLocalSource(t, facts, tc.point)
			assertLoweredAssertion(t, facts, source, tc.want, factflow.ValueSourceExpression)
		})
	}
}

func TestLowerParsedCastClaimsOnlyProduceClaimOverlays(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
local a, b, c, d = x as number, x :: number, x as any, x :: any
`)

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 4)
	cases := []struct {
		name  string
		point cfg.Point
		want  assertion.Value
	}{
		{name: "as number", point: points[0], want: assertion.Type()},
		{name: "colon number", point: points[1], want: assertion.Type()},
		{name: "as any", point: points[2], want: assertion.Any()},
		{name: "colon any", point: points[3], want: assertion.Any()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := mustLocalSource(t, facts, tc.point)
			assertLoweredAssertion(t, facts, source, tc.want, factflow.ValueSourceExpression)
		})
	}
	for _, point := range built.Graph.RPO() {
		if _, ok := facts.BranchRefinement(point); ok {
			t.Fatalf("parsed source cast emitted branch refinement at point %d", point)
		}
	}
}

func TestLowerClaimConditionsDoNotCreateBranchRefinements(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
if x as number then end
if x :: number then end
`)

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	cases := []struct {
		name   string
		index  int
		syntax ast.CastSyntax
	}{
		{name: "as condition", index: 1, syntax: ast.CastSyntaxAs},
		{name: "colon condition", index: 2, syntax: ast.CastSyntaxColonColon},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt := mustIfStmt(t, stmts, tc.index)
			point := requireStmtPoints(t, built, stmt, 1)[0]
			if _, ok := facts.BranchRefinement(point); ok {
				t.Fatalf("%s emitted branch refinement at point %d", tc.name, point)
			}

			branch, ok := result.BranchCondition(point)
			if !ok {
				t.Fatalf("missing branch condition at point %d", point)
			}
			cast, ok := branch.Source.Expr.(*ast.CastExpr)
			if !ok {
				t.Fatalf("branch source expr = %T, want *ast.CastExpr", branch.Source.Expr)
			}
			if cast.Syntax != tc.syntax {
				t.Fatalf("cast syntax = %v, want %v", cast.Syntax, tc.syntax)
			}

			branchLowerer := lowerer{exprs: make(map[any]factflow.ExprRef)}
			branchInput := factflow.FactsInput{ValueOverlays: make(map[factflow.ExprRef]factflow.ValueOverlay)}
			branchLowerer.addAssertionOverlaysForSource(&branchInput, branch.Source)
			branchFacts := factflow.NewFacts(branchInput)
			branchSource := branchLowerer.valueSource(branch.Source)
			assertLoweredAssertion(t, branchFacts, branchSource, assertion.Type(), factflow.ValueSourceExpression)
		})
	}
}

func TestLowerParsedAnyClaimCastsDoNotEraseRuntimeAxes(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
local a, b = x as any, x :: any
`)

	reg := product.DefaultRegistry()
	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 2)
	base := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	inputValues := make(map[factflow.ExprRef]product.Value)
	for _, point := range points {
		source := mustLocalSource(t, facts, point)
		overlay, ok := facts.ValueOverlay(source.ExprRef)
		if !ok {
			t.Fatalf("missing any claim overlay for source ref %d", source.ExprRef)
		}
		assertAssertionOnlyProduct(t, overlay.Overlay(), assertion.Any())
		inputValues[overlay.Source().ExprRef] = base
	}

	apply := factflow.NewFactsNodeTransfer(factflow.FactsNodeTransferConfig{
		Facts: facts,
		Sources: factflow.NewSourceValues(factflow.SourceValuesConfig{
			Registry:         reg,
			ExpressionValues: inputValues,
		}),
	})
	for _, point := range points {
		out := apply(transfer.NodeContext{Registry: reg, Point: point}, state.State{})
		fact, ok := facts.LocalAssignment(point)
		if !ok {
			t.Fatalf("missing local assignment at point %d", point)
		}
		assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
		want := product.Set(reg, base, assertion.Key, assertion.Any())
		if !product.Equal(reg, assigned, want) {
			t.Fatalf("assigned value changed axes other than assertion.Any at point %d", point)
		}
		if got := product.Get(reg, assigned, assertion.Key); !assertion.Equal(got, assertion.Any()) {
			t.Fatalf("assigned assertion = %s, want any", got)
		}
		if got := product.PresenceOf(assigned); !presence.Equal(got, presence.Present()) {
			t.Fatalf("assigned presence = %s, want present", got)
		}
		if got := product.Get(reg, assigned, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.Table)) {
			t.Fatalf("assigned runtime kind = %s, want table", got)
		}
		if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, evidence.Top()) {
			t.Fatalf("assigned evidence = %s, want top", got)
		}
	}
}

func TestExtractedCastValueSourcesPreserveParsedSyntax(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `
local x = 0
local a, b = x as number, x :: any
`)

	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 2)
	cases := []struct {
		name   string
		point  cfg.Point
		syntax ast.CastSyntax
	}{
		{name: "as", point: points[0], syntax: ast.CastSyntaxAs},
		{name: "colon", point: points[1], syntax: ast.CastSyntaxColonColon},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fact, ok := result.LocalAssignment(tc.point)
			if !ok {
				t.Fatalf("missing local assignment at point %d", tc.point)
			}
			cast, ok := fact.Source.Expr.(*ast.CastExpr)
			if !ok {
				t.Fatalf("semantic source expr = %T, want *ast.CastExpr", fact.Source.Expr)
			}
			if cast.Syntax != tc.syntax {
				t.Fatalf("cast syntax = %v, want %v", cast.Syntax, tc.syntax)
			}
		})
	}
}

func TestLowerNestedClaimsPreserveOuterIdentityAndInnerFlow(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	read := ident("x")
	nonNil := &ast.NonNilAssertExpr{Expr: read}
	cast := &ast.CastExpr{Expr: nonNil, Type: primitiveType("number")}
	local := localAssign([]string{"a"}, cast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	source := mustLocalSource(t, facts, requireStmtPoints(t, built, local, 1)[0])
	outer, ok := facts.ValueOverlay(source.ExprRef)
	if !ok {
		t.Fatalf("missing outer assertion for source %#v", source)
	}
	if got := overlayAssertion(t, outer); !assertion.Equal(got, assertion.Type()) {
		t.Fatalf("outer assertion = %s, want type", got)
	}
	innerSource := outer.Source()
	if innerSource.ExprRef == source.ExprRef || innerSource.ExprRef == 0 {
		t.Fatalf("outer assertion did not point at distinct inner expr ref: outer=%#v inner=%#v", source, innerSource)
	}
	inner, ok := facts.ValueOverlay(innerSource.ExprRef)
	if !ok {
		t.Fatalf("missing inner non-nil claim for source %#v", innerSource)
	}
	if got := overlayAssertion(t, inner); !assertion.Equal(got, assertion.NonNil()) {
		t.Fatalf("inner assertion = %s, want non-nil", got)
	}
}

func TestLowerClaimOverlaysApplyIndicatorsWithoutMutatingBaseValues(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeRead := ident("x")
	anyRead := ident("x")
	nonNilRead := ident("x")
	typeCast := &ast.CastExpr{Expr: typeRead, Type: primitiveType("number")}
	anyCast := &ast.CastExpr{Expr: anyRead, Type: primitiveType("any")}
	nonNil := &ast.NonNilAssertExpr{Expr: nonNilRead}
	local := localAssign([]string{"a", "b", "c"}, typeCast, anyCast, nonNil)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	reg, err := product.DefaultRegistryWithAxes(testLowerSparseAxisSpec().Erase())
	if err != nil {
		t.Fatalf("DefaultRegistryWithAxes error = %v", err)
	}
	facts := lowerFacts(t, result, built.Graph, reg)
	points := requireStmtPoints(t, built, local, 3)
	inputValues := make(map[factflow.ExprRef]product.Value)
	type sourceCase struct {
		name              string
		point             cfg.Point
		base              product.Value
		wantClaim         assertion.Value
		wantPresence      presence.Value
		wantRuntimeKind   runtimekind.Value
		checkRuntimeKind  bool
		checkNoRefinement bool
		checkNoProof      bool
	}
	cases := []sourceCase{
		{
			name:         "type",
			point:        points[0],
			base:         product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), testLowerSparseAxisKey, testLowerSparseAxisLow),
			wantClaim:    assertion.Type(),
			wantPresence: presence.Present(),
		},
		{
			name:              "any",
			point:             points[1],
			base:              product.Set(reg, product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), testLowerSparseAxisKey, testLowerSparseAxisLow), runtimekind.Key, runtimekind.Singleton(runtimekind.Table)),
			wantClaim:         assertion.Any(),
			wantPresence:      presence.Present(),
			wantRuntimeKind:   runtimekind.Singleton(runtimekind.Table),
			checkRuntimeKind:  true,
			checkNoRefinement: true,
			checkNoProof:      true,
		},
		{
			name:         "non-nil",
			point:        points[2],
			base:         product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Absent()), testLowerSparseAxisKey, testLowerSparseAxisLow),
			wantClaim:    assertion.NonNil(),
			wantPresence: presence.Absent(),
		},
	}
	for i := range cases {
		source := mustLocalSource(t, facts, cases[i].point)
		overlay, ok := facts.ValueOverlay(source.ExprRef)
		if !ok {
			t.Fatalf("%s overlay missing", cases[i].name)
		}
		inputValues[overlay.Source().ExprRef] = cases[i].base
	}

	apply := factflow.NewFactsNodeTransfer(factflow.FactsNodeTransferConfig{
		Facts: facts,
		Sources: factflow.NewSourceValues(factflow.SourceValuesConfig{
			Registry:         reg,
			ExpressionValues: inputValues,
		}),
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.checkRuntimeKind {
				if got := product.PresenceOf(tc.base); !presence.Equal(got, tc.wantPresence) {
					t.Fatalf("base input presence = %s, want %s", got, tc.wantPresence)
				}
				if got := product.Get(reg, tc.base, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("base input runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			out := apply(transfer.NodeContext{Registry: reg, Point: tc.point}, state.State{})
			fact, ok := facts.LocalAssignment(tc.point)
			if !ok {
				t.Fatalf("missing local assignment at point %d", tc.point)
			}
			assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
			if got := product.Get(reg, assigned, assertion.Key); !assertion.Equal(got, tc.wantClaim) {
				t.Fatalf("assigned assertion = %s, want %s", got, tc.wantClaim)
			}
			if got := product.PresenceOf(assigned); !presence.Equal(got, tc.wantPresence) {
				t.Fatalf("assigned presence = %s, want %s", got, tc.wantPresence)
			}
			if tc.checkRuntimeKind {
				if got := product.Get(reg, assigned, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("assigned runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			if got := product.Get(reg, assigned, testLowerSparseAxisKey); got != testLowerSparseAxisLow {
				t.Fatalf("assigned sparse axis = %v, want %v", got, testLowerSparseAxisLow)
			}
			if tc.checkNoProof {
				if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, evidence.Top()) {
					t.Fatalf("assigned evidence = %s, want top", got)
				}
			}
			if tc.checkNoRefinement {
				if _, ok := facts.BranchRefinement(tc.point); ok {
					t.Fatalf("%s assignment produced branch refinement", tc.name)
				}
			}
			if got := product.Get(reg, tc.base, assertion.Key); !assertion.Equal(got, assertion.Top()) {
				t.Fatalf("base value mutated with assertion = %s", got)
			}
			if tc.checkRuntimeKind {
				if got := product.Get(reg, tc.base, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("base runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			if got := product.Get(reg, tc.base, testLowerSparseAxisKey); got != testLowerSparseAxisLow {
				t.Fatalf("base sparse axis = %v, want %v", got, testLowerSparseAxisLow)
			}
		})
	}
}

func TestLowerNestedClaimOverlaysApplyCombinedIndicators(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	read := ident("x")
	nonNil := &ast.NonNilAssertExpr{Expr: read}
	cast := &ast.CastExpr{Expr: nonNil, Type: primitiveType("number")}
	local := localAssign([]string{"a"}, cast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	reg := product.DefaultRegistry()
	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, local, 1)[0]
	source := mustLocalSource(t, facts, point)
	outer, ok := facts.ValueOverlay(source.ExprRef)
	if !ok {
		t.Fatalf("missing outer assertion overlay")
	}
	inner, ok := facts.ValueOverlay(outer.Source().ExprRef)
	if !ok {
		t.Fatalf("missing inner assertion overlay")
	}
	base := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	apply := factflow.NewFactsNodeTransfer(factflow.FactsNodeTransferConfig{
		Facts: facts,
		Sources: factflow.NewSourceValues(factflow.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				inner.Source().ExprRef: base,
			},
		}),
	})

	out := apply(transfer.NodeContext{Registry: reg, Point: point}, state.State{})
	fact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment at point %d", point)
	}
	assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
	got := product.Get(reg, assigned, assertion.Key)
	if !got.Has(assertion.TypeClaim) || !got.Has(assertion.NonNilClaim) {
		t.Fatalf("nested assertion = %s, want type and non-nil indicators", got)
	}
	if got := product.Get(reg, base, assertion.Key); !assertion.Equal(got, assertion.Top()) {
		t.Fatalf("base value mutated with assertion = %s", got)
	}
}

func TestLowerClaimWrappedCallPreservesProducerAndClaim(t *testing.T) {
	fooIdent := ident("foo")
	fooCall := &ast.FuncCallExpr{Func: fooIdent}
	fooCast := &ast.CastExpr{Expr: fooCall, Type: primitiveType("number")}
	local := localAssign([]string{"x"}, fooCast)
	barCall := &ast.FuncCallExpr{Func: ident("bar")}
	barCast := &ast.CastExpr{Expr: barCall, Type: primitiveType("string")}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{barCast}}
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	readyCast := &ast.CastExpr{Expr: readyCall, Type: primitiveType("boolean")}
	ifStmt := &ast.IfStmt{Condition: readyCast}
	stmts := []ast.Stmt{local, ifStmt, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"foo", "bar", "ready"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatal("BuildChunk returned nil")
	}
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	localPoints := requireStmtPoints(t, built, local, 2)
	producer, ok := facts.Call(localPoints[0])
	if !ok {
		t.Fatal("missing assertion-wrapped assignment call producer")
	}
	innerRef, ok := producer.Expr()
	if !ok || innerRef == 0 {
		t.Fatalf("inner producer expr ref = %d/%v", innerRef, ok)
	}
	localSource := mustLocalSource(t, facts, localPoints[1])
	if localSource.Kind != factflow.ValueSourceCall || localSource.ExprRef == innerRef || localSource.CallPoint != localPoints[0] || !localSource.HasCallPoint {
		t.Fatalf("local wrapped call source = %#v, inner ref %d", localSource, innerRef)
	}
	claim, ok := facts.ValueOverlay(localSource.ExprRef)
	if !ok {
		t.Fatalf("missing assertion sidecar for outer ref %d", localSource.ExprRef)
	}
	if got := overlayAssertion(t, claim); !assertion.Equal(got, assertion.Type()) {
		t.Fatalf("outer assertion = %s, want type", got)
	}
	innerSource := claim.Source()
	if innerSource.Kind != factflow.ValueSourceCall || innerSource.ExprRef != innerRef || innerSource.CallPoint != localPoints[0] || !innerSource.HasCallPoint {
		t.Fatalf("assertion inner source = %#v, want call source ref %d at point %d", innerSource, innerRef, localPoints[0])
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	returnFact, ok := facts.Return(returnPoints[1])
	if !ok {
		t.Fatal("missing wrapped return fact")
	}
	returnSources := returnFact.Sources()
	if len(returnSources) != 1 || returnSources[0].Kind != factflow.ValueSourceCall || returnSources[0].CallPoint != returnPoints[0] || !returnSources[0].HasCallPoint {
		t.Fatalf("wrapped return source = %#v", returnSources)
	}
	assertLoweredAssertion(t, facts, returnSources[0], assertion.Type(), factflow.ValueSourceCall)

	ifPoints := requireStmtPoints(t, built, ifStmt, 2)
	branch, ok := result.BranchCondition(ifPoints[1])
	if !ok {
		t.Fatal("missing wrapped condition branch fact")
	}
	branchLowerer := lowerer{exprs: make(map[any]factflow.ExprRef)}
	branchInput := factflow.FactsInput{ValueOverlays: make(map[factflow.ExprRef]factflow.ValueOverlay)}
	branchLowerer.addAssertionOverlaysForSource(&branchInput, branch.Source)
	branchFacts := factflow.NewFacts(branchInput)
	branchSource := branchLowerer.valueSource(branch.Source)
	if branchSource.Kind != factflow.ValueSourceCall || branchSource.CallPoint != ifPoints[0] || !branchSource.HasCallPoint {
		t.Fatalf("wrapped condition source = %#v", branchSource)
	}
	assertLoweredAssertion(t, branchFacts, branchSource, assertion.Type(), factflow.ValueSourceCall)
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

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	points := requireStmtPoints(t, built, write, 2)
	producer, ok := facts.Call(points[0])
	if !ok {
		t.Fatalf("missing assignment call producer at point %d", points[0])
	}
	if producer.Context() != factflow.CallProducerContextAssignment {
		t.Fatalf("producer context = %v, want assignment", producer.Context())
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
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetOrdinaryAssignment || targets[0].Index() != 0 {
		t.Fatalf("call-site targets = %#v", targets)
	}
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, targetRoot), "t").Field("x")
	if !targets[0].TargetPath().Equal(wantPath) {
		t.Fatalf("call-site target path = %v, want %v", targets[0].TargetPath(), wantPath)
	}
}

func mustLocalSource(t *testing.T, facts factflow.Facts, point cfg.Point) factflow.ValueSource {
	t.Helper()
	fact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment at point %d", point)
	}
	source := fact.Source()
	if !source.HasExpr || source.ExprRef == 0 {
		t.Fatalf("local source = %#v, want expr ref", source)
	}
	return source
}

func assertLoweredAssertion(t *testing.T, facts factflow.Facts, source factflow.ValueSource, want assertion.Value, wantInnerKind factflow.ValueSourceKind) {
	t.Helper()
	claim, ok := facts.ValueOverlay(source.ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", source.ExprRef)
	}
	assertAssertionOnlyProduct(t, claim.Overlay(), want)
	inner := claim.Source()
	if inner.ExprRef == 0 || inner.ExprRef == source.ExprRef || inner.Kind != wantInnerKind {
		t.Fatalf("assertion inner source = %#v, outer %#v", inner, source)
	}
}

func overlayAssertion(t *testing.T, overlay factflow.ValueOverlay) assertion.Value {
	t.Helper()
	return product.Get(product.DefaultRegistry(), overlay.Overlay(), assertion.Key)
}

func assertAssertionOnlyProduct(t *testing.T, value product.Value, want assertion.Value) {
	t.Helper()
	reg := product.DefaultRegistry()
	if got := product.Get(reg, value, assertion.Key); !assertion.Equal(got, want) {
		t.Fatalf("assertion value = %s, want %s", got, want)
	}
	if got := product.ShapeOf(value); got != product.ShapeTop {
		t.Fatalf("assertion overlay shape = %s, want top", got)
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Top()) {
		t.Fatalf("assertion overlay presence = %s, want top", got)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Top()) {
		t.Fatalf("assertion overlay runtime kind = %s, want top", got)
	}
	if got := product.Get(reg, value, evidence.Key); !evidence.Equal(got, evidence.Top()) {
		t.Fatalf("assertion overlay evidence = %s, want top", got)
	}
	if !product.Equal(reg, value, product.Set(reg, product.Top(), assertion.Key, want)) {
		t.Fatalf("assertion overlay carried non-assertion axes")
	}
}

func assertLoweredBranchValuePresence(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantTrue presence.Value,
	hasTrue bool,
	wantFalse presence.Value,
	hasFalse bool,
) {
	t.Helper()
	refinement, ok := facts.BranchRefinement(point)
	if !ok {
		t.Fatalf("missing branch refinement at point %d", point)
	}
	if !refinement.TargetPath().Equal(wantPath) {
		t.Fatalf("branch target path = %#v, want %#v", refinement.TargetPath(), wantPath)
	}
	assertOptionalValuePresence(t, "true edge", refinement.TrueValue, wantTrue, hasTrue)
	assertOptionalValuePresence(t, "false edge", refinement.FalseValue, wantFalse, hasFalse)
}

func assertOptionalValuePresence(
	t *testing.T,
	label string,
	gotFn func() (factflow.ValueRefinement, bool),
	want presence.Value,
	wantOK bool,
) {
	t.Helper()
	got, ok := gotFn()
	if ok != wantOK {
		t.Fatalf("%s value refinement ok = %v, want %v", label, ok, wantOK)
	}
	if !ok {
		return
	}
	constraint, hasConstraint := got.Constraint()
	if !hasConstraint {
		t.Fatalf("%s constraint missing", label)
	}
	gotPresence := product.PresenceOf(constraint)
	if !presence.Equal(gotPresence, want) {
		t.Fatalf("%s presence = %s, want %s", label, gotPresence, want)
	}
}

type valueRefinementExpectation struct {
	presence    presence.Value
	hasPresence bool

	runtimeKind    runtimekind.Value
	hasRuntimeKind bool
}

func assertLoweredBranchValueRefinement(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantTrue valueRefinementExpectation,
	wantFalse valueRefinementExpectation,
) {
	t.Helper()
	refinement, ok := facts.BranchRefinement(point)
	if !ok {
		t.Fatalf("missing branch refinement at point %d", point)
	}
	if !refinement.TargetPath().Equal(wantPath) {
		t.Fatalf("branch target path = %#v, want %#v", refinement.TargetPath(), wantPath)
	}
	trueValue, ok := refinement.TrueValue()
	if !ok {
		t.Fatalf("missing true-edge value refinement")
	}
	falseValue, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge value refinement")
	}
	assertValueRefinement(t, "true edge", trueValue, wantTrue)
	assertValueRefinement(t, "false edge", falseValue, wantFalse)
}

func assertValueRefinement(t *testing.T, label string, got factflow.ValueRefinement, want valueRefinementExpectation) {
	t.Helper()
	constraint, hasConstraint := got.Constraint()
	if !hasConstraint {
		t.Fatalf("%s constraint missing", label)
	}
	gotPresence := product.PresenceOf(constraint)
	hasPresence := !presence.Equal(gotPresence, presence.Top())
	if hasPresence != want.hasPresence {
		t.Fatalf("%s presence ok = %v, want %v", label, hasPresence, want.hasPresence)
	}
	if want.hasPresence && !presence.Equal(gotPresence, want.presence) {
		t.Fatalf("%s presence = %s, want %s", label, gotPresence, want.presence)
	}
	gotRuntimeKind := product.Get(product.DefaultRegistry(), constraint, runtimekind.Key)
	hasRuntimeKind := !runtimekind.Equal(gotRuntimeKind, runtimekind.Top())
	if hasRuntimeKind != want.hasRuntimeKind {
		t.Fatalf("%s runtime kind ok = %v, want %v", label, hasRuntimeKind, want.hasRuntimeKind)
	}
	if want.hasRuntimeKind && !runtimekind.Equal(gotRuntimeKind, want.runtimeKind) {
		t.Fatalf("%s runtime kind = %s, want %s", label, gotRuntimeKind, want.runtimeKind)
	}
}

func assertLoweredObjectEntry(t *testing.T, entry factflow.ObjectEntry, wantSuffix path.Path, wantKind factflow.ValueSourceKind) {
	t.Helper()
	if !entry.Suffix().Equal(wantSuffix) {
		t.Fatalf("entry suffix = %#v, want %#v", entry.Suffix(), wantSuffix)
	}
	source := entry.Source()
	if source.Kind != wantKind || !source.HasExpr || source.ExprRef == 0 {
		t.Fatalf("entry source = %#v, want kind %v with expr ref", source, wantKind)
	}
}

type testLowerSparseAxis uint8

const (
	testLowerSparseAxisBottom testLowerSparseAxis = iota
	testLowerSparseAxisLow
	testLowerSparseAxisTop
)

var testLowerSparseAxisKey = axis.NewKey[testLowerSparseAxis]("transferfacts.test.sparse")

func testLowerSparseAxisSpec() axis.Spec[testLowerSparseAxis] {
	return axis.Spec[testLowerSparseAxis]{
		Key:    testLowerSparseAxisKey,
		Bottom: func() testLowerSparseAxis { return testLowerSparseAxisBottom },
		Top:    func() testLowerSparseAxis { return testLowerSparseAxisTop },
		Equal:  func(a, b testLowerSparseAxis) bool { return a == b },
		LessOrEq: func(a, b testLowerSparseAxis) bool {
			return a <= b
		},
		Join: func(a, b testLowerSparseAxis) testLowerSparseAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b testLowerSparseAxis) testLowerSparseAxis {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next testLowerSparseAxis) testLowerSparseAxis {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v testLowerSparseAxis) uint64 {
			return uint64(v) + 1
		},
	}
}

func fieldSuffix(name string) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: name}}}
}

func fieldChainSuffix(names ...string) path.Path {
	segments := make([]segment.Segment, len(names))
	for i, name := range names {
		segments[i] = segment.Segment{Kind: segment.SegmentField, Name: name}
	}
	return path.Path{Segments: segments}
}

func stringSuffix(name string) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: name}}}
}

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func number(value string) *ast.NumberExpr {
	return &ast.NumberExpr{Value: value}
}

func stringLit(value string) *ast.StringExpr {
	return &ast.StringExpr{Value: value}
}

func primitiveType(name string) *ast.PrimitiveTypeExpr {
	return &ast.PrimitiveTypeExpr{Name: name}
}

func typeCall(arg ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{arg}}
}

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(name),
		KeySyntax: ast.AttrKeyDot,
	}
}

func stringIndex(obj ast.Expr, key string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(key),
		KeySyntax: ast.AttrKeyIndex,
	}
}

func dynamicIndex(obj ast.Expr, key ast.Expr) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       key,
		KeySyntax: ast.AttrKeyIndex,
	}
}

func localAssign(names []string, exprs ...ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: names, Exprs: exprs}
}

func assign(lhs []ast.Expr, rhs ...ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Rhs: rhs}
}

func parseSemanticChunk(t *testing.T, source string, globals ...string) ([]ast.Stmt, *bind.Result, *cfgbuild.Result, *semantics.Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "transferfacts_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	return stmts, bindings, built, result
}

func mustLocalStmt(t *testing.T, stmts []ast.Stmt, index int) *ast.LocalAssignStmt {
	t.Helper()
	if index < 0 || index >= len(stmts) {
		t.Fatalf("statement index %d out of range for %d statements", index, len(stmts))
	}
	stmt, ok := stmts[index].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("statement %d = %T, want *ast.LocalAssignStmt", index, stmts[index])
	}
	return stmt
}

func mustIfStmt(t *testing.T, stmts []ast.Stmt, index int) *ast.IfStmt {
	t.Helper()
	if index < 0 || index >= len(stmts) {
		t.Fatalf("statement index %d out of range for %d statements", index, len(stmts))
	}
	stmt, ok := stmts[index].(*ast.IfStmt)
	if !ok {
		t.Fatalf("statement %d = %T, want *ast.IfStmt", index, stmts[index])
	}
	return stmt
}

func mustLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	id, ok := bindings.LocalSymbolAt(stmt, index)
	if !ok {
		t.Fatalf("missing local symbol %d for %v", index, stmt.Names)
	}
	return id
}

func mustIdentSymbol(t *testing.T, bindings *bind.Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(ident)
	if !ok {
		t.Fatalf("missing symbol for %q", ident.Value)
	}
	return id
}

func requireStmtPoints(t *testing.T, built *cfgbuild.Result, stmt ast.Stmt, want int) []cfg.Point {
	t.Helper()
	points := built.StmtPoints.PointsFor(stmt)
	if len(points) != want {
		t.Fatalf("points for %T = %v, want %d", stmt, points, want)
	}
	return points
}

func assertNoPointFact(t *testing.T, facts factflow.Facts, point cfg.Point) {
	t.Helper()
	if _, ok := facts.LocalAssignment(point); ok {
		t.Fatalf("point %d lowered as local assignment", point)
	}
	if _, ok := facts.OrdinaryAssignment(point); ok {
		t.Fatalf("point %d lowered as ordinary assignment", point)
	}
	if _, ok := facts.PathAssignment(point); ok {
		t.Fatalf("point %d lowered as path assignment", point)
	}
	if _, ok := facts.BranchRefinement(point); ok {
		t.Fatalf("point %d lowered as branch refinement", point)
	}
	if _, ok := facts.Return(point); ok {
		t.Fatalf("point %d lowered as return", point)
	}
	if _, ok := facts.Call(point); ok {
		t.Fatalf("point %d lowered as call producer", point)
	}
	if _, ok := facts.CallSite(point); ok {
		t.Fatalf("point %d lowered as call site", point)
	}
}

func assertNoCompilerASTTypes(t *testing.T, typ reflect.Type) {
	t.Helper()
	seen := make(map[reflect.Type]struct{})
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if typ == nil {
			return
		}
		if _, ok := seen[typ]; ok {
			return
		}
		seen[typ] = struct{}{}
		if strings.Contains(typ.PkgPath(), "/compiler/ast") {
			t.Fatalf("transfer fact type includes compiler AST type: %v", typ)
		}
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(typ.Elem())
		case reflect.Map:
			walk(typ.Key())
			walk(typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				walk(typ.Field(i).Type)
			}
		}
	}
	walk(typ)
}

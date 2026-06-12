package transferfacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

func TestLowerAnnotatedLiteralLocalCarriesDeclaredValue(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `local x: string | number = 42`)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	points := requireStmtPoints(t, built, mustLocalStmt(t, stmts, 0), 1)
	fact, ok := facts.RootAssignment(points[0])
	if !ok {
		t.Fatalf("missing root assignment at point %d", points[0])
	}
	if got := fact.Kind(); got != factflow.RootAssignmentLocalDeclaration {
		t.Fatalf("root assignment kind = %v, want local declaration", got)
	}
	declared, ok := fact.DeclaredValue()
	if !ok {
		t.Fatalf("missing declared value")
	}
	if got := product.PresenceOf(declared); !presence.Equal(got, presence.Present()) {
		t.Fatalf("declared presence = %s, want present", got)
	}
	wantKind := runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Number))
	if got := product.Get(reg, declared, runtimekind.Key); !runtimekind.Equal(got, wantKind) {
		t.Fatalf("declared runtime kind = %s, want %s", got, wantKind)
	}
}

func TestLowerAnnotatedIdentifierLocalDoesNotCarryDeclaredValue(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `local x: string? = value`, "value")

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	points := requireStmtPoints(t, built, mustLocalStmt(t, stmts, 0), 1)
	fact, ok := facts.LocalAssignment(points[0])
	if !ok {
		t.Fatalf("missing local assignment at point %d", points[0])
	}
	if declared, ok := fact.DeclaredValue(); ok {
		t.Fatalf("unexpected declared value for identifier source: %v", declared)
	}
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	localPoints := requireStmtPoints(t, built, local, 5)
	makeProducer, ok := facts.Call(localPoints[0])
	if !ok {
		t.Fatalf("missing make call producer")
	}
	if makeProducer.CalleeSymbol() == 0 {
		t.Fatalf("make producer callee symbol missing")
	}
	makeTargets := makeProducer.ResultTargets()
	if len(makeTargets) != 1 || makeTargets[0].Kind() != factflow.CallResultTargetLocalAssignment || makeTargets[0].Index() != 0 || makeTargets[0].ResultIndex() != 0 {
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
	if aSource.Kind != factflow.ValueSourceCall || aSource.ExprRef == 0 || !aSource.HasExpr || aSource.ExprIndex != 0 || aSource.TargetIndex != 0 || aSource.ResultIndex != 0 || aSource.CallPoint != localPoints[0] || !aSource.HasCallPoint || !aSource.Adjusted {
		t.Fatalf("local a source = %#v", aSource)
	}

	packProducer, ok := facts.Call(localPoints[1])
	if !ok {
		t.Fatalf("missing pack call producer")
	}
	if packProducer.CalleeSymbol() == 0 {
		t.Fatalf("pack producer callee symbol missing")
	}
	packTargets := packProducer.ResultTargets()
	if len(packTargets) != 2 || packTargets[0].Index() != 1 || packTargets[0].ResultIndex() != 0 || packTargets[1].Index() != 2 || packTargets[1].ResultIndex() != 1 {
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
	if got := bFact.Source(); got.ExprRef == 0 || got.ResultIndex != 0 || !got.Expanded || got.OpenTail {
		t.Fatalf("local b source = %#v", got)
	}
	if got := cFact.Source(); got.ExprRef == 0 || got.ResultIndex != 1 || !got.Expanded || got.OpenTail {
		t.Fatalf("local c source = %#v", got)
	}

	writePoints := requireStmtPoints(t, built, write, 2)
	putProducer, ok := facts.Call(writePoints[0])
	if !ok {
		t.Fatalf("missing put call producer")
	}
	if putProducer.CalleeSymbol() == 0 {
		t.Fatalf("put producer callee symbol missing")
	}
	ordinary, ok := facts.RootAssignment(writePoints[1])
	if !ok {
		t.Fatalf("missing root assignment")
	}
	if got := ordinary.Kind(); got != factflow.RootAssignmentOrdinaryRootWrite {
		t.Fatalf("root assignment kind = %v, want ordinary root write", got)
	}
	if ordinary.TargetSymbol() != mustIdentSymbol(t, bindings, aWrite) {
		t.Fatalf("ordinary target = %d, want %d", ordinary.TargetSymbol(), mustIdentSymbol(t, bindings, aWrite))
	}
	if got := ordinary.Source(); got.Kind != factflow.ValueSourceCall || got.ExprRef == 0 || !got.HasExpr || got.CallPoint != writePoints[0] || !got.HasCallPoint {
		t.Fatalf("ordinary source = %#v", got)
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	tailProducer, ok := facts.Call(returnPoints[0])
	if !ok {
		t.Fatalf("missing tail call producer")
	}
	if tailProducer.CalleeSymbol() == 0 {
		t.Fatalf("tail producer callee symbol missing")
	}
	tailTargets := tailProducer.ResultTargets()
	if len(tailTargets) != 1 || tailTargets[0].Kind() != factflow.CallResultTargetReturn || tailTargets[0].Index() != 1 || tailTargets[0].ResultIndex() != 0 {
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
	if sources[1].ExprRef == 0 || !sources[1].Expanded || !sources[1].OpenTail || sources[1].CallPoint != returnPoints[0] || !sources[1].HasCallPoint {
		t.Fatalf("return tail source = %#v", sources[1])
	}
}

func TestLowerStaticExpressionPathSidecar(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = {}
local a = t.name
local b = t["raw"]
local c = t[1]
local k = "name"
local d = t[k]
`)
	_ = stmts

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})

	assertExprPath := func(source factflow.ValueSource, want path.Path) {
		t.Helper()
		got, ok := facts.ExpressionPath(source.ExprRef)
		if !ok {
			t.Fatalf("missing expression path for ref %d", source.ExprRef)
		}
		if !got.Equal(want) {
			t.Fatalf("expression path = %q, want %q", got.String(), want.String())
		}
	}

	tSym := mustLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	nameSource := mustLocalSource(t, facts, requireStmtPoints(t, built, stmts[1], 1)[0])
	rawSource := mustLocalSource(t, facts, requireStmtPoints(t, built, stmts[2], 1)[0])
	intSource := mustLocalSource(t, facts, requireStmtPoints(t, built, stmts[3], 1)[0])
	dynamicSource := mustLocalSource(t, facts, requireStmtPoints(t, built, stmts[5], 1)[0])
	assertExprPath(nameSource, path.NewPath(tSym, "t").Field("name"))
	assertExprPath(rawSource, path.NewPath(tSym, "t").IndexStr("raw"))
	assertExprPath(intSource, path.NewPath(tSym, "t").IndexInt(1))
	if _, ok := facts.ExpressionPath(dynamicSource.ExprRef); ok {
		t.Fatalf("dynamic index source ref %d unexpectedly has a static expression path", dynamicSource.ExprRef)
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	tSym := mustLocalAt(t, bindings, local, 0)

	dotPoint := requireStmtPoints(t, built, dotWrite, 1)[0]
	dotFact, ok := facts.PathAssignment(dotPoint)
	if !ok {
		t.Fatalf("missing dot path assignment")
	}
	if !dotFact.TargetPath().Equal(path.NewPath(tSym, "t").Field("x")) {
		t.Fatalf("dot path assignment target = %v", dotFact.TargetPath())
	}
	dotStatic, ok := facts.PathStaticMemberWrite(dotPoint)
	if !ok {
		t.Fatalf("missing dot static member write")
	}
	if !dotStatic.TargetPath().Equal(path.NewPath(tSym, "t").Field("x")) {
		t.Fatalf("dot static member write target = %v", dotStatic.TargetPath())
	}
	if dotStatic.Source() != dotFact.Source() {
		t.Fatalf("dot static member write source = %#v, want path assignment source %#v", dotStatic.Source(), dotFact.Source())
	}
	if _, ok := facts.OrdinaryAssignment(dotPoint); ok {
		t.Fatalf("dot path assignment also lowered as root assignment")
	}
	if _, ok := facts.PathDescendantInvalidation(dotPoint); ok {
		t.Fatalf("dot path assignment also lowered as descendant invalidation")
	}

	indexPoint := requireStmtPoints(t, built, indexWrite, 1)[0]
	indexFact, ok := facts.PathAssignment(indexPoint)
	if !ok {
		t.Fatalf("missing static index path assignment")
	}
	if !indexFact.TargetPath().Equal(path.NewPath(tSym, "t").IndexStr("x")) {
		t.Fatalf("static index path assignment target = %v", indexFact.TargetPath())
	}
	indexStatic, ok := facts.PathStaticMemberWrite(indexPoint)
	if !ok {
		t.Fatalf("missing static index member write")
	}
	if !indexStatic.TargetPath().Equal(path.NewPath(tSym, "t").IndexStr("x")) {
		t.Fatalf("static index member write target = %v", indexStatic.TargetPath())
	}
	if indexStatic.Source() != indexFact.Source() {
		t.Fatalf("static index member write source = %#v, want path assignment source %#v", indexStatic.Source(), indexFact.Source())
	}
	if _, ok := facts.PathDescendantInvalidation(indexPoint); ok {
		t.Fatalf("static index path assignment also lowered as descendant invalidation")
	}

	dynamicPoint := requireStmtPoints(t, built, dynamicWrite, 1)[0]
	if _, ok := facts.PathAssignment(dynamicPoint); ok {
		t.Fatalf("dynamic index lowered as path assignment")
	}
	if _, ok := facts.OrdinaryAssignment(dynamicPoint); ok {
		t.Fatalf("dynamic index lowered as ordinary root assignment")
	}
	invalidation, ok := facts.PathDescendantInvalidation(dynamicPoint)
	if !ok {
		t.Fatalf("dynamic index did not lower as descendant invalidation")
	}
	if !invalidation.ContainerPath().Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("dynamic index invalidation container = %v", invalidation.ContainerPath())
	}
	dynamicFact, ok := facts.DynamicIndexWrite(dynamicPoint)
	if !ok {
		t.Fatalf("missing dynamic index write")
	}
	if !dynamicFact.TablePath().Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("dynamic index table path = %v", dynamicFact.TablePath())
	}
	if dynamicFact.Admission() != factflow.DynamicIndexAdmissionUnknown {
		t.Fatalf("dynamic index admission = %v, want unknown", dynamicFact.Admission())
	}
	if dynamicFact.ReadbackIntent() != factflow.DynamicIndexReadbackKeyAndValue {
		t.Fatalf("dynamic index readback = %v, want key and value", dynamicFact.ReadbackIntent())
	}
	if dynamicFact.KeySource().Kind != factflow.ValueSourceExpression || !dynamicFact.KeySource().HasExpr {
		t.Fatalf("dynamic index key source = %#v, want expression source", dynamicFact.KeySource())
	}
	if dynamicFact.Source().Kind != factflow.ValueSourceExpression || !dynamicFact.Source().HasExpr {
		t.Fatalf("dynamic index value source = %#v, want expression source", dynamicFact.Source())
	}
	if _, ok := facts.PathStaticMemberWrite(dynamicPoint); ok {
		t.Fatalf("dynamic index lowered as static member write")
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
	if _, ok := facts.PathDescendantInvalidation(rootPoint); ok {
		t.Fatalf("root ordinary assignment also lowered as descendant invalidation")
	}
}

func TestLowerChannelSelectFacts(t *testing.T) {
	point := cfg.Point(700)
	resultPath := path.NewPath(symbol.ID(701), "result")
	wantCases := []path.Path{
		path.NewPath(symbol.ID(702), "events_ch"),
		path.NewPath(symbol.ID(703), "stop_ch"),
	}
	events := (&lowerer{}).channelSelectEvents(point, semantics.ChannelSelectFact{
		ResultTarget: semantics.CallResultTarget{
			Kind:        semantics.CallResultTargetLocalAssignment,
			Path:        resultPath,
			HasPath:     true,
			ResultIndex: 0,
		},
		Cases: []semantics.ChannelSelectCaseFact{
			{ChannelPath: wantCases[0], HasChannelPath: true},
			{ChannelPath: wantCases[1], HasChannelPath: true},
		},
	})
	if len(events) != 5 {
		t.Fatalf("channel select events = %#v, want select plus two case/receive pairs", events)
	}
	if events[0].Kind() != factflow.ChannelSelectSelect || events[0].SelectID() == "" || events[0].Index() != 0 {
		t.Fatalf("select event = %#v", events[0])
	}
	if got, ok := events[0].ResultPath(); !ok || !got.Equal(resultPath) {
		t.Fatalf("select result path = %v/%v, want %v", got, ok, resultPath)
	}
	for i, wantCase := range wantCases {
		caseEvent := events[1+i*2]
		receiveEvent := events[2+i*2]
		if caseEvent.SelectID() != events[0].SelectID() || receiveEvent.SelectID() != events[0].SelectID() {
			t.Fatalf("case %d select IDs = %q/%q, want %q", i, caseEvent.SelectID(), receiveEvent.SelectID(), events[0].SelectID())
		}
		if caseEvent.Kind() != factflow.ChannelSelectCase || caseEvent.Index() != i {
			t.Fatalf("case event %d = %#v", i, caseEvent)
		}
		if got, ok := caseEvent.CasePath(); !ok || !got.Equal(wantCase) {
			t.Fatalf("case path %d = %v/%v, want %v", i, got, ok, wantCase)
		}
		if receiveEvent.Kind() != factflow.ChannelSelectReceive || receiveEvent.Index() != i {
			t.Fatalf("receive event %d = %#v", i, receiveEvent)
		}
		if got, ok := receiveEvent.ResultPath(); !ok || !got.Equal(resultPath) {
			t.Fatalf("receive result path %d = %v/%v, want %v", i, got, ok, resultPath)
		}
		if got, ok := receiveEvent.CasePath(); !ok || !got.Equal(wantCase) {
			t.Fatalf("receive case path %d = %v/%v, want %v", i, got, ok, wantCase)
		}
	}
}

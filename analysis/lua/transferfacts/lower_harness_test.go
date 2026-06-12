package transferfacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
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

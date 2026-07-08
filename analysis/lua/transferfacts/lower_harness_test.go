package transferfacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerPanicsWithoutRegistry(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, "local x = 1")
	_ = stmts
	_ = bindings
	_ = built

	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Config.Registry is required") {
			t.Fatal("Lower did not panic")
		}
	}()

	_ = Lower(built.Graph, Config{})
}

func TestLowerPanicsWithoutRegistryOnEmptyInputs(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Config.Registry is required") {
			t.Fatal("Lower did not panic")
		}
	}()

	_ = Lower(nil, Config{})
}

func TestLowerAnnotatedLiteralLocalPreservesLiteralValue(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built := parseSemanticChunk(t, `local x: string | number = 42`)

	facts := lowerChunkFactsWithWIR(t, "annotated-literal-local", stmts, built, bindings, reg)
	points := requireStmtPoints(t, built, mustLocalStmt(t, stmts, 0), 1)
	fact, ok := facts.RootAssignment(points[0])
	if !ok {
		t.Fatalf("missing root assignment at point %d", points[0])
	}
	if got := fact.Kind(); got != factflow.RootAssignmentLocalDeclaration {
		t.Fatalf("root assignment kind = %v, want local declaration", got)
	}
	if fact.DeclaredValueContracts() || fact.DeclaredValueOverlays() {
		t.Fatalf("declared contract/overlay = %v/%v, want scalar literal source precision", fact.DeclaredValueContracts(), fact.DeclaredValueOverlays())
	}
	annotation, ok := fact.DeclaredAnnotationValue()
	if !ok {
		t.Fatalf("missing inert declared annotation value")
	}
	annotationType, ok := typevalue.TypeOf(reg, annotation)
	if !ok || !typ.TypeEquals(annotationType, typeexpr.Union(typ.String, typ.Number)) {
		t.Fatalf("annotation type = %v/%v, want string | number", annotationType, ok)
	}
	source := fact.Source()
	if source.Kind != factflow.ValueSourceLiteral || source.LiteralKind != factflow.ValueSourceLiteralInteger || source.Int != 42 {
		t.Fatalf("source = %#v, want integer literal 42", source)
	}
}

func TestLowerAnnotatedEmptyArrayLocalCarriesDeclaredValue(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built := parseSemanticChunk(t, `local xs: any[] = {}`)

	facts := lowerChunkFactsWithWIR(t, "annotated-empty-array-local", stmts, built, bindings, reg)
	points := requireStmtPoints(t, built, mustLocalStmt(t, stmts, 0), 1)
	fact, ok := facts.RootAssignment(points[0])
	if !ok {
		t.Fatalf("missing root assignment at point %d", points[0])
	}
	if got := fact.Kind(); got != factflow.RootAssignmentLocalDeclaration {
		t.Fatalf("root assignment kind = %v, want local declaration", got)
	}
	if !fact.DeclaredValueContracts() {
		t.Fatalf("declared empty array should carry an explicit contract")
	}
	declared, ok := fact.DeclaredValue()
	if !ok {
		t.Fatalf("missing declared value")
	}
	gotType, ok := typevalue.TypeOf(reg, declared)
	if !ok || !typ.TypeEquals(gotType, typ.NewArray(typ.Any)) {
		t.Fatalf("declared type = %v/%v, want any[]", gotType, ok)
	}
}

func TestLowerAnnotatedIdentifierLocalDoesNotCarryDeclaredValue(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built := parseSemanticChunk(t, `local x: string? = value`, "value")

	facts := lowerChunkFactsWithWIR(t, "annotated-identifier-local", stmts, built, bindings, reg)
	points := requireStmtPoints(t, built, mustLocalStmt(t, stmts, 0), 1)
	fact, ok := facts.LocalAssignment(points[0])
	if !ok {
		t.Fatalf("missing local assignment at point %d", points[0])
	}
	if declared, ok := fact.DeclaredValue(); ok {
		t.Fatalf("unexpected declared value for identifier source: %v", declared)
	}
	if annotation, ok := fact.DeclaredAnnotationValue(); !ok {
		t.Fatalf("missing declared annotation value")
	} else if annotationType, typeOK := typevalue.TypeOf(reg, annotation); !typeOK || !typ.TypeEquals(annotationType, typeexpr.Optional(typ.String)) {
		t.Fatalf("annotation type = %v/%v, want string?", annotationType, typeOK)
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
	facts := lowerChunkFactsWithWIR(t, "declaration-control-sidecars", stmts, built, bindings, standard.Registry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	for _, point := range requireStmtPoints(t, built, typeDef, 1) {
		if fact, ok := built.Declarations.TypeDefinition(point); !ok || fact.Kind != cfgbuild.TypeDefinitionAlias {
			t.Fatalf("missing type definition metadata at point %d: %#v/%v", point, fact, ok)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, interfaceDef, 1) {
		if fact, ok := built.Declarations.TypeDefinition(point); !ok || fact.Kind != cfgbuild.TypeDefinitionInterface {
			t.Fatalf("missing interface definition metadata at point %d: %#v/%v", point, fact, ok)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, funcDef, 1) {
		if _, ok := built.Declarations.FunctionDefinition(point); !ok {
			t.Fatalf("missing function definition metadata at point %d", point)
		}
		if fact, ok := facts.RootAssignment(point); !ok || fact.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
			t.Fatalf("missing function definition root assignment at point %d: %#v/%v", point, fact, ok)
		}
	}
	for _, point := range requireStmtPoints(t, built, numericFor, 2) {
		if _, ok := built.NumericFors.Get(point); !ok {
			t.Fatalf("missing numeric-for metadata at point %d", point)
		}
		if _, ok := facts.RootAssignment(point); !ok {
			assertNoPointFact(t, facts, point)
		}
	}
	for _, point := range requireStmtPoints(t, built, genericFor, 2) {
		if _, ok := built.GenericFors.Get(point); !ok {
			t.Fatalf("missing generic-for metadata at point %d", point)
		}
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, label, 1) {
		assertNoPointFact(t, facts, point)
	}
	for _, point := range requireStmtPoints(t, built, gotoStmt, 1) {
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
	body := wirlower.Lower("value-list-metadata", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	localPoints := requireStmtPoints(t, built, local, 5)
	makeProducer, ok := callproducer.FromFacts(facts, localPoints[0])
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

	packProducer, ok := callproducer.FromFacts(facts, localPoints[1])
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
	putProducer, ok := callproducer.FromFacts(facts, writePoints[0])
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
	tailProducer, ok := callproducer.FromFacts(facts, returnPoints[0])
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
	if len(sources) != 2 || sources[0].Kind != factflow.ValueSourcePath || sources[0].HasExpr || sources[1].Kind != factflow.ValueSourceCall {
		t.Fatalf("return sources = %#v", sources)
	}
	if sources[1].HasExpr || !sources[1].Expanded || !sources[1].OpenTail || sources[1].CallPoint != returnPoints[0] || !sources[1].HasCallPoint {
		t.Fatalf("return tail source = %#v", sources[1])
	}
}

func TestLowerStaticExpressionPathSidecar(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local t = {}
local a = t.name
local b = t["raw"]
local c = t[1]
local k = "name"
local d = t[k]
`)
	_ = stmts

	facts := lowerChunkFactsWithWIR(t, "static-expression-path-sidecar", stmts, built, bindings, standard.Registry())

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
	kSym := mustLocalAt(t, bindings, stmts[4].(*ast.LocalAssignStmt), 0)
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
	dynamicExpr, ok := facts.DynamicIndexExpression(dynamicSource.ExprRef)
	if !ok {
		t.Fatalf("missing dynamic index expression for ref %d", dynamicSource.ExprRef)
	}
	if !dynamicExpr.TablePath().Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("dynamic index table path = %v, want t", dynamicExpr.TablePath())
	}
	keySource := dynamicExpr.KeySource()
	if keySource.Kind != factflow.ValueSourceExpression || !keySource.HasExpr {
		t.Fatalf("dynamic index key source = %#v, want expression", keySource)
	}
	assertExprPath(keySource, path.NewPath(kSym, "k"))
}

func TestLowerDynamicIndexExpressionPreservesStaticMemberTablePath(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local term = {}
term.spinner_frames = {"a", "b", "c"}
local i = 1
local frame = term.spinner_frames[i]
`)

	facts := lowerChunkFactsWithWIR(t, "dynamic-index-static-member-table-path", stmts, built, bindings, standard.Registry())
	termSym := mustLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	iSym := mustLocalAt(t, bindings, stmts[2].(*ast.LocalAssignStmt), 0)
	frameSource := mustLocalSource(t, facts, requireStmtPoints(t, built, stmts[3], 1)[0])
	if _, ok := facts.ExpressionPath(frameSource.ExprRef); ok {
		t.Fatalf("dynamic member index source ref %d unexpectedly has a static expression path", frameSource.ExprRef)
	}
	dynamicExpr, ok := facts.DynamicIndexExpression(frameSource.ExprRef)
	if !ok {
		t.Fatalf("missing dynamic member index expression for ref %d", frameSource.ExprRef)
	}
	wantTable := path.NewPath(termSym, "term").Field("spinner_frames")
	if !dynamicExpr.TablePath().Equal(wantTable) {
		t.Fatalf("dynamic member index table path = %v, want %v", dynamicExpr.TablePath(), wantTable)
	}
	keySource := dynamicExpr.KeySource()
	if keySource.Kind != factflow.ValueSourceExpression || !keySource.HasExpr {
		t.Fatalf("dynamic member index key source = %#v, want expression", keySource)
	}
	gotKey, ok := facts.ExpressionPath(keySource.ExprRef)
	if !ok {
		t.Fatalf("missing dynamic member index key path for ref %d", keySource.ExprRef)
	}
	if wantKey := path.NewPath(iSym, "i"); !gotKey.Equal(wantKey) {
		t.Fatalf("dynamic member index key path = %v, want %v", gotKey, wantKey)
	}
}

func TestLowerDynamicIndexExpressionCarriesUnnameableTableSource(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local function make()
    return {["root"] = {id = "root"}}
end
local root = make()["root"]
`)

	body := wirlower.Lower("dynamic-index-unnameable-table-source", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	var rootSource factflow.ValueSource
	for _, point := range built.StmtPoints.PointsFor(stmts[1]) {
		fact, ok := facts.LocalAssignment(point)
		if !ok {
			continue
		}
		source := fact.Source()
		if _, ok := facts.DynamicIndexExpression(source.ExprRef); ok {
			rootSource = source
			break
		}
	}
	if rootSource.ExprRef == 0 {
		t.Fatalf("missing dynamic-index local source for call-result assignment")
	}
	if _, ok := facts.ExpressionPath(rootSource.ExprRef); ok {
		t.Fatalf("dynamic call-result index source ref %d unexpectedly has a static expression path", rootSource.ExprRef)
	}
	dynamicExpr, ok := facts.DynamicIndexExpression(rootSource.ExprRef)
	if !ok {
		t.Fatalf("missing dynamic call-result index expression for ref %d", rootSource.ExprRef)
	}
	if !dynamicExpr.TablePath().IsEmpty() {
		t.Fatalf("dynamic call-result table path = %v, want empty", dynamicExpr.TablePath())
	}
	tableSource, ok := dynamicExpr.TableSource()
	if !ok || tableSource.Kind != factflow.ValueSourceCall || tableSource.CallPoint == 0 || !tableSource.HasCallPoint {
		t.Fatalf("dynamic call-result table source = %#v, want call source", tableSource)
	}
	keySource := dynamicExpr.KeySource()
	if keySource.Kind != factflow.ValueSourceLiteral ||
		keySource.LiteralKind != factflow.ValueSourceLiteralString ||
		keySource.String != "root" ||
		keySource.HasExpr {
		t.Fatalf("dynamic call-result key source = %#v, want WIR string literal", keySource)
	}
}

func TestLowerDotAfterDynamicIndexExpressionCarriesTableSource(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local items = {}
local k = 1
items[k] = {id = "root"}
local id = items[k].id
`)

	facts := lowerChunkFactsWithWIR(t, "dot-after-dynamic-index-table-source", stmts, built, bindings, standard.Registry())
	idSource := mustLocalSource(t, facts, requireStmtPoints(t, built, stmts[3], 1)[0])
	if _, ok := facts.ExpressionPath(idSource.ExprRef); ok {
		t.Fatalf("dot after dynamic index source ref %d unexpectedly has a static expression path", idSource.ExprRef)
	}
	dotExpr, ok := facts.DynamicIndexExpression(idSource.ExprRef)
	if !ok {
		t.Fatalf("missing dynamic-index expression for dot-after-dynamic source ref %d", idSource.ExprRef)
	}
	if !dotExpr.TablePath().IsEmpty() {
		t.Fatalf("dot-after-dynamic table path = %v, want expression-backed table", dotExpr.TablePath())
	}
	tableSource, ok := dotExpr.TableSource()
	if !ok || tableSource.Kind != factflow.ValueSourceExpression || !tableSource.HasExpr {
		t.Fatalf("dot-after-dynamic table source = %#v, want expression source", tableSource)
	}
	keySource := dotExpr.KeySource()
	if keySource.Kind != factflow.ValueSourceLiteral || keySource.LiteralKind != factflow.ValueSourceLiteralString || keySource.String != "id" {
		t.Fatalf("dot key source = %#v, want literal id source", keySource)
	}
}

func TestLowerOrdinaryAssignmentsSplitsRootAndStaticPathWrites(t *testing.T) {
	local := localAssign([]string{"t", "k", "x"}, number("0"), stringLit("key"), number("0"))
	dotWrite := assign([]ast.Expr{dot(ident("t"), "x")}, number("1"))
	indexWrite := assign([]ast.Expr{stringIndex(ident("t"), "x")}, number("2"))
	dynamicWrite := assign([]ast.Expr{dynamicIndex(ident("t"), ident("k"))}, number("3"))
	nestedDynamicWrite := assign([]ast.Expr{dot(dynamicIndex(ident("t"), ident("k")), "value")}, number("4"))
	rootWrite := assign([]ast.Expr{ident("x")}, number("4"))
	stmts := []ast.Stmt{local, dotWrite, indexWrite, dynamicWrite, nestedDynamicWrite, rootWrite}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	facts := lowerChunkFactsWithWIR(t, "ordinary-assignment-splits-root-path", stmts, built, bindings, standard.Registry())
	tSym := mustLocalAt(t, bindings, local, 0)
	kSym := mustLocalAt(t, bindings, local, 1)

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
	if dynamicFact.Admission() != dynamicindex.AdmissionUnknown {
		t.Fatalf("dynamic index admission = %v, want unknown", dynamicFact.Admission())
	}
	if dynamicFact.ReadbackIntent() != factflow.DynamicIndexReadbackKeyAndValue {
		t.Fatalf("dynamic index readback = %v, want key and value", dynamicFact.ReadbackIntent())
	}
	if dynamicFact.KeySource().Kind != factflow.ValueSourcePath || dynamicFact.KeySource().PathKey == "" {
		t.Fatalf("dynamic index key source = %#v, want path source", dynamicFact.KeySource())
	}
	if got, ok := dynamicFact.KeyPath(); !ok || !got.Equal(path.NewPath(kSym, "k")) {
		t.Fatalf("dynamic index key path = %v/%v, want k", got, ok)
	}
	if dynamicFact.Source().Kind != factflow.ValueSourceLiteral || dynamicFact.Source().LiteralKind != factflow.ValueSourceLiteralInteger || dynamicFact.Source().Int != 3 {
		t.Fatalf("dynamic index value source = %#v, want integer literal 3", dynamicFact.Source())
	}
	if _, ok := facts.PathStaticMemberWrite(dynamicPoint); ok {
		t.Fatalf("dynamic index lowered as static member write")
	}

	nestedDynamicPoint := requireStmtPoints(t, built, nestedDynamicWrite, 1)[0]
	if _, ok := facts.PathAssignment(nestedDynamicPoint); ok {
		t.Fatalf("nested dynamic index lowered as path assignment")
	}
	if _, ok := facts.OrdinaryAssignment(nestedDynamicPoint); ok {
		t.Fatalf("nested dynamic index lowered as ordinary root assignment")
	}
	nestedInvalidation, ok := facts.PathDescendantInvalidation(nestedDynamicPoint)
	if !ok {
		t.Fatalf("nested dynamic index did not lower as descendant invalidation")
	}
	if !nestedInvalidation.ContainerPath().Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("nested dynamic index invalidation container = %v", nestedInvalidation.ContainerPath())
	}
	tablePath, keySource, suffix, ok := nestedInvalidation.DynamicTarget()
	if !ok {
		t.Fatalf("nested dynamic index invalidation missing dynamic target")
	}
	if !tablePath.Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("nested dynamic target table = %v, want t", tablePath)
	}
	if keySource.Kind != factflow.ValueSourcePath || keySource.PathKey == "" {
		t.Fatalf("nested dynamic target key source = %#v, want path source", keySource)
	}
	if len(suffix) != 1 || suffix[0].Kind != segment.SegmentField || suffix[0].Name != "value" {
		t.Fatalf("nested dynamic target suffix = %#v, want .value", suffix)
	}
	nestedWrite, ok := facts.DynamicIndexWrite(nestedDynamicPoint)
	if !ok {
		t.Fatalf("nested dynamic index missing direct dynamic-index write")
	}
	if !nestedWrite.TablePath().Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("nested dynamic write table = %v, want t", nestedWrite.TablePath())
	}
	if nestedWrite.KeySource().Kind != factflow.ValueSourcePath || nestedWrite.KeySource().PathKey == "" {
		t.Fatalf("nested dynamic write key source = %#v, want path source", nestedWrite.KeySource())
	}
	if _, ok := facts.PathStaticMemberWrite(nestedDynamicPoint); ok {
		t.Fatalf("nested dynamic index lowered as static member write")
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

func TestLowerDynamicIndexReadbackUsesAliasPathOwner(t *testing.T) {
	local := localAssign([]string{"t", "k", "v"}, number("0"), stringLit("key"), stringLit("value"))
	concreteCastWrite := assign(
		[]ast.Expr{dynamicIndex(ident("t"), &ast.CastExpr{Expr: ident("k"), Type: primitiveType("string")})},
		&ast.CastExpr{Expr: ident("v"), Type: primitiveType("string")},
	)
	anyCastWrite := assign(
		[]ast.Expr{dynamicIndex(ident("t"), &ast.CastExpr{Expr: ident("k"), Type: primitiveType("any")})},
		&ast.CastExpr{Expr: ident("v"), Type: primitiveType("any")},
	)
	stmts := []ast.Stmt{local, concreteCastWrite, anyCastWrite}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	facts := lowerChunkFactsWithWIR(t, "dynamic-index-readback-alias-owner", stmts, built, bindings, standard.Registry())
	kSym := mustLocalAt(t, bindings, local, 1)
	vSym := mustLocalAt(t, bindings, local, 2)

	concretePoint := requireStmtPoints(t, built, concreteCastWrite, 1)[0]
	concreteFact, ok := facts.DynamicIndexWrite(concretePoint)
	if !ok {
		t.Fatalf("missing concrete-cast dynamic index write")
	}
	if got, ok := concreteFact.KeyPath(); !ok || !got.Equal(path.NewPath(kSym, "k")) {
		t.Fatalf("concrete-cast dynamic key path = %v/%v, want k", got, ok)
	}
	if got, ok := concreteFact.ValuePath(); !ok || !got.Equal(path.NewPath(vSym, "v")) {
		t.Fatalf("concrete-cast dynamic value path = %v/%v, want v", got, ok)
	}

	anyPoint := requireStmtPoints(t, built, anyCastWrite, 1)[0]
	anyFact, ok := facts.DynamicIndexWrite(anyPoint)
	if !ok {
		t.Fatalf("missing any-cast dynamic index write")
	}
	if got, ok := anyFact.KeyPath(); ok {
		t.Fatalf("any-cast dynamic key path = %v/%v, want proof boundary", got, ok)
	}
	if got, ok := anyFact.ValuePath(); ok {
		t.Fatalf("any-cast dynamic value path = %v/%v, want proof boundary", got, ok)
	}
}

func TestLowerGlobalTableFieldAssignmentAlsoWritesCanonicalGlobalRoot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
	}{
		{
			name: "dot",
			source: `
_G.coroutine = {}
coroutine.spawn(function() end)
`,
		},
		{
			name: "static string index",
			source: `
_G["coroutine"] = {}
coroutine.spawn(function() end)
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stmts, bindings, built := parseSemanticChunk(t, tc.source, "_G", "coroutine")
			facts := lowerChunkFactsWithWIR(t, "global-table-field-assignment", stmts, built, bindings, standard.Registry())

			point := requireStmtPoints(t, built, stmts[0], 1)[0]
			rootFact, ok := facts.OrdinaryAssignment(point)
			if !ok {
				t.Fatalf("missing canonical global root assignment")
			}
			coroutineSym, ok := bindings.GlobalSymbol("coroutine")
			if !ok {
				t.Fatalf("missing coroutine global symbol")
			}
			if rootFact.TargetSymbol() != coroutineSym {
				t.Fatalf("root target = %d, want coroutine symbol %d", rootFact.TargetSymbol(), coroutineSym)
			}
			if !rootFact.TargetPath().Equal(path.NewPath(coroutineSym, "coroutine")) {
				t.Fatalf("root target path = %v", rootFact.TargetPath())
			}

			gSym, ok := bindings.GlobalSymbol("_G")
			if !ok {
				t.Fatalf("missing _G global symbol")
			}
			pathFact, ok := facts.PathAssignment(point)
			if !ok {
				t.Fatalf("missing _G member path assignment")
			}
			if !pathFact.TargetPath().Equal(path.NewPath(gSym, "_G").Field("coroutine")) &&
				!pathFact.TargetPath().Equal(path.NewPath(gSym, "_G").IndexStr("coroutine")) {
				t.Fatalf("path assignment target = %v", pathFact.TargetPath())
			}
		})
	}
}

func TestLowerLocalGlobalTableShadowDoesNotWriteCanonicalGlobalRoot(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local _G = {}
_G.coroutine = {}
`, "coroutine")
	facts := lowerChunkFactsWithWIR(t, "local-global-table-shadow", stmts, built, bindings, standard.Registry())

	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	if _, ok := facts.OrdinaryAssignment(point); ok {
		t.Fatalf("local _G member write lowered as canonical global root assignment")
	}
}

func TestLowerChannelSelectsUseWIRFacts(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function handle(events_ch: Channel<{kind: "event", id: string}>, stop_ch: Channel<{kind: "stop", reason: string}>)
	local selected = channel.select { events_ch:case_receive(), stop_ch:case_receive() }
end
`, "channel")
	body := wirlower.Lower("handle", fn.Stmts, bindings, built)
	wirFacts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var wirSelects []factflow.ChannelSelect
	for _, point := range built.Graph.RPO() {
		if events := wirFacts.ChannelSelects(point); len(events) != 0 {
			wirSelects = events
		}
	}
	if len(wirSelects) != 5 {
		t.Fatalf("channel select events = %#v, want select plus two case/receive pairs", wirSelects)
	}
	if wirSelects[0].Kind() != factflow.ChannelSelectSelect || wirSelects[0].HasDefault() {
		t.Fatalf("select event = %#v", wirSelects[0])
	}
	wantPayloads := []typ.Type{
		typetable.NewRecord().Field("kind", typ.LiteralString("event")).Field("id", typ.String).Build(),
		typetable.NewRecord().Field("kind", typ.LiteralString("stop")).Field("reason", typ.String).Build(),
	}
	for i, want := range wantPayloads {
		event := wirSelects[2+i*2]
		payload, ok := event.PayloadValue()
		if !ok {
			t.Fatalf("WIR receive event %d missing payload witness: %#v", i, event)
		}
		witness := product.Get(standard.Registry(), payload, typewitness.Key)
		got, ok := witness.Type()
		if !ok || !typ.TypeEquals(got, want) {
			t.Fatalf("WIR receive event %d payload = %v/%v, want %v", i, got, ok, want)
		}
	}
}

func TestLowerChannelSelectsPublishWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function handle(events_ch: Channel<{kind: "event", id: string}>, stop_ch: Channel<{kind: "stop", reason: string}>)
    local selected = channel.select { events_ch:case_receive(), stop_ch:case_receive() }
end
`, "channel")
	body := wirlower.Lower("channel-select-no-sidecars", fn.Stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var selects []factflow.ChannelSelect
	for _, point := range built.Graph.RPO() {
		if events := facts.ChannelSelects(point); len(events) != 0 {
			selects = events
			break
		}
	}
	if len(selects) != 5 {
		t.Fatalf("WIR no-sidecar channel select events = %#v, want select plus two case/receive pairs", selects)
	}
	if selects[0].Kind() != factflow.ChannelSelectSelect || selects[0].HasDefault() {
		t.Fatalf("select event = %#v", selects[0])
	}
	wantPayloads := []typ.Type{
		typetable.NewRecord().Field("kind", typ.LiteralString("event")).Field("id", typ.String).Build(),
		typetable.NewRecord().Field("kind", typ.LiteralString("stop")).Field("reason", typ.String).Build(),
	}
	for i, want := range wantPayloads {
		event := selects[2+i*2]
		payload, ok := event.PayloadValue()
		if !ok {
			t.Fatalf("WIR no-sidecar receive event %d missing payload witness: %#v", i, event)
		}
		witness := product.Get(standard.Registry(), payload, typewitness.Key)
		got, ok := witness.Type()
		if !ok || !typ.TypeEquals(got, want) {
			t.Fatalf("WIR no-sidecar receive payload %d = %v/%v, want %v", i, got, ok, want)
		}
	}
}

func TestLowerChannelSelectsInWIRModeDoesNotFallbackToSidecar(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function handle(events_ch: Channel<{kind: "event", id: string}>, stop_ch: Channel<{kind: "stop", reason: string}>)
    local selected = channel.select { events_ch:case_receive(), stop_ch:case_receive() }
end
`, "channel")
	body := wirlower.Lower("handle", fn.Stmts, bindings, built)
	lowered := lowerer{
		registry:    standard.Registry(),
		bindings:    bindings,
		symbolTypes: lowerSymbolTypes(bindings, nil, nil),
		wir:         body,
	}

	for _, point := range built.Graph.RPO() {
		events := lowered.channelSelectsFromWIR(point)
		if len(events) == 0 {
			continue
		}
		if events[0].Kind() != factflow.ChannelSelectSelect || len(events) != 5 {
			t.Fatalf("WIR channel select events at %d = %#v", point, events)
		}
		return
	}
	t.Fatal("missing WIR channel select events")
}

func TestLowerChannelSelectsFromWIRWithoutSemanticCallView(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function handle(events_ch: Channel<{kind: "event", id: string}>)
    local selected = nil
end
`, "channel")
	stmt := fn.Stmts[0].(*ast.LocalAssignStmt)
	point := requireStmtPoints(t, built, stmt, 1)[0]
	eventsPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "events_ch")
	selectedPath := path.NewPath(mustLocalAt(t, bindings, stmt, 0), "selected")
	body := wir.NewBody("synthetic-channel-select")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpSelect,
		Point: point,
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(eventsPath))}}),
	})
	body.SetPointRange(point, start, start+1)
	body.SetCallResultTarget(point, wir.CallResultTarget{
		Kind:        wir.CallResultTargetLocalAssignment,
		Index:       0,
		ResultIndex: 2,
		Path:        selectedPath,
	})

	if _, ok := built.Calls.Get(point); ok {
		t.Fatalf("fixture unexpectedly has cfgbuild call view at point %d", point)
	}
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	events := facts.ChannelSelects(point)
	if len(events) != 3 {
		t.Fatalf("WIR channel select events at point %d = %#v, want select/case/receive without cfgbuild call view", point, events)
	}
	if events[0].Kind() != factflow.ChannelSelectSelect {
		t.Fatalf("select event = %#v", events[0])
	}
	if events[0].Index() != 2 {
		t.Fatalf("select event index = %d, want WIR result index 2", events[0].Index())
	}
	if got, ok := events[0].ResultPath(); !ok || !got.Equal(selectedPath) {
		t.Fatalf("select result path = %v/%v, want %v", got, ok, selectedPath)
	}
	if got, ok := events[1].CasePath(); !ok || !got.Equal(eventsPath) {
		t.Fatalf("case path = %v/%v, want %v", got, ok, eventsPath)
	}
	payload, ok := events[2].PayloadValue()
	if !ok {
		t.Fatalf("receive event missing payload witness: %#v", events[2])
	}
	witness := product.Get(standard.Registry(), payload, typewitness.Key)
	got, ok := witness.Type()
	want := typetable.NewRecord().Field("kind", typ.LiteralString("event")).Field("id", typ.String).Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("receive payload = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerChannelSelectsUseWIRCandidateCasePaths(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function consume(events: Channel<{
	kind: "stream",
	router: {
		selected: {
			kind: "route_a",
			ch: Channel<{ kind: "leaf", id: string } | { kind: "control", name: string }>,
		} | {
			kind: "route_b",
			ch: Channel<{ kind: "control", name: string }>,
		},
		fallback: Channel<{ kind: "control", name: string }>,
	},
}>)
	local selected = channel.select {
		events:case_receive(),
	}
	if selected.channel == events then
		local payload = selected.value
		if payload.kind == "stream" then
			local route = payload.router.selected
			if route.kind == "route_a" then
				local routed = channel.select {
					route.ch:case_receive(),
					payload.router.fallback:case_receive(),
				}
			end
		end
	end
end
`, "channel")
	body := wirlower.Lower("consume", fn.Stmts, bindings, built)
	wirFacts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var selectGroups [][]factflow.ChannelSelect
	for _, point := range built.Graph.RPO() {
		wirSelects := wirFacts.ChannelSelects(point)
		if len(wirSelects) != 0 {
			selectGroups = append(selectGroups, wirSelects)
		}
	}
	if len(selectGroups) != 2 {
		t.Fatalf("found %d channel-select points, want outer and narrowed nested selects", len(selectGroups))
	}
	var sawOuter, sawNested bool
	for _, events := range selectGroups {
		switch len(events) {
		case 3:
			sawOuter = true
			if events[0].Kind() != factflow.ChannelSelectSelect || events[0].HasDefault() {
				t.Fatalf("outer select event = %#v", events[0])
			}
			if events[1].Kind() != factflow.ChannelSelectCase || events[2].Kind() != factflow.ChannelSelectReceive {
				t.Fatalf("outer select events = %#v", events)
			}
		case 5:
			sawNested = true
			if events[0].Kind() != factflow.ChannelSelectSelect || events[0].HasDefault() {
				t.Fatalf("nested select event = %#v", events[0])
			}
			for i := 0; i < 2; i++ {
				caseEvent := events[1+i*2]
				receiveEvent := events[2+i*2]
				if caseEvent.Kind() != factflow.ChannelSelectCase || receiveEvent.Kind() != factflow.ChannelSelectReceive {
					t.Fatalf("nested case/receive pair %d = %#v / %#v", i, caseEvent, receiveEvent)
				}
				if _, ok := caseEvent.CasePath(); !ok {
					t.Fatalf("nested case %d missing candidate path: %#v", i, caseEvent)
				}
				if _, ok := receiveEvent.CasePath(); !ok {
					t.Fatalf("nested receive %d missing candidate path: %#v", i, receiveEvent)
				}
			}
		default:
			t.Fatalf("unexpected channel-select event group = %#v", events)
		}
	}
	if !sawOuter || !sawNested {
		t.Fatalf("saw outer=%v nested=%v, want both", sawOuter, sawNested)
	}
}

func TestLowerOrdinaryRootTableConstructorReassignmentKeepsRuntimeValue(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local res: {answer: string} = {answer = "ok"}
res = {}
`)
	facts := lowerChunkFactsWithWIR(t, "ordinary-root-table-reassignment", stmts, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	fact, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatal("ordinary assignment did not lower")
	}
	if fact.DeclaredValueOverlays() {
		t.Fatalf("ordinary reassignment overlays contextual type; fresh table value must survive")
	}
	if _, ok := fact.DeclaredValue(); ok {
		t.Fatalf("ordinary reassignment carries declared value; fresh table value must survive")
	}
}

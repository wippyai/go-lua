package transferfacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

	facts := Lower(result, built.Graph)
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	localPoints := requireStmtPoints(t, built, local, 5)
	makeProducer, ok := facts.Call(localPoints[0])
	if !ok {
		t.Fatalf("missing make call producer")
	}
	if makeProducer.Context() != transfer.CallProducerContextAssignment || makeProducer.ExprIndex() != 0 || makeProducer.Final() || makeProducer.Expanded() || !makeProducer.Adjusted() || makeProducer.OpenTail() {
		t.Fatalf("make producer flags/context are wrong")
	}
	makeRef, ok := makeProducer.Expr()
	if !ok || makeRef == 0 {
		t.Fatalf("make producer expr ref = %d/%v", makeRef, ok)
	}
	makeTargets := makeProducer.ResultTargets()
	if len(makeTargets) != 1 || makeTargets[0].Kind() != transfer.CallResultTargetLocalAssignment || makeTargets[0].Index() != 0 {
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
	if aSource.Kind != transfer.ValueSourceCall || aSource.ExprRef != makeRef || !aSource.HasExpr || aSource.ExprIndex != 0 || aSource.TargetIndex != 0 || aSource.ResultIndex != 0 || aSource.CallPoint != localPoints[0] || !aSource.HasCallPoint || !aSource.Adjusted {
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
	if got := ordinary.Source(); got.Kind != transfer.ValueSourceCall || got.ExprRef != putRef || !got.HasExpr || got.CallPoint != writePoints[0] || !got.HasCallPoint {
		t.Fatalf("ordinary source = %#v, put ref %d", got, putRef)
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	tailProducer, ok := facts.Call(returnPoints[0])
	if !ok {
		t.Fatalf("missing tail call producer")
	}
	if tailProducer.Context() != transfer.CallProducerContextReturn || !tailProducer.Final() || !tailProducer.Expanded() || !tailProducer.OpenTail() {
		t.Fatalf("tail producer flags/context are wrong")
	}
	tailRef, ok := tailProducer.Expr()
	if !ok || tailRef == 0 {
		t.Fatalf("tail producer expr ref = %d/%v", tailRef, ok)
	}
	tailTargets := tailProducer.ResultTargets()
	if len(tailTargets) != 1 || tailTargets[0].Kind() != transfer.CallResultTargetReturn || tailTargets[0].Index() != 1 {
		t.Fatalf("tail targets = %#v", tailTargets)
	}
	returnFact, ok := facts.Return(returnPoints[1])
	if !ok {
		t.Fatalf("missing return fact")
	}
	sources := returnFact.Sources()
	if len(sources) != 2 || sources[0].Kind != transfer.ValueSourceExpression || !sources[0].HasExpr || sources[1].Kind != transfer.ValueSourceCall {
		t.Fatalf("return sources = %#v", sources)
	}
	if sources[1].ExprRef != tailRef || !sources[1].Expanded || !sources[1].OpenTail || sources[1].CallPoint != returnPoints[0] || !sources[1].HasCallPoint {
		t.Fatalf("return tail source = %#v, tail ref %d", sources[1], tailRef)
	}
}

func TestLowerSkipsUnsupportedCallProducerContexts(t *testing.T) {
	printCall := &ast.FuncCallExpr{Func: ident("print")}
	printStmt := &ast.FuncCallStmt{Expr: printCall}
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	ifStmt := &ast.IfStmt{Condition: readyCall}
	iterCall := &ast.FuncCallExpr{Func: ident("iter")}
	genericFor := &ast.GenericForStmt{Names: []string{"item"}, Exprs: []ast.Expr{iterCall}}
	stmts := []ast.Stmt{printStmt, ifStmt, genericFor}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"print", "ready", "iter"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph)
	for _, point := range requireStmtPoints(t, built, printStmt, 1) {
		if _, ok := facts.Call(point); ok {
			t.Fatalf("statement call point %d lowered as call producer", point)
		}
	}
	branchPoints := requireStmtPoints(t, built, ifStmt, 2)
	if _, ok := facts.Call(branchPoints[0]); ok {
		t.Fatalf("condition call point %d lowered as call producer", branchPoints[0])
	}
	genericPoints := requireStmtPoints(t, built, genericFor, 3)
	if _, ok := facts.Call(genericPoints[0]); ok {
		t.Fatalf("iterator call point %d lowered as call producer", genericPoints[0])
	}
}

func TestLowerSkipsMemberOrdinaryAssignmentShapes(t *testing.T) {
	l := lowerer{exprs: make(map[any]transfer.ExprRef)}
	memberFact := semantics.OrdinaryAssignmentFact{
		Target: dot(ident("t"), "field"),
		Source: semantics.ValueSource{
			Kind:      semantics.ValueSourceExpression,
			Expr:      number("1"),
			ExprIndex: 0,
		},
	}
	if _, ok := l.ordinaryAssignment(memberFact); ok {
		t.Fatalf("member ordinary assignment lowered as root assignment")
	}

	targetSym := symbol.ID(99)
	memberTarget := semantics.CallResultTarget{
		Kind:      semantics.CallResultTargetOrdinaryAssignment,
		Index:     0,
		Symbol:    targetSym,
		HasSymbol: true,
		Path:      path.NewPath(targetSym, "t").Field("field"),
		HasPath:   true,
	}
	if _, ok := l.callResultTarget(memberTarget); ok {
		t.Fatalf("member ordinary call target lowered as root target")
	}
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

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(name),
		KeySyntax: ast.AttrKeyDot,
	}
}

func localAssign(names []string, exprs ...ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: names, Exprs: exprs}
}

func assign(lhs []ast.Expr, rhs ...ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Rhs: rhs}
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

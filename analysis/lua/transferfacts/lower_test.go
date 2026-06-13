package transferfacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
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

func TestLowerLiteralExpressionValues(t *testing.T) {
	nilLocal := localAssign([]string{"missing"}, &ast.NilExpr{})
	numberLocal := localAssign([]string{"count"}, number("7"))
	tableLocal := localAssign([]string{"box"}, &ast.TableExpr{})
	stmts := []ast.Stmt{nilLocal, numberLocal, tableLocal}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	nilSource := mustLocalSource(t, facts, requireStmtPoints(t, built, nilLocal, 1)[0])
	numberSource := mustLocalSource(t, facts, requireStmtPoints(t, built, numberLocal, 1)[0])
	tableSource := mustLocalSource(t, facts, requireStmtPoints(t, built, tableLocal, 1)[0])

	assertExpressionValue(t, facts, nilSource.ExprRef, presence.Absent(), runtimekind.Singleton(runtimekind.Nil))
	assertExpressionValue(t, facts, numberSource.ExprRef, presence.Present(), runtimekind.Singleton(runtimekind.Number))
	assertExpressionValue(t, facts, tableSource.ExprRef, presence.Present(), runtimekind.Singleton(runtimekind.Table))
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

func assertExpressionValue(t *testing.T, facts factflow.Facts, ref factflow.ExprRef, wantPresence presence.Value, wantRuntimeKind runtimekind.Value) {
	t.Helper()
	value, ok := facts.ExpressionValue(ref)
	if !ok {
		t.Fatalf("missing expression value for ref %d", ref)
	}
	if got := product.PresenceOf(value); !presence.Equal(got, wantPresence) {
		t.Fatalf("expression value presence = %s, want %s", got, wantPresence)
	}
	if got := product.Get(standard.Registry(), value, runtimekind.Key); !runtimekind.Equal(got, wantRuntimeKind) {
		t.Fatalf("expression value runtime kind = %s, want %s", got, wantRuntimeKind)
	}
}

func assertLoweredAssertion(t *testing.T, facts factflow.Facts, source factflow.ValueSource, want assertion.Value, wantInnerKind factflow.ValueSourceKind) {
	t.Helper()
	claim, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", source.ExprRef)
	}
	assertAssertionOnlyProduct(t, claim.Refinement(), want)
	inner := claim.Source()
	if inner.ExprRef == 0 || inner.ExprRef == source.ExprRef || inner.Kind != wantInnerKind {
		t.Fatalf("assertion inner source = %#v, outer %#v", inner, source)
	}
}

func refinementAssertion(t *testing.T, refinement factflow.ExpressionRefinement) assertion.Value {
	t.Helper()
	return product.Get(standard.Registry(), refinement.Refinement(), assertion.Key)
}

func assertAssertionOnlyProduct(t *testing.T, value product.Value, want assertion.Value) {
	t.Helper()
	reg := standard.Registry()
	if got := product.Get(reg, value, assertion.Key); !assertion.Equal(got, want) {
		t.Fatalf("assertion value = %s, want %s", got, want)
	}
	if got := product.ShapeOf(value); got != product.ShapeTop {
		t.Fatalf("assertion refinement shape = %s, want top", got)
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Top()) {
		t.Fatalf("assertion refinement presence = %s, want top", got)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Top()) {
		t.Fatalf("assertion refinement runtime kind = %s, want top", got)
	}
	if got := product.Get(reg, value, evidence.Key); !evidence.Equal(got, evidence.Top()) {
		t.Fatalf("assertion refinement evidence = %s, want top", got)
	}
	if !product.Equal(reg, value, product.Set(reg, product.Top(), assertion.Key, want)) {
		t.Fatalf("assertion refinement carried non-assertion axes")
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
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), wantPath)
	if !ok {
		t.Fatalf("missing branch refinement at point %d", point)
	}
	assertOptionalValuePresence(t, "true edge", refinement.TrueValue, wantTrue, hasTrue)
	assertOptionalValuePresence(t, "false edge", refinement.FalseValue, wantFalse, hasFalse)
}

func assertLoweredBranchPresenceProof(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantPresence presence.Value,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	for _, proof := range facts.BranchProofs(point) {
		if proof.Kind() != factflow.BranchProofPathPresence || !proof.Path().Equal(wantPath) {
			continue
		}
		gotPresence, ok := proof.Presence()
		if !ok || !presence.Equal(gotPresence, wantPresence) {
			continue
		}
		if proof.ActiveOnEdge(true) != wantTrue || proof.ActiveOnEdge(false) != wantFalse {
			t.Fatalf(
				"branch presence proof active true/false = %v/%v, want %v/%v",
				proof.ActiveOnEdge(true),
				proof.ActiveOnEdge(false),
				wantTrue,
				wantFalse,
			)
		}
		return
	}
	t.Fatalf("missing branch presence proof at point %d for %s presence %s", point, wantPath, wantPresence)
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

func assertLoweredBranchValueRefinement(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantTrue valueRefinementExpectation,
	wantFalse valueRefinementExpectation,
) {
	t.Helper()
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), wantPath)
	if !ok {
		t.Fatalf("missing branch refinement at point %d", point)
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

func assertLoweredBranchPathEquality(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantLeft path.Path,
	wantRight path.Path,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	if wantRight.Less(wantLeft) {
		wantLeft, wantRight = wantRight, wantLeft
	}
	relations := facts.BranchPathRelations(point)
	if len(relations) != 2 {
		t.Fatalf("branch path relations at point %d = %d, want 2", point, len(relations))
	}
	var equality, inequality factflow.BranchPathRelation
	for _, relation := range relations {
		switch relation.Kind() {
		case factflow.BranchPathRelationEqual:
			equality = relation
		case factflow.BranchPathRelationNotEqual:
			inequality = relation
		default:
			t.Fatalf("branch path relation kind = %v, want equality or inequality", relation.Kind())
		}
	}
	if equality.Kind() != factflow.BranchPathRelationEqual {
		t.Fatal("missing equality branch path relation")
	}
	if inequality.Kind() != factflow.BranchPathRelationNotEqual {
		t.Fatal("missing inequality branch path relation")
	}
	for _, relation := range []factflow.BranchPathRelation{equality, inequality} {
		if !relation.LeftPath().Equal(wantLeft) {
			t.Fatalf("branch path relation left = %#v, want %#v", relation.LeftPath(), wantLeft)
		}
		if !relation.RightPath().Equal(wantRight) {
			t.Fatalf("branch path relation right = %#v, want %#v", relation.RightPath(), wantRight)
		}
	}
	if equality.ActiveOnEdge(true) != wantTrue || equality.ActiveOnEdge(false) != wantFalse {
		t.Fatalf("equality relation active true/false = %v/%v, want %v/%v", equality.ActiveOnEdge(true), equality.ActiveOnEdge(false), wantTrue, wantFalse)
	}
	if inequality.ActiveOnEdge(true) != !wantTrue || inequality.ActiveOnEdge(false) != !wantFalse {
		t.Fatalf("inequality relation active true/false = %v/%v, want %v/%v", inequality.ActiveOnEdge(true), inequality.ActiveOnEdge(false), !wantTrue, !wantFalse)
	}
	if len(facts.BranchRefinements(point)) != 0 {
		t.Fatalf("path equality relation at point %d also lowered as branch refinement", point)
	}
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
	gotRuntimeKind := product.Get(standard.Registry(), constraint, runtimekind.Key)
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

type valueRefinementExpectation struct {
	presence    presence.Value
	hasPresence bool

	runtimeKind    runtimekind.Value
	hasRuntimeKind bool
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

func parseSemanticFunction(t *testing.T, source string, globals ...string) (*ast.FunctionExpr, *bind.Result, *cfgbuild.Result, *semantics.Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "transferfacts_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("ParseString returned %d statements, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("statement = %T, want function definition", stmts[0])
	}
	bindings := bind.BindFunction(def.Func, bind.Options{Globals: globals})
	built := cfgbuild.BuildFunction(def.Func, bindings)
	if built == nil {
		t.Fatalf("BuildFunction returned nil")
	}
	result, err := semantics.ExtractFunction(def.Func, bindings, built)
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	return def.Func, bindings, built, result
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
	if _, ok := facts.PathDescendantInvalidation(point); ok {
		t.Fatalf("point %d lowered as path descendant invalidation", point)
	}
	if len(facts.BranchRefinements(point)) != 0 {
		t.Fatalf("point %d lowered as branch refinement", point)
	}
	if relations := facts.BranchPathRelations(point); len(relations) != 0 {
		t.Fatalf("point %d lowered as branch path relation", point)
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

func branchRefinementAt(refinements []factflow.BranchRefinement, wantPath path.Path) (factflow.BranchRefinement, bool) {
	for _, refinement := range refinements {
		if refinement.TargetPath().Equal(wantPath) {
			return refinement, true
		}
	}
	return factflow.BranchRefinement{}, false
}

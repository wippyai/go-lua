package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	xPath := path.NewPath(mustIdentSymbol(t, bindings, nilRead), "x")
	nilPoint := requireStmtPoints(t, built, nilStmt, 1)[0]
	notNilPoint := requireStmtPoints(t, built, notNilStmt, 1)[0]
	truthyPoint := requireStmtPoints(t, built, truthyStmt, 1)[0]
	falsyPoint := requireStmtPoints(t, built, falsyStmt, 1)[0]
	assertLoweredBranchValuePresence(t, facts, nilPoint, xPath, presence.Absent(), true, presence.Present(), true)
	assertLoweredBranchPresenceProof(t, facts, nilPoint, xPath, presence.Absent(), true, false)
	assertLoweredBranchPresenceProof(t, facts, nilPoint, xPath, presence.Present(), false, true)
	assertLoweredBranchValuePresence(t, facts, notNilPoint, xPath, presence.Present(), true, presence.Absent(), true)
	assertLoweredBranchValuePresence(t, facts, truthyPoint, xPath, presence.Present(), true, presence.Absent(), true)
	assertLoweredBranchFalsyAbsent(t, facts, truthyPoint, xPath, false)
	assertLoweredBranchPresenceProof(t, facts, truthyPoint, xPath, presence.Present(), true, false)
	assertLoweredBranchValuePresence(t, facts, falsyPoint, xPath, presence.Bottom(), false, presence.Present(), true)
	assertLoweredBranchPresenceProof(t, facts, falsyPoint, xPath, presence.Present(), false, true)
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

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, rootRead), "t").Field("child")
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, memberStmt, 1)[0], wantPath, presence.Present(), true, presence.Absent(), true)
}

func TestLowerTypeGuardBranchPathEvidence(t *testing.T) {
	decl := localAssign([]string{"x"}, ident("input"))
	typeRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(typeRead),
		Rhs:      stringLit("string"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, typeRead), "x")
	assertLoweredBranchPresenceProof(t, facts, point, xPath, presence.Present(), true, false)
}

func TestLowerLogicalAndBranchPublishesTrueEdgeConjunctRefinements(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local data_func = raw
if data_func and data_func ~= "" then
end
`, "raw")

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	dataFunc := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0)
	ifStmt := mustIfStmt(t, stmts, 1)
	assertLoweredBranchValuePresence(
		t,
		facts,
		requireStmtPoints(t, built, ifStmt, 1)[0],
		path.NewPath(dataFunc, "data_func"),
		presence.Present(), true,
		presence.Bottom(), false,
	)
}

func TestLowerLogicalOrBranchPublishesFalseEdgeDisjunctRefinements(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local page = input
if not page or not page.data_func or page.data_func == "" then
end
`, "input")

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	page := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0)
	ifStmt := mustIfStmt(t, stmts, 1)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	assertLoweredBranchValuePresence(
		t,
		facts,
		point,
		path.NewPath(page, "page"),
		presence.Bottom(), false,
		presence.Present(), true,
	)
	assertLoweredBranchValuePresence(
		t,
		facts,
		point,
		path.NewPath(page, "page").Field("data_func"),
		presence.Bottom(), false,
		presence.Present(), true,
	)
	assertRootRefinementsBeforeDescendants(t, facts.BranchRefinements(point))
}

func TestLowerTypedOptionalMemberBranchPublishesStaticRuntimeKind(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(page: { data_func: string? }?)
	if not page or not page.data_func or page.data_func == "" then
	end
end
`)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	page := bindings.ParamSlots(fn)[0].Symbol
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	target := path.NewPath(page, "page").Field("data_func")
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), target)
	if !ok {
		t.Fatalf("missing branch refinement for %s", target)
	}
	if _, ok := refinement.TrueValue(); ok {
		t.Fatalf("true edge has refinement for %s, want false edge only", target)
	}
	falseValue, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge refinement for %s", target)
	}
	assertValueRefinement(t, "false edge", falseValue, valueRefinementExpectation{
		presence:       presence.Present(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Singleton(runtimekind.String),
		hasRuntimeKind: true,
	})
}

func TestLowerMemberPathBranchRefinementOrdersRootBeforeChild(t *testing.T) {
	template := typetable.NewRecord().
		Field("kind", typ.LiteralString("template")).
		Field("data_func", typeexpr.Optional(typ.String)).
		Build()
	component := typetable.NewRecord().
		Field("kind", typ.LiteralString("component")).
		Field("url", typ.String).
		Build()
	pageType := typeexpr.Union(template, component)
	page := symbol.ID(701)
	rootPath := path.NewPath(page, "page")
	childPath := rootPath.Field("data_func")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{page: pageType},
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind: branchcond.CheckNotNil,
		Path: childPath,
	})
	rootIndex := branchRefinementIndex(refinements, rootPath)
	if rootIndex < 0 {
		t.Fatalf("missing root refinement for %s", rootPath)
	}
	childIndex := branchRefinementIndex(refinements, childPath)
	if childIndex < 0 {
		t.Fatalf("missing child refinement for %s", childPath)
	}
	if rootIndex >= childIndex {
		t.Fatalf("root refinement index = %d, child index = %d; want root first", rootIndex, childIndex)
	}
}

func TestLowerCompoundBranchOrdersAllRootsBeforeDescendants(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(page: { data_func: string?, url: string? } | { other: string })
	if page.data_func and page.url then
	end
end
`)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	page := bindings.ParamSlots(fn)[0].Symbol
	rootPath := path.NewPath(page, "page")
	dataPath := rootPath.Field("data_func")
	urlPath := rootPath.Field("url")
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	refinements := facts.BranchRefinements(requireStmtPoints(t, built, ifStmt, 1)[0])

	firstDescendant := len(refinements)
	for i, refinement := range refinements {
		if len(refinement.TargetPath().Segments) != 0 {
			firstDescendant = i
			break
		}
	}
	if firstDescendant == len(refinements) {
		t.Fatalf("compound branch produced no descendant refinements: %#v", refinements)
	}
	for i := firstDescendant; i < len(refinements); i++ {
		if len(refinements[i].TargetPath().Segments) == 0 {
			t.Fatalf("root refinement at index %d after descendant index %d", i, firstDescendant)
		}
	}
	if branchRefinementIndex(refinements, rootPath) < 0 {
		t.Fatalf("missing root refinement for compound branch")
	}
	if branchRefinementIndex(refinements, dataPath) < 0 {
		t.Fatalf("missing data_func descendant refinement")
	}
	if branchRefinementIndex(refinements, urlPath) < 0 {
		t.Fatalf("missing url descendant refinement")
	}
}

func TestLowerLiteralDiscriminantBranchRefinesRootOnBothEdges(t *testing.T) {
	left := typetable.NewRecord().
		Field("tag", typ.LiteralString("a")).
		Field("value", typ.String).
		Build()
	right := typetable.NewRecord().
		Field("tag", typ.LiteralString("b")).
		Field("value", typ.Number).
		Build()
	rootType := typeexpr.Union(left, right)
	root := symbol.ID(801)
	rootPath := path.NewPath(root, "r")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{root: rootType},
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind:          branchcond.CheckLiteralEqual,
		Path:          rootPath.Field("tag"),
		LiteralString: "a",
	})
	refinement, ok := branchRefinementAt(refinements, rootPath)
	if !ok {
		t.Fatalf("missing root refinement for literal discriminant")
	}
	trueValue, ok := refinement.TrueValue()
	if !ok {
		t.Fatalf("missing true-edge root refinement")
	}
	falseValue, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge root refinement")
	}
	assertVariantOriginRefinementType(t, "true edge", trueValue, left)
	assertVariantOriginRefinementType(t, "false edge", falseValue, right)
}

func TestLowerTruthyInstantiatedResultBranchRefinesRootOnBothEdges(t *testing.T) {
	resultType, valueCase, errorCase := instantiatedResultTypeParamFixture()
	root := symbol.ID(802)
	rootPath := path.NewPath(root, "result")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{root: resultType},
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind: branchcond.CheckTruthy,
		Path: rootPath.Field("ok"),
	})
	rootFamily, _, ok := variant.OriginOfType(resultType)
	if !ok {
		t.Fatal("missing root origin for Result<T>")
	}

	trueValue := requireBranchRefinementValueAt(t, refinements, rootPath, true)
	falseValue := requireBranchRefinementValueAt(t, refinements, rootPath, false)
	assertVariantOriginRefinementType(t, "true edge", trueValue, valueCase)
	assertVariantOriginRefinementType(t, "false edge", falseValue, errorCase)
	assertVariantOriginRefinementFamily(t, "true edge", trueValue, rootFamily)
	assertVariantOriginRefinementFamily(t, "false edge", falseValue, rootFamily)
}

func assertVariantOriginRefinementType(t *testing.T, label string, refinement factflow.ValueRefinement, want typ.Type) {
	t.Helper()
	constraint, ok := refinement.Constraint()
	if !ok {
		t.Fatalf("%s constraint missing", label)
	}
	origin := product.Get(standard.Registry(), constraint, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		t.Fatalf("%s variant origin = %v, want concrete", label, origin)
	}
	got, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases())
	if !ok {
		t.Fatalf("%s origin type unavailable", label)
	}
	if !typ.SameNodeOrAcyclicEqual(got, want) {
		t.Fatalf("%s origin type = %v, want %v", label, got, want)
	}
}

func assertVariantOriginRefinementFamily(t *testing.T, label string, refinement factflow.ValueRefinement, want uint64) {
	t.Helper()
	constraint, ok := refinement.Constraint()
	if !ok {
		t.Fatalf("%s constraint missing", label)
	}
	origin := product.Get(standard.Registry(), constraint, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		t.Fatalf("%s variant origin = %v, want family %d", label, origin, want)
	}
	if origin.Family() != want {
		t.Fatalf("%s origin family = %d, want %d", label, origin.Family(), want)
	}
}

func TestLowerPathEqualityBranchRelation(t *testing.T) {
	decl := localAssign([]string{"a", "b"}, number("1"), number("2"))
	aRead := ident("a")
	bRead := ident("b")
	eqStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "==", Lhs: aRead, Rhs: bRead}}
	stmts := []ast.Stmt{decl, eqStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	point := requireStmtPoints(t, built, eqStmt, 1)[0]
	assertLoweredBranchPathEquality(
		t,
		facts,
		point,
		path.NewPath(mustIdentSymbol(t, bindings, aRead), "a"),
		path.NewPath(mustIdentSymbol(t, bindings, bRead), "b"),
		true,
		false,
	)
}

func branchRefinementIndex(refinements []factflow.BranchRefinement, wantPath path.Path) int {
	for i, refinement := range refinements {
		if refinement.TargetPath().Equal(wantPath) {
			return i
		}
	}
	return -1
}

func requireBranchRefinementValueAt(t *testing.T, refinements []factflow.BranchRefinement, wantPath path.Path, cond bool) factflow.ValueRefinement {
	t.Helper()
	for _, refinement := range refinements {
		if !refinement.TargetPath().Equal(wantPath) {
			continue
		}
		if value, ok := refinement.ValueForEdge(cond); ok {
			return value
		}
	}
	t.Fatalf("missing %t-edge refinement for %s in %#v", cond, wantPath, refinements)
	return factflow.ValueRefinement{}
}

func assertRootRefinementsBeforeDescendants(t *testing.T, refinements []factflow.BranchRefinement) {
	t.Helper()
	seenDescendant := false
	for i, refinement := range refinements {
		if len(refinement.TargetPath().Segments) == 0 {
			if seenDescendant {
				t.Fatalf("root refinement at index %d appears after descendant in %#v", i, refinements)
			}
			continue
		}
		seenDescendant = true
	}
}

func instantiatedResultTypeParamFixture() (typ.Type, typ.Type, typ.Type) {
	tp := typ.NewTypeParam("T", nil)
	valueCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", tp).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typeexpr.Union(valueCase, errorCase))
	return typ.Instantiate(result, tp), valueCase, errorCase
}

func TestLowerPathInequalityBranchRelation(t *testing.T) {
	decl := localAssign([]string{"t", "u"}, &ast.TableExpr{}, &ast.TableExpr{})
	tRead := ident("t")
	uRead := ident("u")
	neqStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      dot(tRead, "left"),
		Rhs:      dot(uRead, "right"),
	}}
	stmts := []ast.Stmt{decl, neqStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	point := requireStmtPoints(t, built, neqStmt, 1)[0]
	assertLoweredBranchPathEquality(
		t,
		facts,
		point,
		path.NewPath(mustIdentSymbol(t, bindings, tRead), "t").Field("left"),
		path.NewPath(mustIdentSymbol(t, bindings, uRead), "u").Field("right"),
		false,
		true,
	)
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
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
	l := lowerer{registry: standard.Registry()}
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	if len(facts.BranchRefinements(point)) != 0 {
		t.Fatalf("unknown type guard branch point %d lowered as branch refinement", point)
	}
}

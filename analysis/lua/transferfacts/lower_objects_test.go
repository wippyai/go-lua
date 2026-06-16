package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
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
	wantID := identity.LuaTableLiteral(built.Graph.ID(), uint64(source.ExprRef))
	if gotID, ok := literal.Identity(); !ok || gotID != wantID {
		t.Fatalf("literal identity = %v/%v, want %v", gotID, ok, wantID)
	}
}

func TestLowerEmptyObjectLiteralStillPublishesIdentitySidecar(t *testing.T) {
	table := &ast.TableExpr{}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	point := requireStmtPoints(t, built, local, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	source := localFact.Source()
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing empty object literal sidecar for ref %d", source.ExprRef)
	}
	if got := len(literal.Entries()); got != 0 {
		t.Fatalf("empty literal entries = %d, want 0", got)
	}
	wantID := identity.LuaTableLiteral(built.Graph.ID(), uint64(source.ExprRef))
	if gotID, ok := literal.Identity(); !ok || gotID != wantID {
		t.Fatalf("literal identity = %v/%v, want %v", gotID, ok, wantID)
	}
}

func TestLowerAnnotatedEmptyMapObjectLiteralCarriesExpectedContract(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `local t: {[string]: string} = {}`)
	reg := standard.Registry()
	facts := lowerFacts(t, result, built.Graph, reg)
	point := requireStmtPoints(t, built, stmts[0], 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	literal, ok := facts.ObjectLiteral(localFact.Source().ExprRef)
	if !ok {
		t.Fatalf("missing empty map object literal sidecar")
	}
	expected, ok := literal.Expected()
	if !ok {
		t.Fatalf("missing expected contract on empty map literal")
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typetable.NewMap(typ.String, typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("expected type = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerReturnedObjectLiteralCarriesExpectedEntryContracts(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function new_actor(): { state: { processed: {[string]: string} } }
	return { state = { processed = {} } }
end`)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	var returnFact factflow.Return
	for _, point := range requireStmtPoints(t, built, ret, 1) {
		if fact, ok := facts.Return(point); ok {
			returnFact = fact
			break
		}
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want one expression source", sources)
	}
	literal, ok := facts.ObjectLiteral(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing returned object literal sidecar for ref %d", sources[0].ExprRef)
	}
	expected, ok := literal.Expected()
	if !ok {
		t.Fatalf("missing expected contract on returned object literal")
	}
	got, ok := typevalue.TypeOf(reg, expected)
	wantRoot := typetable.NewRecord().
		Field("state", typetable.NewRecord().
			Field("processed", typetable.NewMap(typ.String, typ.String)).
			Build()).
		Build()
	if !ok || !typ.TypeEquals(got, wantRoot) {
		t.Fatalf("returned literal expected = %v/%v, want %v", got, ok, wantRoot)
	}
	var found bool
	for _, entry := range literal.Entries() {
		if !reflect.DeepEqual(entry.Suffix(), fieldChainSuffix("state", "processed")) {
			continue
		}
		found = true
		entryExpected, ok := entry.Expected()
		if !ok {
			t.Fatalf("state.processed entry missing expected contract")
		}
		got, ok := typevalue.TypeOf(reg, entryExpected)
		want := typetable.NewMap(typ.String, typ.String)
		if !ok || !typ.TypeEquals(got, want) {
			t.Fatalf("state.processed expected = %v/%v, want %v", got, ok, want)
		}
	}
	if !found {
		t.Fatalf("returned literal entries = %#v, want state.processed entry", literal.Entries())
	}
}

func TestLowerExplicitAnyObjectLiteralDeclarationUsesDeclaredContract(t *testing.T) {
	stmts, _, built, result := parseSemanticChunk(t, `local raw: any = { id = "cfg" }`)
	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	point := requireStmtPoints(t, built, stmts[0], 1)[0]
	fact, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatal("missing root assignment fact")
	}
	if !fact.DeclaredValueContracts() {
		t.Fatalf("root assignment = %#v, want declared contract for explicit any object literal", fact)
	}
	if _, ok := fact.DeclaredValue(); !ok {
		t.Fatalf("root assignment = %#v, want declared value", fact)
	}
}

func TestReachesRecordAcceptsInstantiatedRecord(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param},
		typetable.NewRecord().Field("value", param).Build())

	if !reachesRecord(typ.Instantiate(box, typ.String)) {
		t.Fatal("reachesRecord(instantiated record) = false, want true")
	}
}

func TestLowerObjectLiteralEntryCallSourcePointsAtNestedProducer(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	table := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       stringLit("x"),
		KeySyntax: ast.AttrKeyDot,
		Value:     makeCall,
	}}}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	points := requireStmtPoints(t, built, local, 2)
	localFact, ok := facts.LocalAssignment(points[1])
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	literal, ok := facts.ObjectLiteral(localFact.Source().ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar")
	}
	entries := literal.Entries()
	if len(entries) != 1 {
		t.Fatalf("literal entries = %#v, want one", entries)
	}
	source := entries[0].Source()
	if source.Kind != factflow.ValueSourceCall || source.CallPoint != points[0] || !source.HasCallPoint || source.ResultIndex != 0 {
		t.Fatalf("entry source = %#v, want call point %d", source, points[0])
	}
}

func TestLowerAnyCastObjectLiteralPublishesClaimNotEntries(t *testing.T) {
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	point := requireStmtPoints(t, built, local, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	source := localFact.Source()
	assertLoweredAssertion(t, facts, source, assertion.Any(), factflow.ValueSourceExpression)
	if literal, ok := facts.ObjectLiteral(source.ExprRef); ok {
		t.Fatalf("any-cast object literal sidecar = %#v, want none", literal)
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

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
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
	nestedSource := entries[1].Source()
	nestedLiteral, ok := facts.ObjectLiteral(nestedSource.ExprRef)
	if !ok {
		t.Fatalf("missing nested object literal sidecar for ref %d", nestedSource.ExprRef)
	}
	if got := len(nestedLiteral.Entries()); got != 1 {
		t.Fatalf("nested literal entries = %d, want one static entry", got)
	}
	assertLoweredObjectEntry(t, nestedLiteral.Entries()[0], fieldSuffix("b"), factflow.ValueSourceExpression)
	wantID := identity.LuaTableLiteral(built.Graph.ID(), uint64(nestedSource.ExprRef))
	if gotID, ok := nestedLiteral.Identity(); !ok || gotID != wantID {
		t.Fatalf("nested literal identity = %v/%v, want %v", gotID, ok, wantID)
	}
}

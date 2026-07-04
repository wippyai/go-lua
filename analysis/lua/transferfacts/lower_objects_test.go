package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerObjectLiteralEntryCarriesSyntaxFreeMetadata(t *testing.T) {
	entrySource := sourceprovenance.ASTSource{Kind: sourceprovenance.SourceNil}
	span := semantics.SourceSpan{StartLine: 3, StartCol: 4, EndLine: 3, EndCol: 9}
	l := lowerer{}
	lowered := l.objectLiteral(semantics.ObjectLiteralFact{
		Entries: []semantics.ObjectEntryFact{
			{
				Suffix:     fieldSuffix("id"),
				Source:     entrySource,
				ValueSpan:  span,
				ValueLabel: "payload.id",
			},
		},
	})
	entries := lowered.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one", entries)
	}
	if got := entries[0].ValueSpan(); got.StartLine != 3 || got.EndCol != 9 {
		t.Fatalf("entry span = %#v, want lowered span", got)
	}
	if got := entries[0].ValueLabel(); got != "payload.id" {
		t.Fatalf("entry label = %q, want payload.id", got)
	}
}

func TestLowerObjectLiteralEntryLabelsAttributeValue(t *testing.T) {
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("id"), KeySyntax: ast.AttrKeyDot, Value: objectAttrGet(ident("payload"), "id")},
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
	point := requireStmtPoints(t, built, local, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	literal, ok := facts.ObjectLiteral(localFact.Source().ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar")
	}
	entries := literal.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if got := entries[0].ValueLabel(); got != "payload.id" {
		t.Fatalf("entry label = %q, want payload.id", got)
	}
}

func objectAttrGet(object ast.Expr, field string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    object,
		Key:       stringLit(field),
		KeySyntax: ast.AttrKeyDot,
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

func TestLowerDeclaredReturnAccumulatorCarriesExpectedContract(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, result := parseSemanticFunction(t, `
function make(raw: any): {any}
    local out = {}
    if raw == nil then
        return out
    end
    return out
end
`)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	localStmt, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing accumulator local assignment at point %d", point)
	}
	if !localFact.DeclaredValueContracts() {
		t.Fatalf("accumulator should carry declared return contract")
	}
	declared, ok := localFact.DeclaredValue()
	if !ok {
		t.Fatalf("missing accumulator declared value")
	}
	gotType, ok := typevalue.TypeOf(reg, declared)
	if !ok || !typ.TypeEquals(gotType, typ.NewArray(typ.Any)) {
		t.Fatalf("accumulator declared type = %v/%v, want {any}", gotType, ok)
	}
	literal, ok := facts.ObjectLiteral(localFact.Source().ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar for accumulator")
	}
	expected, ok := literal.Expected()
	if !ok {
		t.Fatalf("accumulator object literal should carry expected return type")
	}
	expectedType, ok := typevalue.TypeOf(reg, expected)
	if !ok || !typ.TypeEquals(expectedType, typ.NewArray(typ.Any)) {
		t.Fatalf("accumulator expected literal type = %v/%v, want {any}", expectedType, ok)
	}
}

func TestLowerDeclaredReturnAccumulatorRejectsMixedReturnSymbols(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, result := parseSemanticFunction(t, `
function make(flag: boolean): {any}
    local out = {}
    local other = {}
    if flag then
        return out
    end
    return other
end
`)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	localStmt, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing root assignment at point %d", point)
	}
	if declared, ok := localFact.DeclaredValue(); ok {
		t.Fatalf("mixed return symbols should not infer accumulator contract: %v", declared)
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

func TestLowerOrdinaryOptionalObjectLiteralExpectedRootIsPresent(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, result := parseSemanticFunction(t, `
function setup(): ()
    local runtime: { apply: (string) -> string }?
    runtime = {
        apply = function(phase: string): string
            return phase
        end,
    }
end
`)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})

	assignStmt, ok := fn.Stmts[1].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want assignment", fn.Stmts[1])
	}
	assignPoint := requireStmtPoints(t, built, assignStmt, 1)[0]
	assignFact, ok := facts.OrdinaryAssignment(assignPoint)
	if !ok {
		t.Fatalf("missing ordinary assignment fact")
	}
	literal, ok := facts.ObjectLiteral(assignFact.Source().ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar for assignment source %#v", assignFact.Source())
	}
	expected, ok := literal.Expected()
	if !ok {
		t.Fatalf("missing expected contract on assigned object literal")
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typetable.NewRecord().
		Field("apply", typ.Func().Param("phase", typ.String).Returns(typ.String).Build()).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("literal expected root = %v/%v, want present %v", got, ok, want)
	}
	if typ.TypeEquals(got, typeexpr.Optional(want)) {
		t.Fatalf("literal expected root stayed optional: %v", got)
	}
}

func TestLowerLogicalOperandObjectLiteralPublishesSidecar(t *testing.T) {
	_, _, built, result := parseSemanticChunk(t, `local value = true and {}`)
	reg := standard.Registry()
	facts := lowerFacts(t, result, built.Graph, reg)

	for _, literal := range facts.ObjectLiterals() {
		if _, ok := literal.Identity(); ok {
			return
		}
	}
	t.Fatalf("object literals = %#v, want sidecar for table constructor nested under logical expression", facts.ObjectLiterals())
}

func TestLowerDynamicIndexLogicalDefaultObjectLiteralCarriesSlotContract(t *testing.T) {
	_, bindings, built, result := parseSemanticChunk(t, `
local suites: {[string]: any[]} = {}
local suite = "alpha"
suites[suite] = suites[suite] or {}
`)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	want := typ.NewArray(typ.Any)

	for _, literal := range facts.ObjectLiterals() {
		expected, ok := literal.Expected()
		if !ok {
			continue
		}
		got, ok := typevalue.TypeOf(reg, expected)
		if ok && typ.TypeEquals(got, want) {
			return
		}
	}
	t.Fatalf("object literals = %#v, want logical default constructor to carry dynamic slot contract %v", facts.ObjectLiterals(), want)
}

func TestLowerAnnotatedLogicalFallbackObjectLiteralCarriesDeclaredContract(t *testing.T) {
	_, bindings, built, result := parseSemanticChunk(t, `
type Payload = {
    tool_calls: {string},
}
local payload: Payload?
local current: Payload = payload or {
    tool_calls = {},
}
`)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	want := typ.NewArray(typ.String)

	for _, literal := range facts.ObjectLiterals() {
		for _, entry := range literal.Entries() {
			if !reflect.DeepEqual(entry.Suffix(), fieldSuffix("tool_calls")) {
				continue
			}
			expected, ok := entry.Expected()
			if !ok {
				t.Fatalf("tool_calls entry missing expected contract")
			}
			got, ok := typevalue.TypeOf(reg, expected)
			if !ok || !typ.TypeEquals(got, want) {
				t.Fatalf("tool_calls expected = %v/%v, want %v", got, ok, want)
			}
			return
		}
	}
	t.Fatalf("object literals = %#v, want logical fallback tool_calls entry contract %v", facts.ObjectLiterals(), want)
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

func TestLowerReturnedNestedObjectLiteralCarriesExpectedEntryContracts(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function output_error(err_type: string, message: string, code: any?): { type: string, error: { type: string, message: string, code: any? }? }
	return {
		type = "error",
		error = {
			type = err_type or "server_error",
			message = message or "Unknown error",
			code = code,
		},
	}
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
	want := map[string]struct {
		path path.Path
		typ  typ.Type
	}{
		"error.type":    {path: fieldChainSuffix("error", "type"), typ: typ.String},
		"error.message": {path: fieldChainSuffix("error", "message"), typ: typ.String},
		"error.code":    {path: fieldChainSuffix("error", "code"), typ: typ.MaterializeOptional(typ.Any)},
	}
	for _, entry := range literal.Entries() {
		suffix := entry.Suffix()
		for name, expected := range want {
			if !reflect.DeepEqual(suffix, expected.path) {
				continue
			}
			entryExpected, ok := entry.Expected()
			if !ok {
				t.Fatalf("%s entry missing expected contract", name)
			}
			got, ok := proof.New(reg, typevalue.NewCache()).ValueType(entryExpected)
			if !ok || !typ.TypeEquals(got, expected.typ) {
				t.Fatalf("%s expected = %v/%v, want %v", name, got, ok, expected.typ)
			}
			delete(want, name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing expected entries: %#v", want)
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

func TestLowerTypedCastObjectLiteralCarriesExpectedType(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local state = {
    active_sessions = {} :: {[string]: string},
}
`)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})

	point := requireStmtPoints(t, built, stmts[0], 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment fact")
	}
	root, ok := facts.ObjectLiteral(localFact.Source().ExprRef)
	if !ok {
		t.Fatalf("missing root object literal sidecar")
	}
	entries := root.Entries()
	if len(entries) != 1 {
		t.Fatalf("root literal entries = %#v, want one", entries)
	}
	castSource := entries[0].Source()
	castLiteral, ok := facts.ObjectLiteral(castSource.ExprRef)
	if !ok {
		t.Fatalf("missing casted field object literal sidecar for ref %d", castSource.ExprRef)
	}
	expected, ok := castLiteral.Expected()
	if !ok {
		t.Fatalf("casted field literal has no expected type")
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typetable.NewMap(typ.String, typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("casted field expected type = %v/%v, want %v", got, ok, want)
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

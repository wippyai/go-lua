package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/expressionid"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/normalize"
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

func TestLowerWIRTableConstructorSourceCarriesLiteralIdentity(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = { child = { leaf = 1 } }
`)
	reg := standard.Registry()
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[0], 1)[0]
	source := mustLocalSource(t, facts, point)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing object literal sidecar for WIR source ref %d", source.ExprRef)
	}
	if entries := literal.Entries(); len(entries) != 2 {
		t.Fatalf("WIR root literal entries = %#v, want root child and nested leaf", entries)
	}
	wantID, ok := literal.Identity()
	if !ok {
		t.Fatalf("WIR object literal missing identity")
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for WIR table source ref %d", source.ExprRef)
	}
	gotID, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || gotID != wantID {
		t.Fatalf("WIR table source identity = %v/%v, want object literal identity %v", gotID, ok, wantID)
	}
}

func TestLowerWIRObjectLiteralPublishesWithoutSemanticObjectLiteralFact(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
local t = { child = { leaf = payload.id } }
`, "payload")
	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", stmts[0])
	}
	tableExpr, ok := local.Exprs[0].(*ast.TableExpr)
	if !ok {
		t.Fatalf("expr = %T, want table constructor", local.Exprs[0])
	}
	body := wirlower.Lower("chunk", stmts, bindings, built)
	reg := standard.Registry()
	input := factflow.FactsInput{ObjectLiterals: make(map[factflow.ExprRef]factflow.ObjectLiteral)}
	l := lowerer{
		registry: reg,
		bindings: bindings,
		graph:    built.Graph,
		graphID:  built.Graph.ID(),
		wir:      body,
		exprs:    make(map[any]factflow.ExprRef),
	}
	l.addObjectLiteralExpr(&input, nil, tableExpr)
	facts := factflow.NewFacts(input)

	rootRef, ok := l.tableConstructorExprRef(tableExpr)
	if !ok {
		t.Fatal("missing WIR table expression ref")
	}
	literal, ok := facts.ObjectLiteral(rootRef)
	if !ok {
		t.Fatalf("missing WIR object literal for root ref %d without semantic fact", rootRef)
	}
	if got := len(literal.Entries()); got != 2 {
		t.Fatalf("root WIR entries = %#v, want flattened child and child.leaf", literal.Entries())
	}
	if gotID, ok := literal.Identity(); !ok || gotID != identity.LuaTableLiteral(built.Graph.ID(), uint64(rootRef)) {
		t.Fatalf("root WIR identity = %v/%v, want graph/ref identity", gotID, ok)
	}
	var nestedRef factflow.ExprRef
	for _, entry := range literal.Entries() {
		if !reflect.DeepEqual(entry.Suffix(), fieldChainSuffix("child")) {
			continue
		}
		source := entry.Source()
		if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
			nestedRef = source.ExprRef
		}
	}
	if nestedRef == 0 {
		t.Fatalf("root WIR entries = %#v, want child expression source", literal.Entries())
	}
	nested, ok := facts.ObjectLiteral(nestedRef)
	if !ok {
		t.Fatalf("missing nested WIR object literal for ref %d without semantic fact", nestedRef)
	}
	if entries := nested.Entries(); len(entries) != 1 || entries[0].ValueLabel() != "payload.id" {
		t.Fatalf("nested WIR entries = %#v, want payload.id leaf", entries)
	}
}

func TestLowerWithWIRObjectLiteralPublishesWithoutSemanticSidecars(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
local t = { child = { leaf = payload.id } }
`, "payload")
	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", stmts[0])
	}
	tableExpr, ok := local.Exprs[0].(*ast.TableExpr)
	if !ok {
		t.Fatalf("expr = %T, want table constructor", local.Exprs[0])
	}
	body := wirlower.Lower("chunk-no-sidecars", stmts, bindings, built)
	_ = tableExpr

	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	literals := facts.ObjectLiterals()
	if len(literals) < 2 {
		t.Fatalf("public WIR no-sidecar object literals = %#v, want root and nested constructors", literals)
	}
	var sawNestedLeaf bool
	for _, literal := range literals {
		for _, entry := range literal.Entries() {
			if reflect.DeepEqual(entry.Suffix(), fieldChainSuffix("leaf")) && entry.ValueLabel() == "payload.id" {
				sawNestedLeaf = true
			}
		}
	}
	if !sawNestedLeaf {
		t.Fatalf("public WIR no-sidecar object literals = %#v, want nested payload.id leaf", literals)
	}
}

func TestLowerWIRObjectLiteralCarriesDeclaredEntryContract(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Box = { items: {[string]: string}, label: string }
local box: Box = { items = {}, label = "" }
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	source := mustLocalSource(t, facts, point)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing WIR object literal for source ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 2 {
		t.Fatalf("WIR literal entries = %#v, want items and label", entries)
	}
	expected, ok := entries[0].Expected()
	if !ok {
		t.Fatalf("WIR object literal entry %s missing declared contract", entries[0].Suffix().String())
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typ.NewMap(typ.String, typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("WIR entry expected type = %v/%v, want %v", got, ok, want)
	}
	nestedSource := entries[0].Source()
	nested, ok := facts.ObjectLiteral(nestedSource.ExprRef)
	if !ok {
		t.Fatalf("missing nested WIR object literal for entry source ref %d", nestedSource.ExprRef)
	}
	nestedExpected, ok := nested.Expected()
	if !ok {
		t.Fatalf("nested WIR object literal missing declared contract")
	}
	nestedType, ok := typevalue.TypeOf(reg, nestedExpected)
	if !ok || !typ.TypeEquals(nestedType, want) {
		t.Fatalf("nested WIR expected type = %v/%v, want %v", nestedType, ok, want)
	}
}

func TestLowerWIRAnnotatedObjectLiteralUsesDeclaredOverlay(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Box = { items: {[string]: {id: string}}, count: number }
local box: Box = { items = {}, count = 0 }
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	fact, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing root assignment at point %d", point)
	}
	if fact.DeclaredValueContracts() {
		t.Fatalf("annotated object literal root assignment used declared replacement contract; want overlay preserving literal identity")
	}
	if !fact.DeclaredValueOverlays() {
		t.Fatalf("annotated object literal root assignment missing declared overlay")
	}
	declared, ok := fact.DeclaredValue()
	if !ok {
		t.Fatalf("annotated object literal root assignment missing declared value")
	}
	if got := product.Get(reg, declared, assertion.Key); !got.Has(assertion.TypeClaim) {
		t.Fatalf("declared overlay assertion = %s, want type claim", got)
	}
	source := fact.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("annotated object literal source = %#v, want expression source", source)
	}
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing object literal for source ref %d", source.ExprRef)
	}
	if id, ok := literal.Identity(); !ok || id == (identity.ID{}) {
		t.Fatalf("object literal identity = %s/%v, want stable table identity", id, ok)
	}
}

func TestLowerWIRAnnotatedArrayTableLiteralCarriesDeclaredContractClaim(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type SystemMessage = { role: "system" }
local final_messages: {SystemMessage} = {}
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	fact, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing root assignment at point %d", point)
	}
	if !fact.DeclaredValueContracts() {
		t.Fatalf("annotated array table literal used overlay/no contract; want declared contract")
	}
	declared, ok := fact.DeclaredValue()
	if !ok {
		t.Fatalf("annotated array table literal root assignment missing declared value")
	}
	if got := product.Get(reg, declared, assertion.Key); !got.Has(assertion.TypeClaim) {
		t.Fatalf("declared contract assertion = %s, want type claim", got)
	}
}

func TestLowerAnnotatedObjectLiteralUsesDeclaredOverlay(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Box = { items: {[string]: {id: string}}, count: number }
local box: Box = { items = {}, count = 0 }
`)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	fact, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing root assignment at point %d", point)
	}
	if fact.DeclaredValueContracts() {
		t.Fatalf("annotated object literal root assignment used declared replacement contract; want overlay preserving literal identity")
	}
	if !fact.DeclaredValueOverlays() {
		t.Fatalf("annotated object literal root assignment missing declared overlay")
	}
	declared, ok := fact.DeclaredValue()
	if !ok {
		t.Fatalf("annotated object literal root assignment missing declared value")
	}
	if got := product.Get(reg, declared, assertion.Key); !got.Has(assertion.TypeClaim) {
		t.Fatalf("declared overlay assertion = %s, want type claim", got)
	}
}

func TestLowerReturnedAnnotatedObjectLiteralUsesDeclaredOverlay(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, result := parseSemanticFunction(t, `
function build(): {items: {[string]: {id: string}}, count: number}
	local batch: {items: {[string]: {id: string}}, count: number} = {items = {}, count = 0}
	return batch
end
`)
	body := fn.Stmts
	point := requireStmtPoints(t, built, body[0], 1)[0]
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	fact, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing root assignment at point %d", point)
	}
	if fact.DeclaredValueContracts() {
		t.Fatalf("returned annotated object literal used declared replacement contract; want overlay preserving literal identity")
	}
	if !fact.DeclaredValueOverlays() {
		t.Fatalf("returned annotated object literal missing declared overlay")
	}
}

func TestLowerWIRObjectLiteralCarriesDeclaredEntryContractWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, _ := parseSemanticChunk(t, `
type Box = { items: {[string]: string}, label: string }
local box: Box = { items = {}, label = "" }
`)
	body := wirlower.Lower("chunk-no-sidecars", stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	source := mustLocalSource(t, facts, point)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing WIR object literal for source ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 2 {
		t.Fatalf("WIR literal entries = %#v, want items and label", entries)
	}
	expected, ok := entries[0].Expected()
	if !ok {
		t.Fatalf("WIR object literal entry %s missing declared contract without semantic sidecars", entries[0].Suffix().String())
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typ.NewMap(typ.String, typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("WIR entry expected type = %v/%v, want %v", got, ok, want)
	}
	nestedSource := entries[0].Source()
	nested, ok := facts.ObjectLiteral(nestedSource.ExprRef)
	if !ok {
		t.Fatalf("missing nested WIR object literal for entry source ref %d", nestedSource.ExprRef)
	}
	nestedExpected, ok := nested.Expected()
	if !ok {
		t.Fatalf("nested WIR object literal missing declared contract without semantic sidecars")
	}
	nestedType, ok := typevalue.TypeOf(reg, nestedExpected)
	if !ok || !typ.TypeEquals(nestedType, want) {
		t.Fatalf("nested WIR expected type = %v/%v, want %v", nestedType, ok, want)
	}
}

func TestLowerWIRContextualObjectLiteralExpressionValueUsesExpectedType(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Context = {[string]: any}
local input: { context: Context? } = { context = nil }
local user_ctx: Context = input.context or {}
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[2], 1)[0]
	source := mustLocalSource(t, facts, point)
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok {
		t.Fatalf("missing logical expression operation for source ref %d", source.ExprRef)
	}
	right := op.Right()
	if right.Kind != factflow.ValueSourceExpression || !right.HasExpr {
		t.Fatalf("logical fallback source = %#v, want expression source", right)
	}
	literal, ok := facts.ObjectLiteral(right.ExprRef)
	if !ok {
		t.Fatalf("missing WIR fallback object literal for ref %d", right.ExprRef)
	}
	if _, ok := literal.Expected(); !ok {
		t.Fatalf("WIR fallback object literal missing expected contract")
	}
	value, ok := facts.ExpressionValue(right.ExprRef)
	if !ok {
		t.Fatalf("missing WIR fallback expression value for ref %d", right.ExprRef)
	}
	got, ok := typevalue.TypeOf(reg, value)
	want := typ.NewMap(typ.String, typ.Any)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("WIR fallback expression type = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerWIRContextualObjectLiteralExpressionValueUsesExpectedTypeWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, _ := parseSemanticChunk(t, `
type Context = {[string]: any}
local input: { context: Context? } = { context = nil }
local user_ctx: Context = input.context or {}
`)
	body := wirlower.Lower("chunk-no-sidecars", stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[2], 1)[0]
	source := mustLocalSource(t, facts, point)
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok {
		t.Fatalf("missing logical expression operation for source ref %d", source.ExprRef)
	}
	right := op.Right()
	if right.Kind != factflow.ValueSourceExpression || !right.HasExpr {
		t.Fatalf("logical fallback source = %#v, want expression source", right)
	}
	literal, ok := facts.ObjectLiteral(right.ExprRef)
	if !ok {
		t.Fatalf("missing WIR fallback object literal for ref %d", right.ExprRef)
	}
	if _, ok := literal.Expected(); !ok {
		t.Fatalf("WIR fallback object literal missing expected contract without semantic sidecars")
	}
	value, ok := facts.ExpressionValue(right.ExprRef)
	if !ok {
		t.Fatalf("missing WIR fallback expression value for ref %d", right.ExprRef)
	}
	got, ok := typevalue.TypeOf(reg, value)
	want := typ.NewMap(typ.String, typ.Any)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("WIR fallback expression type = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerWIRObjectLiteralFieldExposureWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, _ := parseSemanticChunk(t, `
type Wide = { x: number | string }
local narrow: { x: number } = { x = 1 }
local holder: { ref: Wide } = { ref = narrow }
`)
	body := wirlower.Lower("chunk-no-sidecars", stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	holderStmt, ok := stmts[2].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want holder local assignment", stmts[2])
	}
	point := requireStmtPoints(t, built, holderStmt, 1)[0]
	exposures := facts.CovariantExposures(point)
	if len(exposures) != 1 {
		t.Fatalf("WIR field exposures = %#v, want one exposure without semantic sidecars", exposures)
	}
	wantPath := path.NewPath(mustLocalAt(t, bindings, stmts[1].(*ast.LocalAssignStmt), 0), "narrow")
	if got := exposures[0].SourcePath(); !got.Equal(wantPath) {
		t.Fatalf("WIR field exposure source = %v, want %v", got, wantPath)
	}
	gotType, ok := typevalue.TypeOf(reg, exposures[0].WideValue())
	wantType := typetable.NewRecord().Field("x", normalize.UnionForEvidence(typ.Number, typ.String)).Build()
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("WIR field exposure type = %v/%v, want %v", gotType, ok, wantType)
	}
}

func TestLowerWIRObjectLiteralEntrySourceComesFromWIR(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = { leaf = 1 }
`)
	localStmt := stmts[0].(*ast.LocalAssignStmt)
	table := localStmt.Exprs[0].(*ast.TableExpr)
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	target := path.NewPath(mustLocalAt(t, bindings, localStmt, 0), "t")
	body := wir.NewBody("synthetic-object-entry")
	value := wir.Operand{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "from-wir"}))}
	start := body.Emit(wir.Instruction{
		Op:     wir.OpMakeTable,
		Point:  point,
		Dst:    wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(target))},
		List:   body.AppendOperands([]wir.Operand{value}),
		Assign: wir.AssignLocalDeclaration,
		TableEntries: body.AppendTableEntries([]wir.TableEntry{
			{
				Suffix:     fieldSuffix("leaf"),
				Value:      value,
				ValueLabel: "from_wir.label",
			},
			{
				Suffix:     fieldSuffix("wir_only"),
				Value:      value,
				ValueLabel: "from_wir.extra",
			},
		}),
		ExprID: expressionid.Of(table),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	source := mustLocalSource(t, facts, point)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing WIR object literal for source ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want WIR entries, including the WIR-only entry", entries)
	}
	entrySource := entries[0].Source()
	if entrySource.Kind != factflow.ValueSourceLiteral || entrySource.LiteralKind != factflow.ValueSourceLiteralString || entrySource.String != "from-wir" {
		t.Fatalf("entry source = %#v, want WIR string literal", entrySource)
	}
	if got := entries[0].ValueLabel(); got != "from_wir.label" {
		t.Fatalf("entry label = %q, want WIR label", got)
	}
	if got := entries[1].Suffix(); !reflect.DeepEqual(got, fieldSuffix("wir_only")) {
		t.Fatalf("second entry suffix = %s, want WIR-only suffix", got.String())
	}
	if got := entries[1].ValueLabel(); got != "from_wir.extra" {
		t.Fatalf("WIR-only entry label = %q, want from_wir.extra", got)
	}
}

func TestLowerWIRObjectLiteralUnsupportedEntryDoesNotFallbackToSemanticSource(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = { leaf = value }
`, "value")
	localStmt := stmts[0].(*ast.LocalAssignStmt)
	table := localStmt.Exprs[0].(*ast.TableExpr)
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	target := path.NewPath(mustLocalAt(t, bindings, localStmt, 0), "t")
	body := wir.NewBody("unsupported-object-entry")
	typeRef := body.InternType(typ.String)
	start := body.Emit(wir.Instruction{
		Op:     wir.OpMakeTable,
		Point:  point,
		Dst:    wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(target))},
		Assign: wir.AssignLocalDeclaration,
		TableEntries: body.AppendTableEntries([]wir.TableEntry{
			{
				Suffix:     fieldSuffix("leaf"),
				Value:      wir.Operand{Kind: wir.OperandType, Ref: uint32(typeRef)},
				ValueLabel: "from_wir.unsupported",
			},
		}),
		ExprID: expressionid.Of(table),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	source := mustLocalSource(t, facts, point)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing WIR object literal for source ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want WIR entry", entries)
	}
	entrySource := entries[0].Source()
	if entrySource.Kind != factflow.ValueSourceUnknown || entrySource.HasExpr {
		t.Fatalf("entry source = %#v, want unknown without semantic expression fallback", entrySource)
	}
	if got := entries[0].ValueLabel(); got != "from_wir.unsupported" {
		t.Fatalf("entry label = %q, want WIR label", got)
	}
}

func TestLowerWIRObjectLiteralDoesNotKeepSemanticOnlyEntries(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = { leaf = 1, semantic_only = value }
`, "value")
	localStmt := stmts[0].(*ast.LocalAssignStmt)
	table := localStmt.Exprs[0].(*ast.TableExpr)
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	target := path.NewPath(mustLocalAt(t, bindings, localStmt, 0), "t")
	body := wir.NewBody("object-entry-no-semantic-extras")
	value := wir.Operand{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "from-wir"}))}
	start := body.Emit(wir.Instruction{
		Op:     wir.OpMakeTable,
		Point:  point,
		Dst:    wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(target))},
		Assign: wir.AssignLocalDeclaration,
		TableEntries: body.AppendTableEntries([]wir.TableEntry{
			{
				Suffix:     fieldSuffix("leaf"),
				Value:      value,
				ValueLabel: "from_wir.leaf",
			},
		}),
		ExprID: expressionid.Of(table),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	source := mustLocalSource(t, facts, point)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing WIR object literal for source ref %d", source.ExprRef)
	}
	entries := literal.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want only WIR-owned entries", entries)
	}
	if got := entries[0].Suffix(); !reflect.DeepEqual(got, fieldSuffix("leaf")) {
		t.Fatalf("entry suffix = %s, want only WIR leaf entry", got.String())
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

func TestLowerWithWIRDeclaredReturnAccumulatorWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, _ := parseSemanticFunction(t, `
function make(): {items: {[string]: string}}
    local out = {}
    return out
end
`)
	body := wirlower.LowerFunction("make", fn, bindings, built)

	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	localStmt, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing root assignment at point %d", point)
	}
	if !localFact.DeclaredValueContracts() {
		t.Fatalf("WIR returned local should carry declared return contract without semantic sidecars")
	}
	declared, ok := localFact.DeclaredValue()
	if !ok {
		t.Fatalf("missing declared return value")
	}
	gotType, ok := typevalue.TypeOf(reg, declared)
	want := typetable.NewRecord().
		Field("items", typetable.NewMap(typ.String, typ.String)).
		Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("WIR declared return local type = %v/%v, want %v", gotType, ok, want)
	}
}

func TestLowerWithWIRDeclaredReturnAccumulatorRejectsMixedSymbolsWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, _ := parseSemanticFunction(t, `
function make(flag: boolean): {any}
    local out = {}
    local other = {}
    if flag then
        return out
    end
    return other
end
`)
	body := wirlower.LowerFunction("make", fn, bindings, built)

	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
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
		t.Fatalf("WIR mixed return symbols should not infer accumulator contract: %v", declared)
	}
}

func TestLowerSetmetatableReturnLocalCarriesNonNilDeclaredContract(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, result := parseSemanticFunction(t, `
function make(): {run: (self: any) -> ()}?
    local mt = {}
    local instance = {}
    if false then
        return nil
    end
    return setmetatable(instance, mt), nil
end
`)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	localStmt, ok := fn.Stmts[1].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want instance local assignment", fn.Stmts[1])
	}
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing instance local assignment at point %d", point)
	}
	if !localFact.DeclaredValueContracts() {
		t.Fatalf("setmetatable-returned local should carry declared return contract")
	}
	declared, ok := localFact.DeclaredValue()
	if !ok {
		t.Fatalf("missing declared return value")
	}
	gotType, ok := typevalue.TypeOf(reg, declared)
	want := typetable.NewRecord().Field("run", typ.Func().Param("self", typ.Any).Build()).Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("declared return local type = %v/%v, want %v", gotType, ok, want)
	}
}

func TestLowerWithWIRSetmetatableReturnLocalWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, _ := parseSemanticFunction(t, `
function make(): {run: (self: any) -> ()}?
    local mt = {}
    local instance = {}
    if false then
        return nil
    end
    return setmetatable(instance, mt), nil
end
`)
	body := wirlower.LowerFunction("make", fn, bindings, built)

	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	localStmt, ok := fn.Stmts[1].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want instance local assignment", fn.Stmts[1])
	}
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	localFact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing instance local assignment at point %d", point)
	}
	if !localFact.DeclaredValueContracts() {
		t.Fatalf("WIR setmetatable-returned local should carry declared return contract without semantic sidecars")
	}
	declared, ok := localFact.DeclaredValue()
	if !ok {
		t.Fatalf("missing declared return value")
	}
	gotType, ok := typevalue.TypeOf(reg, declared)
	want := typetable.NewRecord().Field("run", typ.Func().Param("self", typ.Any).Build()).Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("WIR setmetatable return local type = %v/%v, want %v", gotType, ok, want)
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

func TestLowerWithWIRDynamicIndexLogicalDefaultObjectLiteralCarriesSlotContract(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local suites: {[string]: any[]} = {}
local suite = "alpha"
suites[suite] = suites[suite] or {}
`)
	body := wirlower.Lower("dynamic-default", stmts, bindings, built)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	want := typ.NewArray(typ.Any)

	foundExpected := false
	for _, literal := range facts.ObjectLiterals() {
		expected, ok := literal.Expected()
		if !ok {
			continue
		}
		got, ok := typevalue.TypeOf(reg, expected)
		if ok && typ.TypeEquals(got, want) {
			foundExpected = true
			break
		}
	}
	if !foundExpected {
		t.Fatalf("object literals = %#v, want WIR logical default constructor to carry dynamic slot contract %v", facts.ObjectLiterals(), want)
	}
	assignPoint := cfg.Point(0)
	for _, point := range cfg.RPOReadOnly(built.Graph) {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op == wir.OpDynamicIndexWrite {
				assignPoint = point
				break
			}
		}
		if assignPoint != 0 {
			break
		}
	}
	if assignPoint == 0 {
		t.Fatalf("missing WIR dynamic-index write\nWIR:\n%s", wir.Print(body, built.Graph))
	}
	write, ok := facts.DynamicIndexWrite(assignPoint)
	if !ok {
		t.Fatalf("missing dynamic-index write fact at WIR point %d\nWIR:\n%s", assignPoint, wir.Print(body, built.Graph))
	}
	source := write.Source()
	if !source.HasExpr {
		t.Fatalf("dynamic-index write source = %#v, want logical expression source", source)
	}
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok {
		t.Fatalf("missing logical expression operation for source ref %d", source.ExprRef)
	}
	if op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "or" {
		t.Fatalf("logical expression operation = %#v, want binary or", op)
	}
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

func TestLowerObjectLiteralCarriesOpenListElementSourceFromVararg(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function collect(...: number)
	local values = {...}
end`)
	localStmt := fn.Stmts[0].(*ast.LocalAssignStmt)
	body := wirlower.LowerFunction("collect", fn, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	source := mustLocalSource(t, facts, point)
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing object literal for vararg table source %#v", source)
	}
	element, ok := literal.ListElementSource()
	if !ok {
		t.Fatalf("missing list element source on vararg table literal %#v", literal)
	}
	if element.Kind != factflow.ValueSourceVararg || !element.Final || !element.Expanded || !element.OpenTail {
		t.Fatalf("list element source = %#v, want open expanded vararg", element)
	}
}

func TestLowerWithWIRReturnedObjectLiteralExpectedContractComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function new_actor(): { state: { processed: {[string]: string} } }
	return { state = { processed = {} } }
end`)
	body := wirlower.LowerFunction("new_actor", fn, bindings, built)
	fn.ReturnTypes = nil

	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
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
		t.Fatalf("missing returned object literal for ref %d", sources[0].ExprRef)
	}
	expected, ok := literal.Expected()
	if !ok {
		t.Fatalf("missing WIR-owned expected contract on returned object literal; return sources=%#v literals=%#v declared=%#v\nWIR:\n%s", sources, facts.ObjectLiterals(), body.DeclaredReturnTypes(), wir.Print(body, built.Graph))
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typetable.NewRecord().
		Field("state", typetable.NewRecord().
			Field("processed", typetable.NewMap(typ.String, typ.String)).
			Build()).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("WIR returned literal expected = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerWithWIRReturnedObjectLiteralExpectedContractWithoutSemanticResult(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function new_actor(): { state: { processed: {[string]: string} } }
	return { state = { processed = {} } }
end`)
	body := wirlower.LowerFunction("new_actor", fn, bindings, built)

	reg := standard.Registry()
	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
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
		t.Fatalf("missing returned object literal for ref %d", sources[0].ExprRef)
	}
	expected, ok := literal.Expected()
	if !ok {
		t.Fatalf("missing WIR-owned expected contract on returned object literal without semantic result; return sources=%#v literals=%#v declared=%#v\nWIR:\n%s", sources, facts.ObjectLiterals(), body.DeclaredReturnTypes(), wir.Print(body, built.Graph))
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typetable.NewRecord().
		Field("state", typetable.NewRecord().
			Field("processed", typetable.NewMap(typ.String, typ.String)).
			Build()).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("WIR returned literal expected = %v/%v, want %v", got, ok, want)
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
	rootValue, ok := facts.ExpressionValue(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing returned object literal expression value for ref %d", sources[0].ExprRef)
	}
	rootType, ok := typevalue.TypeOf(reg, rootValue)
	wantRoot := typetable.NewRecord().
		Field("type", typ.String).
		Field("error", typeexpr.Optional(typetable.NewRecord().
			Field("type", typ.String).
			Field("message", typ.String).
			Field("code", typ.MaterializeOptional(typ.Any)).
			Build())).
		Build()
	if !ok || !typ.TypeEquals(rootType, wantRoot) {
		t.Fatalf("returned object literal expression type = %v/%v, want %v", rootType, ok, wantRoot)
	}
	var nestedErrorRef factflow.ExprRef
	for _, entry := range literal.Entries() {
		if !reflect.DeepEqual(entry.Suffix(), fieldChainSuffix("error")) {
			continue
		}
		source := entry.Source()
		if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
			nestedErrorRef = source.ExprRef
		}
	}
	if nestedErrorRef == 0 {
		t.Fatalf("returned literal entries = %#v, want nested error source", literal.Entries())
	}
	nestedValue, ok := facts.ExpressionValue(nestedErrorRef)
	if !ok {
		t.Fatalf("missing nested returned object literal expression value for ref %d", nestedErrorRef)
	}
	nestedType, ok := typevalue.TypeOf(reg, nestedValue)
	wantNested := typetable.NewRecord().
		Field("type", typ.String).
		Field("message", typ.String).
		Field("code", typ.MaterializeOptional(typ.Any)).
		Build()
	if !ok || !typ.TypeEquals(nestedType, wantNested) {
		t.Fatalf("nested returned object literal expression type = %v/%v, want %v", nestedType, ok, wantNested)
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

func TestLowerDynamicIndexObjectLiteralCarriesExpectedAnyFieldType(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}
local state = {
    active_sessions = {} :: {[string]: ActiveSession},
}
local session_id = "s1"
state.active_sessions[session_id] = {
    pid = "pid",
    created_at = 1,
    terminating = false,
}
`)
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, TypeValues: typeValues})

	assignPoint := requireStmtPoints(t, built, stmts[3], 1)[0]
	write, ok := facts.DynamicIndexWrite(assignPoint)
	if !ok {
		t.Fatalf("missing dynamic-index write")
	}
	source := write.Source()
	lit, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing dynamic-index value object literal for ref %d", source.ExprRef)
	}
	expected, ok := lit.Expected()
	if !ok {
		t.Fatalf("dynamic-index value literal has no expected type")
	}
	got, ok := typeValues.TypeOf(reg, expected)
	want := typetable.NewRecord().
		Field("pid", typ.Any).
		Field("created_at", typ.Number).
		Field("terminating", typ.Boolean).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("dynamic-index value expected type = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerWIRDynamicIndexObjectLiteralExpectedContractWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}
local state = {
    active_sessions = {} :: {[string]: ActiveSession},
}
local session_id = "s1"
state.active_sessions[session_id] = {
    pid = "pid",
    created_at = 1,
    terminating = false,
}
`)
	body := wirlower.Lower("dynamic-index-object-expected-no-sidecars", stmts, bindings, built)
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	lowered := LowerWithSidecars(nil, built.Graph, Config{Registry: reg, Bindings: bindings, TypeValues: typeValues, WIR: body})
	facts := lowered.Facts

	assignPoint := requireStmtPoints(t, built, stmts[3], 1)[0]
	write, ok := facts.DynamicIndexWrite(assignPoint)
	if !ok {
		t.Fatalf("missing WIR dynamic-index write without semantic result")
	}
	source := write.Source()
	lit, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing WIR dynamic-index value object literal for ref %d", source.ExprRef)
	}
	expected, ok := lit.Expected()
	if !ok {
		t.Fatalf("WIR dynamic-index value literal has no expected type without semantic result\nWIR:\n%s", wir.Print(body, built.Graph))
	}
	got, ok := typeValues.TypeOf(reg, expected)
	want := typetable.NewRecord().
		Field("pid", typ.Any).
		Field("created_at", typ.Number).
		Field("terminating", typ.Boolean).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("WIR dynamic-index value expected type = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerWIROrdinaryAssignmentObjectLiteralExpectedContractWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
type Payload = { name: string, count: number }
local payload: Payload = { name = "", count = 0 }
payload = { name = "next", count = 1 }
`)
	body := wirlower.Lower("ordinary-object-expected-no-sidecars", stmts, bindings, built)
	reg := standard.Registry()
	facts := Lower(nil, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	point := requireStmtPoints(t, built, stmts[2], 1)[0]
	assign, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing WIR ordinary assignment at point %d without semantic sidecars", point)
	}
	source := assign.Source()
	lit, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		t.Fatalf("missing WIR ordinary assignment object literal for ref %d", source.ExprRef)
	}
	expected, ok := lit.Expected()
	if !ok {
		t.Fatalf("WIR ordinary assignment object literal missing expected type without semantic sidecars")
	}
	got, ok := typevalue.TypeOf(reg, expected)
	want := typetable.NewRecord().
		Field("name", typ.String).
		Field("count", typ.Number).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("ordinary assignment expected type = %v/%v, want %v", got, ok, want)
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

func TestTableConstructorExprRefKeyUsesWIRIdentityWhenAvailable(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
local t = {}
`)
	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", stmts[0])
	}
	tableExpr, ok := local.Exprs[0].(*ast.TableExpr)
	if !ok {
		t.Fatalf("expr = %T, want table constructor", local.Exprs[0])
	}
	body := wirlower.Lower("table-constructor-ref-key", stmts, bindings, built)
	id := expressionid.Of(tableExpr)
	if id == 0 {
		t.Fatal("missing expression id for table constructor")
	}

	l := lowerer{wir: body, exprs: make(map[any]factflow.ExprRef)}
	ref, ok := l.tableConstructorExprRef(tableExpr)
	if !ok {
		t.Fatal("missing WIR-backed table constructor ref")
	}
	key := wirTableExprRefKey{id: id}
	if got := l.exprs[key]; got != ref {
		t.Fatalf("WIR table key ref = %d, want %d", got, ref)
	}
	if _, ok := l.exprs[tableExpr]; ok {
		t.Fatalf("WIR-backed constructor used AST key %#v instead of WIR key", tableExpr)
	}
	existing, ok := l.existingTableConstructorExprRef(tableExpr)
	if !ok || existing != ref {
		t.Fatalf("existing WIR-backed ref = %d/%v, want %d", existing, ok, ref)
	}

	plain := lowerer{exprs: make(map[any]factflow.ExprRef)}
	plainRef, ok := plain.tableConstructorExprRef(tableExpr)
	if !ok {
		t.Fatal("missing AST-backed table constructor ref")
	}
	if got := plain.exprs[tableExpr]; got != plainRef {
		t.Fatalf("AST table key ref = %d, want %d", got, plainRef)
	}
	if _, ok := plain.exprs[key]; ok {
		t.Fatalf("AST-backed constructor unexpectedly used WIR key %#v", key)
	}
}

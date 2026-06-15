package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestGenericForIPairsElementCarriesObjectLiteralAnyField(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local raw: any = nil
local pages = {
	{ id = raw, route = "/ok" },
}
for _, page in ipairs(pages) do
	local id = page.id
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[2].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, local, 0)
	got, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0])
	if !ok {
		t.Fatal("ExpressionValueAtBoundary returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("page.id evidence = %s, want %s", gotEvidence, evidence.ExplicitTop())
	}
}

// TestGenericForLoopVarNegatedDiscriminantEdgeNarrowsRoot proves the else edge of
// a discriminant equality guard narrows an un-annotated generic-for loop variable
// to the complementary variant. The else-branch read item.payment_id must project
// the refund arm's required field as Present, not the union's optional string?.
func TestGenericForLoopVarNegatedDiscriminantEdgeNarrowsRoot(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Release = {kind: "release", token: string}
type Refund = {kind: "refund", payment_id: string}
type Compensation = Release | Refund

local items: {Compensation} = {}
for _, item in ipairs(items) do
	if item.kind == "release" then
		local token = item.token
	else
		local payment = item.payment_id
	end
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[4].(*ast.GenericForStmt)
	ifStmt := loop.Stmts[0].(*ast.IfStmt)
	elseLocal := ifStmt.Else[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, elseLocal, 0)
	got, ok := result.ExpressionValueAtBoundary(point, elseLocal.Exprs[0])
	if !ok {
		t.Fatal("ExpressionValueAtBoundary for item.payment_id returned false")
	}
	assertPresence(t, reg, got, presence.Present())
}

func TestGenericForPairsUsesAssertedIteratorSourceTypeForLoopVariables(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local transform_config: nil | string | {[string]: string} = {}
if type(transform_config) == "table" then
	for field_name, expression in pairs(transform_config :: {[string]: string}) do
		local field_copy = field_name
		local expression_copy = expression
	end
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	ifStmt := stmts[1].(*ast.IfStmt)
	loop := ifStmt.Then[0].(*ast.GenericForStmt)
	fieldLocal := loop.Stmts[0].(*ast.LocalAssignStmt)
	exprLocal := loop.Stmts[1].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, fieldLocal, typ.String)
	assertExpressionTypeAtBoundary(t, reg, result, exprLocal, typ.String)
}

func assertExpressionTypeAtBoundary(t *testing.T, reg *axis.Registry, result *Result, local *ast.LocalAssignStmt, want typ.Type) {
	t.Helper()
	point := requireLocalAssignmentPoint(t, result, local, 0)
	got, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0])
	if !ok {
		t.Fatalf("ExpressionValueAtBoundary returned false for %#v", local.Exprs[0])
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("expression type = %v/%v, want %v", gotType, ok, want)
	}
}

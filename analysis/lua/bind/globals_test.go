package bind

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestDirectGlobalCallsEnumeratePlainGlobalHeadsInSourceOrder(t *testing.T) {
	_, result := parseBindSource(t, `
require("first")
other()
require(require("nested"))
local require = function() end
require("shadowed")
module.require("member")
`)

	calls := result.DirectGlobalCalls()
	if got, want := len(calls), 4; got != want {
		t.Fatalf("DirectGlobalCalls length = %d, want %d", got, want)
	}
	for index, want := range []string{"require", "other", "require", "require"} {
		ident, ok := calls[index].Call.Func.(*ast.IdentExpr)
		if !ok || ident == nil || ident.Value != want {
			t.Fatalf("DirectGlobalCalls[%d] head = %T/%v, want %q", index, calls[index].Call.Func, ident, want)
		}
		if !calls[index].Global.Matches(want) {
			t.Fatalf("DirectGlobalCalls[%d] global does not match %q", index, want)
		}
	}

	calls[0].Call = nil
	if fresh := result.DirectGlobalCalls(); len(fresh) != 4 || fresh[0].Call == nil {
		t.Fatalf("DirectGlobalCalls did not return detached evidence: %#v", fresh)
	}
}

func TestDirectGlobalCallsPublishDetachedArgumentEvidence(t *testing.T) {
	_, result := parseBindSource(t, `
require("runtime")
require()
require("first", "second")
local request = "dynamic"
require(request)
require("")
`)

	calls := result.DirectGlobalCalls()
	if got, want := len(calls), 5; got != want {
		t.Fatalf("DirectGlobalCalls length = %d, want %d", got, want)
	}
	want := []struct {
		count  int
		value  string
		hasVal bool
	}{
		{count: 1, value: "runtime", hasVal: true},
		{count: 0},
		{count: 2},
		{count: 1},
		{count: 1, hasVal: true},
	}
	for index, expected := range want {
		call := calls[index]
		if call.ArgumentCount != expected.count || call.AuthoredString != expected.value || call.HasAuthoredString != expected.hasVal {
			t.Fatalf("call[%d] argument evidence = count %d/string %q/%v, want %d/%q/%v", index, call.ArgumentCount, call.AuthoredString, call.HasAuthoredString, expected.count, expected.value, expected.hasVal)
		}
	}
}

func TestGlobalCensusIncludesValueAndStaticRootsButExcludesTypeOnlyBase(t *testing.T) {
	stmts, err := parse.ParseString(`
local value = ordinary
type Query = typeof(static_value)
type Remote = stream.Stream
`, "global_census.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	census := result.GlobalCensus()
	if census.Len() != 3 {
		t.Fatalf("GlobalCensus length = %d, want 3", census.Len())
	}
	for _, name := range []string{"ordinary", "static_value", "stream"} {
		if !censusHasName(census, name) {
			t.Fatalf("GlobalCensus omitted %q", name)
		}
	}
	typeOnly, onlyOK := parseRuntimeBase(t, `
type Shape = number
local value = Shape(1)
`, "runtime_type_only_census.lua")
	typeOnlyResult := BindChunk(typeOnly)
	identity, ok := typeOnlyResult.GlobalIdentity(onlyOK)
	if !ok || identity.Name() != "Shape" {
		t.Fatalf("runtime type identity = %#v/%v, want Shape", identity, ok)
	}
	if got := typeOnlyResult.GlobalCensus().Len(); got != 0 {
		t.Fatalf("runtime-type-only census length = %d, want 0", got)
	}
	if _, ok := identity.Slot(); ok {
		t.Fatal("runtime-type-only identity received a Cell slot")
	}
}

func TestGlobalCensusRuntimeTypeIdentityUpgradesToOneCell(t *testing.T) {
	stmts, base := parseRuntimeBase(t, `
type Shape = number
local value = Shape(1)
local later = Shape
`, "runtime_type_upgrade_census.lua")
	result := BindChunk(stmts)
	later := stmts[2].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	baseIdentity, baseOK := result.GlobalIdentity(base)
	laterIdentity, laterOK := result.GlobalIdentity(later)
	if !baseOK || !laterOK || !baseIdentity.Same(laterIdentity) {
		t.Fatalf("runtime type identities = %#v/%v and %#v/%v, want one Result identity", baseIdentity, baseOK, laterIdentity, laterOK)
	}
	cell, ok := result.GlobalCensus().Cell(baseIdentity)
	if !ok || cell.Name() != "Shape" || cell.Slot() != 0 || cell.Ordinal() != 1 {
		t.Fatalf("upgraded global Cell = %#v/%v, want Shape/slot0/ordinal1", cell, ok)
	}
	if got, want := cell.Origin().Line, base.Line(); got != want {
		t.Fatalf("upgraded Cell origin line = %d, want first authored line %d", got, want)
	}
}

func TestGlobalCensusIncludesStaticPublicationRoot(t *testing.T) {
	stmts, err := parse.ParseString(`
type User = number
M.User = User
`, "global_census_publication.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	if !censusHasName(result.GlobalCensus(), "M") {
		t.Fatal("static publication root M was omitted from GlobalCensus")
	}
	root := stmts[1].(*ast.AssignStmt).Lhs[0].(*ast.AttrGetExpr).Object.(*ast.IdentExpr)
	identity, ok := result.GlobalIdentity(root)
	if !ok {
		t.Fatal("static publication root has no GlobalIdentity")
	}
	if _, ok := result.GlobalCensus().Cell(identity); !ok {
		t.Fatal("static publication root identity has no reserved Cell")
	}
}

func TestGlobalCensusAssignsAuthoredSlotsToWriteAndReadIdentities(t *testing.T) {
	stmts, err := parse.ParseString("lhs = rhs", "global_census_assignment_order.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	lhs := stmts[0].(*ast.AssignStmt).Lhs[0].(*ast.IdentExpr)
	rhs := stmts[0].(*ast.AssignStmt).Rhs[0].(*ast.IdentExpr)
	lhsIdentity, lhsOK := result.GlobalIdentity(lhs)
	rhsIdentity, rhsOK := result.GlobalIdentity(rhs)
	if !lhsOK || !rhsOK {
		t.Fatalf("assignment identities = %#v/%v and %#v/%v", lhsIdentity, lhsOK, rhsIdentity, rhsOK)
	}
	if lhsIdentity.Same(rhsIdentity) {
		t.Fatal("assignment write/read identities unexpectedly alias")
	}
	lhsCell, lhsCellOK := result.GlobalCensus().Cell(lhsIdentity)
	rhsCell, rhsCellOK := result.GlobalCensus().Cell(rhsIdentity)
	if !lhsCellOK || !rhsCellOK || lhsCell.Slot() != 0 || rhsCell.Slot() != 1 {
		t.Fatalf("assignment slots = %#v/%v and %#v/%v, want lhs=0 rhs=1", lhsCell, lhsCellOK, rhsCell, rhsCellOK)
	}
}

func TestGlobalCensusAEqualsBCallUsesAuthoredTargetOrder(t *testing.T) {
	stmts, err := parse.ParseString("A = B(A)", "global_census_call_assignment_order.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	for index, want := range []string{"A", "B"} {
		cell, ok := result.GlobalCensus().At(index)
		if !ok || cell.Name() != want || cell.Slot() != uint32(index) {
			t.Fatalf("GlobalCensus.At(%d) = %#v/%v, want %q at slot %d", index, cell, ok, want, index)
		}
	}
	if calls := result.DirectGlobalCalls(); len(calls) != 1 || !calls[0].Global.Matches("B") {
		t.Fatalf("A = B(A) direct calls = %#v, want one B", calls)
	}
}

func TestGlobalCensusNestedFunctionAssignmentDepthRemainsAuthoredLinear(t *testing.T) {
	stmts, err := parse.ParseString(`
local root = function()
	first = second
	local child = function()
		third = fourth
		local grand = function()
			fifth = sixth
		end
	end
end
`, "global_census_nested_assignment_depth.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	want := []string{"first", "second", "third", "fourth", "fifth", "sixth"}
	if got := result.GlobalCensus().Len(); got != len(want) {
		t.Fatalf("nested GlobalCensus length = %d, want %d", got, len(want))
	}
	for index, name := range want {
		cell, ok := result.GlobalCensus().At(index)
		if !ok || cell.Name() != name || cell.Slot() != uint32(index) {
			t.Fatalf("nested GlobalCensus.At(%d) = %#v/%v, want %q at slot %d", index, cell, ok, name, index)
		}
	}
}

func TestGlobalCensusNestedAssignmentsRemainSplicedInsideOuterRHS(t *testing.T) {
	stmts, err := parse.ParseString(`
A = function()
	B = function()
		C = function()
			D = E
		end
	end
end
`, "global_census_nested_active_assignment_frames.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	want := []string{"A", "B", "C", "D", "E"}
	census := result.GlobalCensus()
	if got := census.Len(); got != len(want) {
		t.Fatalf("nested active-frame GlobalCensus length = %d, want %d", got, len(want))
	}
	for index, name := range want {
		cell, ok := census.At(index)
		if !ok || cell.Name() != name || cell.Slot() != uint32(index) {
			t.Fatalf("nested active-frame GlobalCensus.At(%d) = %#v/%v, want %q at slot %d", index, cell, ok, name, index)
		}
	}
}

func TestGlobalCensusQualifiedRootAndPublicationUseExactSlots(t *testing.T) {
	stmts, err := parse.ParseString(`
type Remote = stream.Stream
M.User = Remote
`, "global_census_qualified_publication_slots.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	ref := stmts[0].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	rootID, ok := result.QualifiedTypeRootSymbol(ref)
	if !ok {
		t.Fatal("qualified TypeRef root symbol missing")
	}
	root, ok := result.GlobalIdentityOf(rootID)
	if !ok {
		t.Fatal("qualified TypeRef root identity missing")
	}
	publicationRoot := stmts[1].(*ast.AssignStmt).Lhs[0].(*ast.AttrGetExpr).Object.(*ast.IdentExpr)
	publication, ok := result.GlobalIdentity(publicationRoot)
	if !ok {
		t.Fatal("publication root identity missing")
	}
	rootCell, rootOK := result.GlobalCensus().Cell(root)
	publicationCell, publicationOK := result.GlobalCensus().Cell(publication)
	if !rootOK || !publicationOK || rootCell.Slot() != 0 || publicationCell.Slot() <= rootCell.Slot() {
		t.Fatalf("qualified/publication slots = %#v/%v and %#v/%v", rootCell, rootOK, publicationCell, publicationOK)
	}
}

func TestStaticQueryGlobalRetainsDirectCallEvidence(t *testing.T) {
	stmts, err := parse.ParseString(`
type ImportType = typeof(require("module"))
`, "static_query_require.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	if calls := result.DirectGlobalCalls(); len(calls) != 1 || !calls[0].Global.Matches("require") {
		t.Fatalf("static typeof(require(...)) DirectGlobalCalls = %#v, want one require", calls)
	}
	if !censusHasName(result.GlobalCensus(), "require") {
		t.Fatal("static typeof(require(...)) omitted require from GlobalCensus")
	}
}

func TestGlobalCensusUsesAuthoredOrderWhenAnEarlierTypeBaseUpgrades(t *testing.T) {
	stmts, err := parse.ParseString(`
type A = number
local only = A(1)
local first = B
local later = A
`, "global_census_upgrade_order.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	census := result.GlobalCensus()
	if census.Len() != 2 {
		t.Fatalf("GlobalCensus length = %d, want 2", census.Len())
	}
	for index, want := range []string{"A", "B"} {
		cell, ok := census.At(index)
		if !ok || cell.Name() != want || cell.Slot() != uint32(index) {
			t.Fatalf("GlobalCensus.At(%d) = %#v/%v, want %q at slot %d", index, cell, ok, want, index)
		}
	}
}

func TestGlobalCensusIsStableAndRejectsForeignIdentity(t *testing.T) {
	stmts, err := parse.ParseString(`
local first = alpha
local second = beta
local again = alpha
`, "global_census_order.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	census := result.GlobalCensus()
	if census.Len() != 2 {
		t.Fatalf("GlobalCensus length = %d, want 2", census.Len())
	}
	for index, want := range []string{"alpha", "beta"} {
		cell, ok := census.At(index)
		if !ok || cell.Name() != want || cell.Slot() != uint32(index) {
			t.Fatalf("GlobalCensus.At(%d) = %#v/%v, want %q at slot %d", index, cell, ok, want, index)
		}
	}
	alpha, ok := result.GlobalIdentity(stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr))
	if !ok || !census.Contains(alpha) {
		t.Fatal("same-Result alpha identity was not contained in census")
	}
	otherStmts, err := parse.ParseString("local other = alpha", "foreign_global_census.lua")
	if err != nil {
		t.Fatal(err)
	}
	other := BindChunk(otherStmts)
	foreign, ok := other.GlobalIdentity(otherStmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr))
	if !ok {
		t.Fatal("foreign identity was not bound")
	}
	if census.Contains(foreign) {
		t.Fatal("foreign Result identity entered census")
	}
	if _, ok := result.GlobalCell(foreign); ok {
		t.Fatal("Result accepted foreign global identity")
	}
}

func censusHasName(census GlobalCensus, name string) bool {
	for index := 0; index < census.Len(); index++ {
		cell, ok := census.At(index)
		if ok && cell.Name() == name {
			return true
		}
	}
	return false
}

func parseRuntimeBase(t testing.TB, source, name string) ([]ast.Stmt, *ast.IdentExpr) {
	t.Helper()
	stmts, err := parse.ParseString(source, name)
	if err != nil {
		t.Fatal(err)
	}
	assign := stmts[1].(*ast.LocalAssignStmt)
	call := assign.Exprs[0].(*ast.FuncCallExpr)
	base, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		t.Fatalf("runtime type base = %T, want identifier", call.Func)
	}
	return stmts, base
}

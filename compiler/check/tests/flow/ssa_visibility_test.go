package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/kind"
)

// TestSSAVisibility_NestedFuncBeforeAssignment verifies that a nested function
// defined BEFORE a variable assignment does NOT see the later assignment's type.
// This tests proper SSA version visibility.
func TestSSAVisibility_NestedFuncBeforeAssignment(t *testing.T) {
	source := `
		local x

		local function before_assign(): any
			return x
		end

		x = {name = "test", count = 42}

		local n: string = x.name
	`

	result := testutil.Check(source, testutil.WithStdlib())
	// The test should pass type checking because:
	// - before_assign returns 'any' (x is unknown at that point)
	// - After assignment, x has the record type
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestSSAVisibility_NestedFuncAfterAssignment verifies that a nested function
// defined AFTER a variable assignment DOES see the assigned type.
func TestSSAVisibility_NestedFuncAfterAssignment(t *testing.T) {
	source := `
		local x = {name = "test", count = 42}

		local function after_assign(): string
			return x.name
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	// This should pass because the nested function is defined AFTER the assignment
	// and should see the record type for x
	if result.HasError() {
		t.Fatalf("nested function after assignment should see enriched type: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestSSAVisibility_NestedFuncSeesWrongVersion verifies that a nested function
// defined BEFORE an assignment does NOT see the later assignment's type.
// The return type mismatch is the true indicator of the bug.
func TestSSAVisibility_NestedFuncSeesWrongVersion(t *testing.T) {
	// If the nested function sees x as Unknown, returning x.name as string should fail
	// because x.name is Unknown, not string.
	// But field access on Unknown is silently skipped, so we need a different approach.

	// Instead, we verify that the return value type mismatches.
	// If x is Unknown, then x.name is also Unknown, and assigning Unknown to string should fail.
	source := `
		local x

		local function f(): string
			local result: string = x.name
			return result
		end

		x = {name = "test"}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	// If the bug exists (nested func sees later assignment), x.name is string, assignment works.
	// If the bug is fixed, x.name is Unknown, assignment to string fails.
	if !result.HasError() {
		t.Error("nested function before assignment should NOT see later assignment's type - assigning Unknown to string should fail")
	}
}

// TestSSAVisibility_NestedFuncScopeInspection traces the scope at nested function definition
func TestSSAVisibility_NestedFuncScopeInspection(t *testing.T) {
	source := `
		local x

		local function f()
			local y = x
		end

		x = {name = "test"}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	rootResult := result.Session.RootResult
	if rootResult == nil || rootResult.Graph == nil {
		t.Fatal("no root result")
	}

	graph := rootResult.Graph

	// Find the nested function definition point
	nestedFuncs := graph.NestedFunctions()
	if len(nestedFuncs) == 0 {
		t.Fatal("no nested functions found")
	}

	nf := nestedFuncs[0]
	defPoint := nf.Point

	// Check what type x has at the nested function's definition point using TypeFacts
	sym, ok := graph.SymbolAt(defPoint, "x")
	if !ok {
		t.Fatal("x symbol not found at nested function definition point")
	}

	if rootResult.Facts == nil {
		t.Fatal("no type facts")
	}

	tv := rootResult.Facts.EffectiveTypeAt(defPoint, sym)

	// x should be unknown at the nested function definition point (before assignment)
	if tv.Type != nil && tv.Type.Kind() != kind.Unknown && tv.Type.Kind() != kind.Any {
		t.Errorf("x should be unknown at nested function definition point, got: %v (kind=%v)", tv.Type, tv.Type.Kind())
	}
}

// TestSSAVisibility_ParentSymbolTypesAtNestedPoint checks symbolTypes at nested function point
func TestSSAVisibility_ParentSymbolTypesAtNestedPoint(t *testing.T) {
	source := `
		local x

		local function f()
			local y = x
		end

		x = {name = "test"}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	rootResult := result.Session.RootResult
	if rootResult == nil || rootResult.Graph == nil {
		t.Fatal("no root result")
	}

	graph := rootResult.Graph
	nestedFuncs := graph.NestedFunctions()
	if len(nestedFuncs) == 0 {
		t.Fatal("no nested functions found")
	}

	nfPoint := nestedFuncs[0].Point

	// Check if x is visible at the nested function's definition point
	xSym, ok := graph.SymbolAt(nfPoint, "x")
	if !ok {
		t.Log("x not visible at nested function point")
		return
	}

	// Check the SSA version of x at different points
	xVerAtNF := graph.VisibleVersion(nfPoint, xSym)
	t.Logf("x SSA version at nested function point: %v", xVerAtNF)

	// Find the assignment point and check x's version there
	for _, p := range graph.RPO() {
		info := graph.Assign(p)
		if info != nil && len(info.Targets) > 0 {
			for _, target := range info.Targets {
				if target.Name == "x" && len(info.Sources) > 0 {
					xVerAtAssign := graph.VisibleVersion(p, xSym)
					t.Logf("x SSA version at assignment point %d: %v", p, xVerAtAssign)
					if xVerAtNF == xVerAtAssign {
						t.Errorf("x's SSA version at nested function point should differ from assignment point")
					}
				}
			}
		}
	}
}

package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/lint"
)

// TestTypeComparisonReturnIsLowered pins that a body whose return is a
// normalized type comparison reaches the engine at all. The CFG and the
// lowering must agree on whether the comparison owns its call; when they
// disagree the return statement is dropped and the whole body states nothing.
func TestTypeComparisonReturnIsLowered(t *testing.T) {
	result, err := engine.Check(`local function f(v: any): number
    return type(v) == "string" and v ~= ""
end
local x: any = 1
return f(x)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(result, "type.return.contract", "no proof shows it is number") {
		t.Fatalf("a lowered predicate body checks its declared return: %#v", result.PublishedDiagnostics)
	}
}

// TestTypeComparisonAssignmentResolves pins the same agreement in assignment
// position. A comparison whose call the lowering erased but the CFG did not
// leaves the assignment source unwritten, which stalls the whole analysis.
func TestTypeComparisonAssignmentResolves(t *testing.T) {
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{
		Entries: []lint.Entry{{Path: "main.lua", ModulePath: "main", Source: `local function f(v: any): boolean
    local ok = type(v) == "string"
    return ok
end
local x: any = 1
return f(x)`}},
		Targets: []string{"main"},
	})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(string(diagnostic.Code), "lint.analysis") {
			t.Fatalf("predicate assignment stalled analysis: %s", diagnostic.Message)
		}
	}
}

// TestComparisonResultDischargesABooleanDeclaration pins that an operator owns
// its result kind. Lua yields a boolean from a comparison whatever the operands
// hold, so a boundary that crossed into the operands decides which boolean the
// comparison produces, never whether it is one.
func TestComparisonResultDischargesABooleanDeclaration(t *testing.T) {
	discharged, err := engine.Check(`local function f(v: any): boolean
    return type(v) == "string" and v ~= ""
end
local x: any = 1
return f(x)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if hasPublished(discharged, "type.return.contract", "any/unknown") {
		t.Fatalf("a comparison result satisfies a boolean declaration: %#v", discharged.PublishedDiagnostics)
	}

	// The discharge is limited to declarations that accept every boolean; a
	// concrete declaration still owes its proof.
	owed, err := engine.Check(`local function f(v: any): number
    return type(v) == "string" and v ~= ""
end
local x: any = 1
return f(x)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(owed, "type.return.contract", "any/unknown") {
		t.Fatalf("a number declaration is not discharged by a boolean result: %#v", owed.PublishedDiagnostics)
	}
}

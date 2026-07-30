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

// TestUserPredicateTrueEdgeNarrowsArgument pins the interprocedural half of a
// one-sided type predicate: a callee whose returned value is truthy only where
// a type-equal check on its formal held decides, at the caller's true edge,
// that the argument carries the tested kind.
func TestUserPredicateTrueEdgeNarrowsArgument(t *testing.T) {
	narrowed, err := engine.Check(`local function is_number(value)
    return type(value) == "number"
end
local function run(a: any): ()
    if is_number(a) then
        local n: number = a
    end
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(narrowed.PublishedDiagnostics) != 0 {
		t.Fatalf("a proven true edge discharges the declared assignment: %#v", narrowed.PublishedDiagnostics)
	}
}

// TestUserPredicateConjunctionNarrowsArgument pins the same relation for a
// body whose return is an `and` chain. The bypass edge carries the type check's
// own false value, so a truthy result is reachable only where the check held.
func TestUserPredicateConjunctionNarrowsArgument(t *testing.T) {
	narrowed, err := engine.Check(`local function is_positive_number(value)
    return type(value) == "number" and value > 0
end
local function run(a: any): ()
    if is_positive_number(a) then
        local n: number = a
    end
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(narrowed.PublishedDiagnostics) != 0 {
		t.Fatalf("a conjunction predicate proves its leading type check: %#v", narrowed.PublishedDiagnostics)
	}
}

// TestUserPredicateFalseEdgeNarrowsNothing pins the one-sidedness. The body is
// a conjunction in general, so a false result refutes some conjunct rather than
// the type test, and the argument stays gradual past the guard.
func TestUserPredicateFalseEdgeNarrowsNothing(t *testing.T) {
	result, err := engine.Check(`local function is_number(value)
    return type(value) == "number"
end
local function run(a: any): ()
    if is_number(a) then
        local proven: number = a
    end
    local unproven: number = a
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(result, "type.assignment", "cannot assign a") {
		t.Fatalf("the false edge owes the boundary proof: %#v", result.PublishedDiagnostics)
	}
	if len(result.PublishedDiagnostics) != 1 {
		t.Fatalf("only the unguarded assignment is refuted: %#v", result.PublishedDiagnostics)
	}
}

// TestUserPredicateProvesOnlyItsOwnKind pins that the exported relation names
// the kind the body tested and no other.
func TestUserPredicateProvesOnlyItsOwnKind(t *testing.T) {
	result, err := engine.Check(`local function is_number(value)
    return type(value) == "number"
end
local function run(a: any): ()
    if is_number(a) then
        local text: string = a
    end
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(result, "type.assignment", "cannot assign a") {
		t.Fatalf("a number proof does not discharge a string declaration: %#v", result.PublishedDiagnostics)
	}
}

// TestNonPredicateCalleeNarrowsNothing pins the fail-closed default: a callee
// whose return states no type-equal check on a formal exports no relation, so
// its caller's true edge proves nothing about the argument.
func TestNonPredicateCalleeNarrowsNothing(t *testing.T) {
	result, err := engine.Check(`local function truthy(value)
    return value ~= nil
end
local function run(a: any): ()
    if truthy(a) then
        local n: number = a
    end
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(result, "type.assignment", "cannot assign a") {
		t.Fatalf("a non-predicate callee proves no kind: %#v", result.PublishedDiagnostics)
	}
}

// TestReboundPredicateCalleeNarrowsNothing pins that the relation belongs to
// the body the callee cell holds at the call. A later write rebinds that cell
// to another allocation, and the guard consumes the summary of the body it now
// holds rather than the one the cell was born with.
func TestReboundPredicateCalleeNarrowsNothing(t *testing.T) {
	result, err := engine.Check(`local function is_number(value)
    return type(value) == "number"
end
local function always(value)
    return true
end
local function run(a: any): ()
    local check = is_number
    check = always
    if check(a) then
        local n: number = a
    end
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(result, "type.assignment", "cannot assign a") {
		t.Fatalf("a rebound callee carries no predicate summary: %#v", result.PublishedDiagnostics)
	}
}

// TestDeclaredCalleeNarrowsNothing pins that a declaration is not a summary. A
// formal holds whatever a caller passed, so the body behind it is unknown here
// and its guard proves nothing about the argument.
func TestDeclaredCalleeNarrowsNothing(t *testing.T) {
	result, err := engine.Check(`local function run(check: any, a: any): ()
    if check(a) then
        local n: number = a
    end
end
return run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublished(result, "type.assignment", "cannot assign a") {
		t.Fatalf("a declared callee carries no predicate summary: %#v", result.PublishedDiagnostics)
	}
}

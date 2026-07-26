package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// declaredChildBoundary compiles one source and returns its single nested body
// together with the formal terms that body's declaration seeds.
func declaredChildBoundary(t *testing.T, source string) (front.Compilation, map[string]bool) {
	t.Helper()
	compilation, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(compilation.Nested) != 1 {
		t.Fatalf("nested bodies = %d, want 1", len(compilation.Nested))
	}
	child := compilation.Nested[0]
	formals := make(map[string]bool, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		formals[boundaryTerm(parameter.Symbol)] = true
	}
	return child, formals
}

// declaredAssignmentClaims names the annotation claims a declaration-only entry
// owns, keyed by the display the claim states.
func declaredAssignmentClaims(t *testing.T, child front.Compilation, formals map[string]bool) map[string]bool {
	t.Helper()
	derived := uncalledDeclaredFormalDerivedCells(child, formals)
	owned := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if !uncalledDeclaredFormalAssignment(child, operation, formals, derived) {
			continue
		}
		display, found := artifactOperand(operation.Operands, "display")
		if !found {
			t.Fatalf("claim %s has no display", operation.Target.Name)
		}
		owned[string(display)] = true
	}
	return owned
}

// TestShortCircuitResultCellIsDeclarationOwned states the relation the nested
// body needs: the front threads a value-position short-circuit through a cell it
// writes once before the guard and once on the edge that evaluates the right
// operand, so no claim names either operand. Every one of those writes names a
// static member of the seeded formal, so the cell holds a value that is a
// function of the declared parameter type and the contract stated on it is
// decided here.
func TestShortCircuitResultCellIsDeclarationOwned(t *testing.T) {
	child, formals := declaredChildBoundary(t, `type User = {name: string, nick: string?}
local function display(u: User): string
    local shown: string = u.nick or u.name
    return shown
end
return display`)
	if owned := declaredAssignmentClaims(t, child, formals); !owned["shown"] {
		t.Fatalf("declaration-owned assignment claims = %v, want the short-circuit result", owned)
	}
}

// TestShortCircuitResultCellStatesTheAndFormToo keeps both lowerings on the same
// relation: `and` writes the cell on the true edge instead of the false one.
func TestShortCircuitResultCellStatesTheAndFormToo(t *testing.T) {
	child, formals := declaredChildBoundary(t, `type Entry = {meta: {suite: string?}?}
local function suite(e: Entry): string?
    local found: string? = e.meta and e.meta.suite
    return found
end
return suite`)
	if owned := declaredAssignmentClaims(t, child, formals); !owned["found"] {
		t.Fatalf("declaration-owned assignment claims = %v, want the short-circuit result", owned)
	}
}

// TestCellFilledByAnUnaccountedWriteStaysDemandDriven is the fail-closed half: a
// call result is not a term this declaration owns, so the cell it fills carries
// no declaration-decided value and its contract keeps the ordinary path where a
// concrete call can still discharge it.
func TestCellFilledByAnUnaccountedWriteStaysDemandDriven(t *testing.T) {
	child, formals := declaredChildBoundary(t, `type User = {name: string, nick: string?}
local function display(u: User, make: fun(): string?): string
    local shown: string = make() or u.name
    return shown
end
return display`)
	if owned := declaredAssignmentClaims(t, child, formals); owned["shown"] {
		t.Fatalf("declaration-owned assignment claims = %v, want no call-result cell", owned)
	}
}

// TestDerivedCellsRefuseAConstructorBinding keeps a table out of the scalar
// chain: the write that binds a constructor to its cell carries a graph rather
// than a term, so the cell is not stated by this relation.
func TestDerivedCellsRefuseAConstructorBinding(t *testing.T) {
	child, formals := declaredChildBoundary(t, `type User = {name: string, nick: string?}
local function display(u: User): string
    local box = {value = u.name}
    local shown: string = box.value
    return shown
end
return display`)
	derived := uncalledDeclaredFormalDerivedCells(child, formals)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" || !declaredCellAllocationWrite(operation) {
			continue
		}
		target, found := artifactOperand(operation.Operands, "target")
		if found && derived[string(target)] {
			t.Fatalf("constructor cell %q entered the derived set", target)
		}
	}
}

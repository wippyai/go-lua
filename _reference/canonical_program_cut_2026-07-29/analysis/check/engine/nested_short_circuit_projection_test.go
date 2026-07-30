package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

// nestedShortCircuitMessages checks one source and returns the assignment
// refutations it publishes.
func nestedShortCircuitMessages(t *testing.T, source string) []string {
	t.Helper()
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	messages := make([]string, 0, len(result.PublishedDiagnostics))
	for _, diagnostic := range result.PublishedDiagnostics {
		messages = append(messages, diagnostic.Code+": "+diagnostic.Message)
	}
	return messages
}

// TestCalledBodyOrResultIsTheTruthyProjection states the headline relation
// inside a nested body: `a or b` reaches its bypass edge exactly when a is
// truthy, so an optional left operand beside a total right operand admits no
// nil and the whole result is the element type. A Top result would refute
// nothing and leave this body's narrowing vacuous.
func TestCalledBodyOrResultIsTheTruthyProjection(t *testing.T) {
	messages := nestedShortCircuitMessages(t, `type User = {name: number, nick: number?}
local function display(u: User): number
    local shown: number = u.nick or u.name
    return shown
end
return display({name = 1})`)
	if len(messages) != 0 {
		t.Fatalf("or projection in a called body published %v, want the whole result to be number", messages)
	}
}

// TestCalledBodyOrResultRefutesAWrongAnnotation is the same relation stated by
// its counterexample: the projection is a concrete type, so a contract it does
// not satisfy is refused rather than admitted for want of a value.
func TestCalledBodyOrResultRefutesAWrongAnnotation(t *testing.T) {
	messages := nestedShortCircuitMessages(t, `type User = {name: number, nick: number?}
local function display(u: User): string
    local shown: string = u.nick or u.name
    return shown
end
return display({name = 1})`)
	if len(messages) != 1 || messages[0] != "type.assignment: cannot assign shown because it is number, not string" {
		t.Fatalf("or projection refutation in a called body = %v", messages)
	}
}

// TestCalledBodyAndResultKeepsTheFalsyProjection is the mirror: `a and b`
// carries its left operand only when that operand is falsy, so an optional left
// operand contributes exactly its nil and the result stays optional.
func TestCalledBodyAndResultKeepsTheFalsyProjection(t *testing.T) {
	messages := nestedShortCircuitMessages(t, `type User = {name: number, nick: number?}
local function display(u: User): number
    local shown: number = u.nick and u.name
    return 0
end
return display({name = 1})`)
	if len(messages) != 1 || messages[0] != "type.assignment: cannot assign shown because it is number?, not number" {
		t.Fatalf("and projection refutation in a called body = %v", messages)
	}
}

// TestUncalledDeclarationCarriesTheSameProjection keeps the boundary honest: the
// projection is proven from the declared formal, so no invocation is needed to
// state the contract it refutes.
func TestUncalledDeclarationCarriesTheSameProjection(t *testing.T) {
	messages := nestedShortCircuitMessages(t, `type User = {name: number, nick: number?}
local function display(u: User): string
    local shown: string = u.nick or u.name
    return shown
end
return display`)
	if len(messages) != 1 || messages[0] != "type.assignment: cannot assign shown because it is number, not string" {
		t.Fatalf("or projection refutation in an uncalled declaration = %v", messages)
	}
}

// TestModuleScopeProjectionIsUnchanged pins the form that already held: the
// nested lanes state the same relation the module scope does.
func TestModuleScopeProjectionIsUnchanged(t *testing.T) {
	messages := nestedShortCircuitMessages(t, `type User = {name: number, nick: number?}
local u: User = {name = 1}
local shown: string = u.nick or u.name
return shown`)
	if len(messages) != 1 || messages[0] != "type.assignment: cannot assign shown because it is number, not string" {
		t.Fatalf("or projection refutation at module scope = %v", messages)
	}
}

// TestUnaccountedOperandLeavesTheBodyDormant is the fail-closed control: a call
// result is no term this declaration owns, so the cell it fills states no
// declaration-decided contract and a concrete caller still discharges it.
func TestUnaccountedOperandLeavesTheBodyDormant(t *testing.T) {
	messages := nestedShortCircuitMessages(t, `type User = {name: number, nick: number?}
local function display(u: User, make: fun(): number?): string
    local shown: string = make() or u.name
    return shown
end
return display`)
	if len(messages) != 0 {
		t.Fatalf("call-result operand published %v, want the demand-driven path", messages)
	}
}

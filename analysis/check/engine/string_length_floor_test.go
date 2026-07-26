package engine_test

import (
	"strings"
	"testing"
)

// stringFloorSource wraps one guarded body around an opaque string so the length
// floor is a guard rather than a folded constant.
func stringFloorSource(body string) string {
	return `local function need_integer(v: integer): integer return v end
local s: string = string.rep("a", 3)
if #s >= 3 then
` + body + `
end
return need_integer
`
}

// TestLengthFloorDischargesCoveredPosition pins the positional half of the
// standard-library contract: string.byte is optional because the position may
// lie past the end, and a floor of 3 puts positions 1 through 3 inside the
// string.
func TestLengthFloorDischargesCoveredPosition(t *testing.T) {
	for _, position := range []string{"1", "3", ""} {
		diagnostics := checkSource(t, stringFloorSource(
			`    local b = s:byte(`+position+`)
    need_integer(b)`))
		if len(diagnostics) != 0 {
			t.Fatalf("position %q inside the proven floor stayed optional:\n%s", position, diagnosticSummaries(diagnostics))
		}
	}
}

// TestLengthFloorLeavesUncoveredPosition pins where the floor stops. A position
// past it, at or below zero, or computed rather than constant is not covered by
// the guard, and the declared optional stands.
func TestLengthFloorLeavesUncoveredPosition(t *testing.T) {
	for _, position := range []string{"4", "0", "-1", "#s + 1"} {
		diagnostics := checkSource(t, stringFloorSource(
			`    local b = s:byte(`+position+`)
    need_integer(b)`))
		if len(diagnostics) == 0 {
			t.Fatalf("position %q was discharged by a floor that does not cover it", position)
		}
	}
}

// TestLengthFloorNamesItsOwnSubject pins that the floor must be proven for the
// string actually read: one string's length says nothing about another's.
func TestLengthFloorNamesItsOwnSubject(t *testing.T) {
	diagnostics := checkSource(t, `local function need_integer(v: integer): integer return v end
local s: string = string.rep("a", 3)
local u: string = string.rep("b", 9)
if #s >= 3 then
    local b = u:byte(1)
    need_integer(b)
end
return need_integer
`)
	if len(diagnostics) == 0 {
		t.Fatal("a floor on one string discharged a read of another")
	}
}

// TestUnguardedStringPositionStaysOptional pins the base case: without a floor
// the string may be empty, so even position 1 is past its end.
func TestUnguardedStringPositionStaysOptional(t *testing.T) {
	diagnostics := checkSource(t, `local function need_integer(v: integer): integer return v end
local s: string = string.rep("a", 3)
local b = s:byte(1)
need_integer(b)
return need_integer
`)
	if len(diagnostics) == 0 {
		t.Fatal("an unguarded string position was discharged")
	}
}

// TestGlobalAndMethodFormsShareOneContract pins that the colon method and the
// global call resolve the same positional condition, since the receiver binds as
// the signature's first operand.
func TestGlobalAndMethodFormsShareOneContract(t *testing.T) {
	covered := checkSource(t, stringFloorSource(
		`    local b = string.byte(s, 2)
    need_integer(b)`))
	if len(covered) != 0 {
		t.Fatalf("global form did not discharge a covered position:\n%s", diagnosticSummaries(covered))
	}
	past := checkSource(t, stringFloorSource(
		`    local b = string.byte(s, 4)
    need_integer(b)`))
	if len(past) == 0 {
		t.Fatal("global form discharged a position past the proven floor")
	}
}

// TestNestedProviderResultCarriesItsOptionality pins that the obligation belongs
// to the contract rather than to the spelling that consumes it: a call written
// straight into an argument list carries the same declared optional a local
// binding does.
func TestNestedProviderResultCarriesItsOptionality(t *testing.T) {
	diagnostics := checkSource(t, `local function need_integer(v: integer): integer return v end
local function f(s: string): integer
    return need_integer(s:byte(1))
end
return f
`)
	if !strings.Contains(diagnosticSummaries(diagnostics), "may be nil") {
		t.Fatalf("nested provider result discharged its own optionality:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestAnnotatedLocalCarriesProviderOptionality pins the same obligation on an
// annotated local whose initializer is the provider result itself.
func TestAnnotatedLocalCarriesProviderOptionality(t *testing.T) {
	diagnostics := checkSource(t, `local function f(s: string): integer
    local b: integer = s:byte(1)
    return b
end
return f
`)
	if !strings.Contains(diagnosticSummaries(diagnostics), "integer?") {
		t.Fatalf("annotated local discharged the provider's declared optional:\n%s", diagnosticSummaries(diagnostics))
	}
}

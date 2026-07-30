package engine_test

import (
	"strings"
	"testing"

	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

func unprovenNonNilClaim(diagnostics []diag.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Code == "lint.claim.unproven" && strings.Contains(item.Message, "non-nil") {
			return true
		}
	}
	return false
}

// TestAssertionOnNilNarrowedFormalIsDeclarationOwned pins the declaration-only
// admission of a non-nil assertion: the nil edge of the body's own guard states
// the formal's value there, so no caller argument can discharge the assertion
// and the boundary decides it without a call.
func TestAssertionOnNilNarrowedFormalIsDeclarationOwned(t *testing.T) {
	diagnostics := checkSource(t, `local function f(x: string?): string
    if x == nil then
        return x!
    end
    return x
end
return f
`)
	if !unprovenNonNilClaim(diagnostics) {
		t.Fatalf("an assertion on a nil-narrowed formal was left dormant:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestAssertionOnUnguardedFormalStaysDemandDriven keeps the admission from
// becoming a blanket rule: without an edge that states the formal's value, a
// concrete caller argument can still discharge the assertion, so the boundary
// publishes nothing.
func TestAssertionOnUnguardedFormalStaysDemandDriven(t *testing.T) {
	diagnostics := checkSource(t, `local function f(x: string?): string
    return x!
end
local ok: string = f("ready")
return ok
`)
	if unprovenNonNilClaim(diagnostics) {
		t.Fatalf("an unguarded assertion was decided without its call path:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestAssertionOnTruthyNarrowedFormalIsProven pins the other edge: the same
// admission evaluates the body, and a truthy edge proves the assertion rather
// than refuting it.
func TestAssertionOnTruthyNarrowedFormalIsProven(t *testing.T) {
	diagnostics := checkSource(t, `local function f(x: string?): string
    if x then
        return x!
    end
    return "fallback"
end
return f
`)
	if unprovenNonNilClaim(diagnostics) {
		t.Fatalf("an assertion the guard proves must stay silent:\n%s", diagnosticSummaries(diagnostics))
	}
}

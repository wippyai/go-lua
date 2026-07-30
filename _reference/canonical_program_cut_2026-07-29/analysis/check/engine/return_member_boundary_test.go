package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

func returnContract(t *testing.T, source string) engine.PublishedDiagnostic {
	t.Helper()
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.return.contract" {
			return diagnostic
		}
	}
	return engine.PublishedDiagnostic{}
}

// TestReturnedMemberFromAnyBoundaryIsRefutedAtTheMember pins that a declared
// record return is discharged field by field: a member filled from an
// unvalidated any/unknown boundary states the missing proof at the field the
// declaration names, not as an opaque whole-value mismatch.
func TestReturnedMemberFromAnyBoundaryIsRefutedAtTheMember(t *testing.T) {
	direct := returnContract(t, `type R = {status: string, message: string}
local function f(result: any): R
    return {status = "error", message = result.error}
end
return f`)
	if !strings.Contains(direct.Message, "returned value 1.message comes from any/unknown") ||
		!strings.Contains(direct.Message, "declared return type string") {
		t.Fatalf("member-anchored boundary message: got %q", direct.Message)
	}
}

// TestConcatenatedAnyMemberKeepsItsBoundary pins that an operator computing a
// runtime value from an unvalidated source discharges no declaration. The
// concatenation produces a string at runtime, but the member still holds a
// value nothing validated, so the declared field is not proven.
func TestConcatenatedAnyMemberKeepsItsBoundary(t *testing.T) {
	relayed := returnContract(t, `type R = {status: string, message: string}
local function f(result: any): R
    return {status = "error", message = "failed: " .. result.error}
end
return f`)
	if !strings.Contains(relayed.Message, "returned value 1.message comes from any/unknown") {
		t.Fatalf("concatenation relays the boundary onto the member: got %q", relayed.Message)
	}

	clean := returnContract(t, `type R = {status: string, message: string}
local function f(n: number): R
    return {status = "ok", message = "count: " .. n}
end
return f`)
	if clean.Code != "" {
		t.Fatalf("a concatenation of proven operands discharges the field: got %q", clean.Message)
	}
}

// TestReturnedMemberBoundaryIsClearedByItsValidator pins the fail-closed edge:
// the refutation follows the member's actual provenance. A runtime type test is
// that boundary's validator, a top-like declared field requires no proof, and a
// member with no any provenance keeps its ordinary relation.
func TestReturnedMemberBoundaryIsClearedByItsValidator(t *testing.T) {
	validated := returnContract(t, `type R = {status: string, message: string}
local function f(result: any): R
    if type(result.error) == "string" then
        return {status = "error", message = result.error}
    end
    return {status = "ok", message = "fine"}
end
return f`)
	if validated.Code != "" {
		t.Fatalf("a proven string edge validates the member: got %q", validated.Message)
	}

	gradualField := returnContract(t, `type R = {status: string, message: any?}
local function f(result: any): R
    return {status = "error", message = result.error}
end
return f`)
	if gradualField.Code != "" {
		t.Fatalf("an any? field states no concrete contract: got %q", gradualField.Message)
	}
}

// TestReturnedAggregateKeepsTheWholeValueMessage pins that the member branch
// claims only boundary refutations. A member refuted by its own observed value
// is still an aggregate mismatch and keeps the whole-value narration.
func TestReturnedAggregateKeepsTheWholeValueMessage(t *testing.T) {
	literal := returnContract(t, `type R = {status: string, message: number}
local function f(): R
    return {status = "ok", message = "text"}
end
return f`)
	if !strings.Contains(literal.Message, "returned value 1 is table, not") {
		t.Fatalf("a literal member mismatch keeps the aggregate message: got %q", literal.Message)
	}
}

// TestReturnedMemberBoundaryNarratesItsProvenance pins the evidence chain the
// member-anchored contract publishes: the member's own type, the declaration it
// must satisfy at the annotation's coordinate, the trust an any annotation
// carries, the unvalidated boundary, and the missing proof.
func TestReturnedMemberBoundaryNarratesItsProvenance(t *testing.T) {
	item := returnContract(t, `type R = {status: string, message: string}
local function f(result: any): R
    return {status = "error", message = result.error}
end
return f`)
	want := []string{
		"returned value 1.message has type any",
		"returned value 1.message must satisfy declared return type string",
		"user asserted any; not abstract-interpreter proof",
		"returned value 1.message comes from any/unknown",
		"no proof on this path shows returned value 1.message satisfies the declared return type",
	}
	if len(item.Evidence) != len(want) {
		t.Fatalf("evidence chain length = %d, want %d: %#v", len(item.Evidence), len(want), item.Evidence)
	}
	for index, message := range want {
		if !strings.Contains(item.Evidence[index].Message, message) {
			t.Fatalf("evidence %d = %q, want %q", index+1, item.Evidence[index].Message, message)
		}
	}
	if item.Evidence[0].Trust != diag.TrustProven || item.Evidence[1].Trust != diag.TrustClaimed || item.Evidence[2].Trust != diag.TrustClaimed {
		t.Fatalf("evidence trust: %#v", item.Evidence)
	}
	if item.Evidence[3].Reason != diag.EvidenceReasonExplicitBoundaryValidation || item.Evidence[4].Reason != diag.EvidenceReasonBoundaryValidationMissing {
		t.Fatalf("evidence reasons: %#v", item.Evidence)
	}
	if !strings.Contains(item.Help, "Return a value compatible with the declared return type") {
		t.Fatalf("help = %q", item.Help)
	}
	if len(item.Labels) != 2 || !strings.Contains(item.Labels[0].Message, "returned value") || !strings.Contains(item.Labels[1].Message, "declared return type") {
		t.Fatalf("labels = %#v", item.Labels)
	}
	// The contract is anchored where the member was filled, not at the return.
	if item.Span.StartLine != 3 || item.Span.StartCol != 41 {
		t.Fatalf("member span = %d:%d, want 3:41 (the member value, not the return)", item.Span.StartLine, item.Span.StartCol)
	}
	if item.Evidence[1].Span.StartLine != 2 {
		t.Fatalf("declaration evidence span line = %d, want 2", item.Evidence[1].Span.StartLine)
	}
}

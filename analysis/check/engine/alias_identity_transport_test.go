package engine_test

import (
	"strings"
	"testing"

	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

// narrowFieldCorrupted reports whether the diagnostics prove that a write
// through a wider alias reached narrow's own field.
func narrowFieldCorrupted(diagnostics []diag.Diagnostic, field string) bool {
	for _, item := range diagnostics {
		if item.Code == "type.assignment" && strings.Contains(item.Message, field) && strings.Contains(item.Message, "it is string, not number") {
			return true
		}
	}
	return false
}

// TestCastPreservesOperandTableIdentity pins that a cast is a view of its
// operand rather than a copy. The wider cast result addresses narrow's own
// cell, so the write through it corrupts narrow.x.
func TestCastPreservesOperandTableIdentity(t *testing.T) {
	diagnostics := checkSource(t, `local function covariant_cast(): number
    local narrow: {x: number} = {x = 1}
    local wide = narrow as {x: number | string}
    wide.x = "boom"
    local n: number = narrow.x
    return n
end
return covariant_cast
`)
	if !narrowFieldCorrupted(diagnostics, "narrow.x") {
		t.Fatalf("a write through a cast view did not reach the operand's cell:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestCastOfAnyStaysRuntimeValidated keeps the identity carry from becoming a
// second proof: a cast fed by an any boundary still has no table identity to
// carry, and the validated call remains silent.
func TestCastOfAnyStaysRuntimeValidated(t *testing.T) {
	diagnostics := checkSource(t, `local function need(o: {name: string}): number return 1 end
local function cast_runtime_validate(y: any): number
    return need(y as {name: string})
end
return need, cast_runtime_validate
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a runtime-validated cast must stay silent:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestNestedMemberWriteCrossesCallBoundary pins the nested mirror's cell: a
// callee writing through a two-segment path on a transported table authors the
// inner table's cell, which is the family the call boundary projects back.
func TestNestedMemberWriteCrossesCallBoundary(t *testing.T) {
	diagnostics := checkSource(t, `local function covariant_field_store(): number
    local narrow: {x: number} = {x = 1}
    local holder: {ref: {x: number | string}} = {ref = narrow}
    local function leak() holder.ref.x = "boom" end
    leak()
    local n: number = narrow.x
    return n
end
return covariant_field_store
`)
	if !narrowFieldCorrupted(diagnostics, "narrow.x") {
		t.Fatalf("a nested member write inside a closure was lost at the call boundary:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestNestedIndexWriteCrossesCallBoundary is the array-element spelling of the
// same nested address.
func TestNestedIndexWriteCrossesCallBoundary(t *testing.T) {
	diagnostics := checkSource(t, `local function covariant_index_store(): number
    local narrow: {x: number} = {x = 1}
    local sink: {{x: number | string}} = {narrow}
    local function leak() sink[1].x = "boom" end
    leak()
    local n: number = narrow.x
    return n
end
return covariant_index_store
`)
	if !narrowFieldCorrupted(diagnostics, "narrow.x") {
		t.Fatalf("a nested index write inside a closure was lost at the call boundary:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestFormalStoredIntoCapturedSinkEscapes pins the escape rule: a callee that
// stores its formal into a captured table's member hands the caller a second
// route to that argument, so the argument's identity crosses the call.
func TestFormalStoredIntoCapturedSinkEscapes(t *testing.T) {
	diagnostics := checkSource(t, `local function f(): number
    local narrow: {x: number} = {x = 1}
    local sink: {ref: {x: number | string}} = {ref = {x = 0}}
    local function stash(o: {x: number | string}) sink.ref = o end
    stash(narrow)
    sink.ref.x = "boom"
    local n: number = narrow.x
    return n
end
return f
`)
	if !narrowFieldCorrupted(diagnostics, "narrow.x") {
		t.Fatalf("a formal stored into a captured sink did not escape:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestReturnedContainerBindsArgumentIdentity pins the return-position binding:
// the container the callee returns holds the caller's own table in .ref, so a
// write through the returned member reaches the argument's cell.
func TestReturnedContainerBindsArgumentIdentity(t *testing.T) {
	diagnostics := checkSource(t, `local function ibox(o: {x: number | string}): {ref: {x: number | string}} return {ref = o} end
local function covariant_interproc(): number
    local narrow: {x: number} = {x = 1}
    local h = ibox(narrow)
    h.ref.x = "boom"
    local n: number = narrow.x
    return n
end
return ibox, covariant_interproc
`)
	if !narrowFieldCorrupted(diagnostics, "narrow.x") {
		t.Fatalf("a returned container did not bind its member to the argument:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestReturnedFreshContainerIsNotAliased keeps the minted result identity
// per-call: two invocations return two containers, so a write through one does
// not corrupt the table the other holds.
func TestReturnedFreshContainerIsNotAliased(t *testing.T) {
	diagnostics := checkSource(t, `local function ibox(o: {x: number | string}): {ref: {x: number | string}} return {ref = o} end
local function f(): number
    local first: {x: number} = {x = 1}
    local second: {x: number} = {x = 2}
    local a = ibox(first)
    local b = ibox(second)
    a.ref.x = "boom"
    local n: number = second.x
    return n
end
return ibox, f
`)
	if narrowFieldCorrupted(diagnostics, "second.x") {
		t.Fatalf("two calls aliased one returned container:\n%s", diagnosticSummaries(diagnostics))
	}
}

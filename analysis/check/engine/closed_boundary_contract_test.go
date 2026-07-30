package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

func hasPublishedCode(diagnostics []engine.PublishedDiagnostic, code string) bool {
	for _, item := range diagnostics {
		if string(item.Code) == code {
			return true
		}
	}
	return false
}

func TestClosedChildBodyHasNoEquationCountAdmissionCap(t *testing.T) {
	result, err := engine.Check(`
local function build()
    local wrong: string = 1
    local record: {name: string} = {}
    local total = 1 + 2
    local text = "a" .. "b"
    return record, total, text, wrong
end
return build
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasPublishedCode(result.PublishedDiagnostics, "type.assignment") {
		t.Fatalf("closed body claim was not published: %#v", result.PublishedDiagnostics)
	}
}

func TestClosedContiguousLiteralLengthIsExact(t *testing.T) {
	correct, err := engine.Check(`
local values = {"alpha", "beta"}
local count: 2 = #values
`)
	if err != nil {
		t.Fatalf("correct Check: %v", err)
	}
	if hasPublishedCode(correct.PublishedDiagnostics, "type.assignment") {
		t.Fatalf("exact sequence length was not accepted: %#v", correct.PublishedDiagnostics)
	}
	wrong, err := engine.Check(`
local values = {"alpha", "beta"}
local count: 3 = #values
`)
	if err != nil {
		t.Fatalf("wrong Check: %v", err)
	}
	if !hasPublishedCode(wrong.PublishedDiagnostics, "type.assignment") {
		t.Fatalf("wrong exact sequence length was accepted: %#v", wrong.PublishedDiagnostics)
	}
}

func TestCallableWireEnforcesStructuralArgumentContract(t *testing.T) {
	result, err := engine.Check(`
local function need(value: {name: string}): number return 1 end
local answer = need({name = 1})
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.PublishedDiagnostics {
		if string(item.Code) == "type.call.direct.argument_type" && strings.Contains(item.Message, "not {name: string}") {
			return
		}
	}
	t.Fatalf("structural parameter mismatch was not reported: %#v", result.PublishedDiagnostics)
}

// declaredUnionReturnSource builds a call site whose callee declares a
// discriminated union return and branches on its own argument, so evaluating
// the body with a literal reaches exactly one arm.
func declaredUnionReturnSource(argument, tail string) string {
	return `type Admin = {kind: "admin", id: string, level: number}
type Guest = {kind: "guest", id: string, expires: number}
type Principal = Admin | Guest

local function principal(index: number): Principal
    if index % 2 == 0 then
        return {kind = "admin", id = "a", level = index}
    end
    return {kind = "guest", id = "g", expires = index}
end

local u: Principal = principal(` + argument + `)
` + tail
}

// TestDeclaredUnionReturnKeepsBothArmsAtLiteralCallSite pins the callee's
// declaration as the caller's contract. The body reaches one arm for any given
// literal, but the declaration promises both, so both branches of the
// discriminant guard stay live and each keeps its own member contract.
func TestDeclaredUnionReturnKeepsBothArmsAtLiteralCallSite(t *testing.T) {
	for _, argument := range []string{"1", "2"} {
		diagnostics := checkSource(t, declaredUnionReturnSource(argument, `if u.kind == "admin" then
    local level: number = u.level
    local wrong: string = u.level
else
    local expires: number = u.expires
    local absent: number = u.level
end
`))
		if len(diagnostics) != 2 {
			t.Fatalf("principal(%s): both arms must be checked, got:\n%s", argument, diagnosticSummaries(diagnostics))
		}
		if diagnostics[0].Position.Line != 15 || string(diagnostics[0].Code) != "type.assignment" {
			t.Fatalf("principal(%s): admin arm lost its member contract:\n%s", argument, diagnosticSummaries(diagnostics))
		}
		if diagnostics[1].Position.Line != 18 || string(diagnostics[1].Code) != "type.member.missing" {
			t.Fatalf("principal(%s): guest arm lost its member contract:\n%s", argument, diagnosticSummaries(diagnostics))
		}
	}
}

// TestDeclaredUnionReturnDiscriminantHoldsBothLiterals proves the same contract
// on the discriminant itself: neither arm's literal is the call result's type,
// so assigning the discriminant to either one is refused for every argument.
func TestDeclaredUnionReturnDiscriminantHoldsBothLiterals(t *testing.T) {
	for _, argument := range []string{"1", "2"} {
		diagnostics := checkSource(t, declaredUnionReturnSource(argument, `local guest: "guest" = u.kind
local admin: "admin" = u.kind
`))
		if len(diagnostics) != 2 {
			t.Fatalf("principal(%s): the declared discriminant collapsed to one arm:\n%s", argument, diagnosticSummaries(diagnostics))
		}
	}
}

// TestDeclaredRecordReturnKeepsBodyDerivedResult keeps the complementary case
// green: a callee declaring a single record still hands its caller the value
// its body materialized, so member reads through the call result remain exact.
func TestDeclaredRecordReturnKeepsBodyDerivedResult(t *testing.T) {
	diagnostics := checkSource(t, `type Admin = {kind: "admin", id: string, level: number}

local function admin(index: number): Admin
    return {kind = "admin", id = "a", level = index}
end

local a = admin(1)
local level: number = a.level
local id: string = a.id
`)
	if len(diagnostics) != 0 {
		t.Fatalf("declared record return lost its body-derived result:\n%s", diagnosticSummaries(diagnostics))
	}
}

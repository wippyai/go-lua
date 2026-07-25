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

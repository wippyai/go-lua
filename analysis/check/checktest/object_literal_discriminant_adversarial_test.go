package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestObjectLiteralDotFieldDiscriminantSatisfiesUnionArm(t *testing.T) {
	result := Check(`
type Start = {kind: "start", payload: string}
type Stop = {kind: "stop", code: number}
type Event = Start | Stop

local event: Event = {kind = "stop", code = 1}
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for dot-field discriminant", result.Diagnostics)
	}
}

func TestObjectLiteralBracketStringDiscriminantDoesNotSatisfyDotFieldUnion(t *testing.T) {
	src := `
type Start = {kind: "start", payload: string}
type Stop = {kind: "stop", code: number}
type Event = Start | Stop

local event: Event = {["kind"] = "stop", code = 1}
`
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            6,
		Column:          22,
		Span:            diagnostic.Span{StartLine: 6, StartCol: 22, EndLine: 6, EndCol: 50},
		MessageContains: []string{
			`{code: 1, ["kind"]: "stop"}`,
			`not`,
			`{kind: "start", payload: string} | {code: number, kind: "stop"}`,
		},
		EvidenceMin: 2,
		EvidenceOrdered: []string{
			`assigned value has type {code: 1, ["kind"]: "stop"}`,
			`event is declared as Event`,
		},
		LabelMin:      2,
		LabelContains: []string{"declared type", "assigned value"},
		HelpContains: []string{
			"Use a value compatible with the expected type",
			"change the target type",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.assignment]: cannot assign {code: 1, ["kind"]: "stop"} to {kind: "start", payload: string} | {code: number, kind: "stop"}`,
			"--> test.lua:6:22",
			"↓ declared type",
			`6 | local event: Event = {["kind"] = "stop", code = 1}`,
			"↑ assigned value",
			"because:",
			`1. proven: assigned value has type {code: 1, ["kind"]: "stop"}`,
			"2. claimed: event is declared as Event",
			"help: Use a value compatible with the expected type",
		},
		RenderNotContains: []string{
			"^~",
			"where:",
			"want Event",
		},
	})
}

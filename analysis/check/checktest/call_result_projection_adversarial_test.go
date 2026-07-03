package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDirectCallCurrentReturnProjectionHandlesMultiStatementBody(t *testing.T) {
	src := strings.TrimLeft(`
local function make()
    local row = { id = 1 }
    return row
end

local got: { id: string } = make()
`, "\n")
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            6,
		Column:          29,
		Span: diagnostic.Span{
			StartLine: 6,
			StartCol:  29,
			EndLine:   6,
			EndCol:    32,
		},
		MessageContains: []string{"make(...)", "{id: 1}", "not", "{id: string}"},
		EvidenceMin:     2,
		EvidenceContains: []string{
			"make(...) has type {id: 1}",
			"got is declared as {id: string}",
		},
		EvidenceOrdered: []string{
			"make(...) has type {id: 1}",
			"got is declared as {id: string}",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"make(...)", "{id: 1}"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"got", "{id: string}"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"assigned value", "declared type"},
		HelpContains: []string{
			"Use a value compatible with the expected type",
			"change the target type",
			"`make(...)` is valid",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign make(...) because it is {id: 1}, not {id: string}",
			"test.lua:6:29",
			"↓ declared type",
			"6 | local got: { id: string } = make()",
			"↑ assigned value",
			"because:",
			"proven: make(...) has type {id: 1}",
			"claimed: got is declared as {id: string}",
			"help: Use a value compatible with the expected type",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

func TestDirectCallReturnedUnionPathComparisonNarrowsPayload(t *testing.T) {
	src := strings.TrimLeft(`
type ChanInt = {__tag: "int"}
type ChanStr = {__tag: "str"}
type SelResult =
    {channel: ChanInt, value: number, ok: boolean} |
    {channel: ChanStr, value: string, ok: boolean}

function get_result(a: ChanInt, b: ChanStr): SelResult
    return {channel = a, value = 42, ok = true}
end

function f(ch1: ChanInt, ch2: ChanStr)
    local result = get_result(ch1, ch2)
    if result.channel == ch1 then
        local n: number = result.value
    else
        local s: string = result.value
    end
end
`, "\n")
	result := Check(src)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no diagnostics for unreachable/narrowed alternate union arm", result.Diagnostics)
	}
}

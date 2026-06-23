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

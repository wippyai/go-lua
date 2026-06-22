package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// TestGenericForStatelessFunctionIteratorTypesLoopVariableAsString proves a
// generic-for loop over a stateless function iterator types the loop variable
// from the iterator function's result. gmatch returns fun(): string?, so w is
// string inside the loop body; assigning it to a number annotation is a type
// error.
func TestGenericForStatelessFunctionIteratorRejectsNumberAnnotation(t *testing.T) {
	src := strings.TrimLeft(`
local s: string = "hello world"
for w in s:gmatch("%a+") do
    local n: number = w
end
`, "\n")
	result := Check(src, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            3,
		Column:          23,
		Span:            diagnostic.Span{StartLine: 3, StartCol: 23, EndLine: 3, EndCol: 24},
		MessageContains: []string{"cannot assign w", "string", "not number"},
		EvidenceMin:     3,
		EvidenceOrdered: []string{
			"w has type string",
			"n is declared as number",
			"no proof on this path shows w satisfies the declared type",
		},
		LabelMin: 2,
		LabelContains: []string{
			"assigned value",
			"declared type",
		},
		HelpContains: []string{"Use a value compatible", "change the target type"},
		Sources:      diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign w because it is string, not number",
			"test.lua:3:",
			"declared type",
			"3 |     local n: number = w",
			"assigned value",
			"because:",
			"proven: w has type string",
			"claimed: n is declared as number",
			"missing proof: no proof on this path shows w satisfies the declared type",
			"help:",
			"Use a value compatible with the expected type",
		},
		RenderNotContains: []string{
			"want number",
			"^~",
		},
	})
}

// TestGenericForStatelessFunctionIteratorAcceptsStringAnnotation proves the same
// loop variable is the iterator function's non-nil first result: assigning it to
// a string annotation checks clean.
func TestGenericForStatelessFunctionIteratorAcceptsStringAnnotation(t *testing.T) {
	result := Check(`
local s: string = "hello world"
for w in s:gmatch("%a+") do
	local ok: string = w
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for assigning string loop variable to string", result.Diagnostics)
	}
}

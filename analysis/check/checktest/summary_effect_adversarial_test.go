package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestSummaryDynamicIndexWriteInvalidatesCallerGuard(t *testing.T) {
	src := strings.TrimLeft(`
type Box = { value: string? }

local function clear(box: Box, key: string): ()
    box[key] = nil
end

local box: Box = { value = "ready" }
if box.value then
    clear(box, "value")
    local after: string = box.value
end
`, "\n")
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            10,
		Column:          27,
		Span: diagnostic.Span{
			StartLine: 10,
			StartCol:  27,
			EndLine:   10,
			EndCol:    35,
		},
		MessageContains: []string{"cannot assign box.value", "is nil", "not string"},
		EvidenceMin:     4,
		EvidenceContains: []string{
			"box.value has type nil",
			"after is declared as string",
			"clear(...) may change box, so the read of box.value needs a fresh check",
			"no proof on this path shows box.value satisfies the declared type",
		},
		EvidenceOrdered: []string{
			"box.value has type nil",
			"after is declared as string",
			"clear(...) may change box, so the read of box.value needs a fresh check",
			"no proof on this path shows box.value satisfies the declared type",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"box.value", "nil"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"after", "string"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"clear(...)", "fresh check"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no proof", "declared type"},
			},
		},
		LabelMin: 2,
		LabelContains: []string{
			"assigned value",
			"declared type",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign box.value because it is nil, not string",
			"test.lua:10:27",
			"declared type",
			"10 |     local after: string = box.value",
			"assigned value",
			"because:",
			"proven: box.value has type nil",
			"claimed: after is declared as string",
			"proven: clear(...) may change box, so the read of box.value needs a fresh check",
			"missing proof: no proof on this path shows box.value satisfies the declared type",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

func TestExportedDynamicIndexWriteInvalidatesCallerGuard(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.clear(box: { value: string? }, key: string): ()
    box[key] = nil
end

return M
`, "mutator")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	src := strings.TrimLeft(`
local mutator = require("mutator")
type Box = { value: string? }

local box: Box = { value = "ready" }
if box.value then
    mutator.clear(box, "value")
    local after: string = box.value
end
`, "\n")
	result := Check(src, WithStdlib(), WithModule("mutator", mod))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            7,
		Column:          27,
		Span: diagnostic.Span{
			StartLine: 7,
			StartCol:  27,
			EndLine:   7,
			EndCol:    35,
		},
		MessageContains: []string{"cannot assign box.value", "is nil", "not string"},
		EvidenceMin:     4,
		EvidenceContains: []string{
			"box.value has type nil",
			"after is declared as string",
			"mutator.clear(...) may change box, so the read of box.value needs a fresh check",
			"no proof on this path shows box.value satisfies the declared type",
		},
		EvidenceOrdered: []string{
			"box.value has type nil",
			"after is declared as string",
			"mutator.clear(...) may change box, so the read of box.value needs a fresh check",
			"no proof on this path shows box.value satisfies the declared type",
		},
		LabelMin: 2,
		LabelContains: []string{
			"assigned value",
			"declared type",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign box.value because it is nil, not string",
			"test.lua:7:27",
			"declared type",
			"7 |     local after: string = box.value",
			"assigned value",
			"because:",
			"proven: box.value has type nil",
			"claimed: after is declared as string",
			"proven: mutator.clear(...) may change box, so the read of box.value needs a fresh check",
			"missing proof: no proof on this path shows box.value satisfies the declared type",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

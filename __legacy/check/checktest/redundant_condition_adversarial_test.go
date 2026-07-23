package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestRedundantConditionWarningUsesImportedPureSummaryEvidence(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.observe(box: { value: string? }): ()
end

function M.clear(box: { value: string? }): ()
    box.value = nil
end

return M
`, "ops")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	t.Run("pure imported call preserves guarded member", func(t *testing.T) {
		result := Check(`
local ops = require("ops")
type Box = { value: string? }

local box: Box = { value = "ready" }
if box.value then
    ops.observe(box)
    if box.value then
        return box.value
    end
end
`, WithStdlib(), WithModule("ops", mod), WithDiagnosticRule(
			diagnostics.CodeRedundantCondition,
			diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
		))
		if len(result.Diagnostics) != 1 {
			t.Fatalf("diagnostics = %#v, want one redundant-condition diagnostic", result.Diagnostics)
		}
		diag := requireDiagnosticCode(t, result, diagnostics.CodeRedundantCondition)
		if diag.Severity != diagnostic.SeverityHint {
			t.Fatalf("diagnostic = %#v, want hint severity", diag)
		}
		requireEvidenceMessage(t, diag, "current check")
		requireEvidenceMessage(t, diag, "prior guard established")
		requireEvidenceMessage(t, diag, "box.value is unchanged between the prior guard and this check")
		requireLabelMessage(t, diag, "current check")
		requireLabelMessage(t, diag, "prior guard")
	})

	t.Run("mutating imported call invalidates guarded member", func(t *testing.T) {
		result := Check(`
local ops = require("ops")
type Box = { value: string? }

local box: Box = { value = "ready" }
if box.value then
    ops.clear(box)
    if box.value then
        return box.value
    end
end
`, WithStdlib(), WithModule("ops", mod), WithDiagnosticRule(
			diagnostics.CodeRedundantCondition,
			diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
		))
		for _, diag := range result.Diagnostics {
			if diag.Code == diagnostics.CodeRedundantCondition {
				t.Fatalf("diagnostics = %#v, want imported mutation to invalidate member guard", result.Diagnostics)
			}
		}
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want no diagnostics after imported mutation invalidates guard", result.Diagnostics)
		}
	})
}

func TestRedundantConditionWarningIgnoresShadowedAssignmentInvalidation(t *testing.T) {
	result := Check(`
local value = true
if value then
    do
        local value = false
        value = true
    end
    if value then
        return value
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeRedundantCondition,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one redundant-condition diagnostic for original value", result.Diagnostics)
	}
	diag := requireDiagnosticCode(t, result, diagnostics.CodeRedundantCondition)
	requireEvidenceMessage(t, diag, "prior guard established")
	requireEvidenceMessage(t, diag, "value is unchanged between the prior guard and this check")
}

func TestRedundantConditionWarningUsesExactNilPredicateEvidence(t *testing.T) {
	result := Check(`
type Cache = { value: string? }

local function inspect(cache: Cache): ()
    if cache.value ~= nil then
        if cache.value ~= nil then
            local stable: string = cache.value
        end
    end

    if cache.value == nil then
        if cache.value ~= nil then
            local impossible = cache.value
        end
    end
end

return inspect
`, WithDiagnosticRule(
		diagnostics.CodeRedundantCondition,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want two redundant-condition diagnostics", result.Diagnostics)
	}
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeRedundantCondition,
		Severity:        diagnostic.SeverityHint,
		DiagnosticCount: 2,
		Line:            6,
		Column:          12,
		MessageContains: []string{"condition is always true here"},
		EvidenceOrdered: []string{
			"current check: cache.value ~= nil",
			"prior guard established cache.value is not nil",
			"cache.value is unchanged between the prior guard and this check",
		},
		LabelContains: []string{"current check", "prior guard"},
		HelpContains:  []string{"Remove this repeated check"},
	})
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeRedundantCondition,
		Severity:        diagnostic.SeverityHint,
		DiagnosticCount: 2,
		Line:            12,
		Column:          12,
		MessageContains: []string{"condition is always false here"},
		EvidenceOrdered: []string{
			"current check: cache.value ~= nil",
			"prior guard established cache.value is nil",
			"cache.value is unchanged between the prior guard and this check",
		},
		LabelContains: []string{"current check", "prior guard"},
		HelpContains:  []string{"Remove this unreachable branch"},
	})
	for _, diag := range result.Diagnostics {
		for _, evidence := range diag.Explanation.Evidence() {
			if strings.Contains(evidence.Message, "checked for non-nil") || strings.Contains(evidence.Message, "checked for nil") {
				t.Fatalf("evidence uses vague nil-check wording: %#v", diag)
			}
		}
	}
}

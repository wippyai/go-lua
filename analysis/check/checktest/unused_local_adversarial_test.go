package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestUnusedLocalWarningTreatsClosureCaptureAsRead(t *testing.T) {
	result := Check(`
local value = "ok"
local function get(): string
    return value
end
return get()
`, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none because closure capture reads value", result.Diagnostics)
	}
}

func TestUnusedLocalWarningTreatsNestedClosureCaptureAsRead(t *testing.T) {
	result := Check(`
local value = "ok"
local make_get = function()
    return function(): string
        return value
    end
end
return make_get()()
`, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none because nested closure capture reads value", result.Diagnostics)
	}
}

func TestUnusedLocalWarningTreatsReachableTypeofQueriesAsRead(t *testing.T) {
	for _, src := range []string{
		`
local value = { status = "ready" }
type Snapshot = typeof(value)
`,
		`
local value = { status = "ready" }
local copy: typeof(value) = nil
return copy
`,
		`
local value = { status = "ready" }
local function consume(input: typeof(value)): ()
end
return consume
`,
		`
local value = { status = "ready" }
local function wrap(input)
    return input
end
type Snapshot = typeof(wrap(value))
`,
	} {
		result := Check(src, WithDiagnosticRule(
			diagnostics.CodeUnusedLocal,
			diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
		))
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want none because reachable typeof(value) reads value", result.Diagnostics)
		}
	}
}

func TestUnusedLocalWarningIgnoresTypeofQueriesInUnreachableBranches(t *testing.T) {
	result := Check(`
local value = "unused"
local item = { kind = "ready" }
if item.kind == "ready" then
    if item.kind == "other" then
        type Snapshot = typeof(value)
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unused-local diagnostic", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeUnusedLocal || diag.Position.Line != 2 {
		t.Fatalf("diagnostic = %#v, want unused-local diagnostic for line 2", diag)
	}
	requireEvidenceMessage(t, diag, `no read of local "value" was found in this scope`)
}

func TestUnusedLocalWarningIgnoresReadInUnreachableBranch(t *testing.T) {
	src := strings.TrimLeft(`
local value = "unused"
local item = { kind = "ready" }
if item.kind == "ready" then
    if item.kind == "other" then
        return value
    end
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeUnusedLocal,
		Severity:        diagnostic.SeverityHint,
		DiagnosticCount: 1,
		Line:            1,
		Column:          7,
		Span:            diagnostic.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 11},
		MessageContains: []string{`local "value" is never read`},
		EvidenceMin:     1,
		EvidenceOrdered: []string{
			`no read of local "value" was found in this scope`,
		},
		LabelMin:      1,
		LabelContains: []string{"unused local"},
		HelpContains:  []string{"Remove it", "rename it with a leading _"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`hint[lint.unused.local]: local "value" is never read`,
			"test.lua:1:7",
			"1 | local value = \"unused\"",
			"unused local",
			"because:",
			`proven: no read of local "value" was found in this scope`,
			"help:",
			"Remove it, use it, or rename it with a leading _",
		},
		RenderNotContains: []string{
			"return value",
			"kind == \"other\"",
			"^~",
		},
	})
}

func TestUnusedLocalWarningIgnoresUnreachableClosureCapture(t *testing.T) {
	result := Check(`
local value = "unused"
local item = { kind = "ready" }
if item.kind == "ready" then
    if item.kind == "other" then
        local get = function(): string
            return value
        end
        return get
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unused-local diagnostic", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeUnusedLocal || diag.Position.Line != 2 {
		t.Fatalf("diagnostic = %#v, want unused-local diagnostic for line 2", diag)
	}
	requireEvidenceMessage(t, diag, `no read of local "value" was found in this scope`)
}

func TestUnusedLocalWarningReportsOnlyShadowedOuterBinding(t *testing.T) {
	result := Check(`
local value = "outer"
do
    local value = "inner"
    return value
end
`, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unused-local diagnostic for shadowed outer binding", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeUnusedLocal || diag.Severity != diagnostic.SeverityHint {
		t.Fatalf("diagnostic = %#v, want unused-local hint", diag)
	}
	if diag.Position.Line != 2 {
		t.Fatalf("diagnostic line = %d, want outer binding on line 2", diag.Position.Line)
	}
	requireEvidenceMessage(t, diag, `no read of local "value" was found in this scope`)
}

func TestUnusedAndDeadAssignmentWarningsBothExplainWriteOnlyOverwrite(t *testing.T) {
	result := Check(`
local value = 1
value = 2
`, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	), WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want unused-local and dead-assignment diagnostics", result.Diagnostics)
	}

	unused := requireDiagnosticCode(t, result, diagnostics.CodeUnusedLocal)
	if unused.Severity != diagnostic.SeverityHint {
		t.Fatalf("unused diagnostic = %#v, want hint severity", unused)
	}
	requireEvidenceMessage(t, unused, `no read of local "value" was found in this scope`)

	dead := requireDiagnosticCode(t, result, diagnostics.CodeDeadAssignment)
	if dead.Severity != diagnostic.SeverityHint {
		t.Fatalf("dead-assignment diagnostic = %#v, want hint severity", dead)
	}
	requireEvidenceMessage(t, dead, `later assignment replaces "value" before the earlier value is read`)
}

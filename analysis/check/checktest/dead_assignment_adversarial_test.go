package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDeadAssignmentWarningIgnoresReadInImpossibleBranch(t *testing.T) {
	result := Check(`
local value = "old"
local flag = "old"
if flag == "old" then
    if flag == "new" then
        local observed = value
    end
end
value = "new"
return value
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one dead-assignment diagnostic; impossible branch read must not keep old value alive", result.Diagnostics)
	}
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDeadAssignment)
	if diag.Position.Line != 2 {
		t.Fatalf("diagnostic line = %d, want original assignment line 2", diag.Position.Line)
	}
	requireEvidenceMessage(t, diag, `later assignment replaces "value" before the earlier value is read`)
}

func TestDeadAssignmentWarningIgnoresOverwriteInImpossibleBranch(t *testing.T) {
	result := Check(`
local value = "old"
local flag = "old"
if flag == "old" then
    if flag == "new" then
        value = "impossible"
    end
end
return value
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no dead-assignment diagnostic from an impossible overwrite", result.Diagnostics)
	}
}

func TestDeadAssignmentWarningTreatsReachableTypeofQueryAsRead(t *testing.T) {
	for _, src := range []string{
		`
local value = { status = "old" }
type Snapshot = typeof(value)
value = { status = "new" }
return value
`,
		`
local value = { status = "old" }
local function consume(input: typeof(value)): ()
end
value = { status = "new" }
return value, consume
`,
		`
local value = { status = "old" }
local function wrap(input)
    return input
end
type Snapshot = typeof(wrap(value))
value = { status = "new" }
return value
`,
	} {
		result := Check(src, WithDiagnosticRule(
			diagnostics.CodeDeadAssignment,
			diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
		))
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want no dead-assignment warning because reachable typeof(value) reads the first value", result.Diagnostics)
		}
	}
}

func TestDeadAssignmentWarningIgnoresTypeofQueryInImpossibleBranch(t *testing.T) {
	result := Check(`
local value = "old"
local item = { kind = "ready" }
if item.kind == "ready" then
    if item.kind == "other" then
        type Snapshot = typeof(value)
    end
end
value = "new"
return value
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDeadAssignment)
	if diag.Position.Line != 2 {
		t.Fatalf("diagnostic line = %d, want original assignment line 2", diag.Position.Line)
	}
	requireEvidenceMessage(t, diag, `later assignment replaces "value" before the earlier value is read`)
}

func TestDeadAssignmentWarningRendersImpossibleBranchProofClearly(t *testing.T) {
	result := Check(`
local value = "old"
local flag = "old"
if flag == "old" then
    if flag == "new" then
        local observed = value
    end
end
value = "new"
return value
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDeadAssignment)
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources: diagnostic.SourceMap{
			"test.lua": `
local value = "old"
local flag = "old"
if flag == "old" then
    if flag == "new" then
        local observed = value
    end
end
value = "new"
return value
`,
		},
	})
	for _, want := range []string{
		"hint[lint.dead.assignment]: assignment to \"value\" is overwritten before it is read",
		"2 | local value = \"old\"",
		"9 | value = \"new\"",
		"later assignment replaces \"value\" before the earlier value is read",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", want, rendered)
		}
	}
	for _, reject := range []string{
		"observed",
		"↑ dead assignment",
		"^ overwrite",
	} {
		if strings.Contains(rendered, reject) {
			t.Fatalf("rendered diagnostic contains impossible-branch or verbose-label noise %q:\n%s", reject, rendered)
		}
	}
}

func TestDeadAssignmentWarningPreservesCaptureBeforeOverwriteConservatism(t *testing.T) {
	result := Check(`
local value = "old"
local observe = function()
    return value
end
value = "new"
return observe, value
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no dead-assignment warning when a closure capturing value exists before overwrite", result.Diagnostics)
	}
}

func TestDeadAssignmentWarningReportsOverwriteBeforeLaterCapture(t *testing.T) {
	result := Check(`
local value = "old"
value = "new"
local observe = function()
    return value
end
return observe()
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDeadAssignment)
	if diag.Position.Line != 2 {
		t.Fatalf("diagnostic line = %d, want original assignment line 2", diag.Position.Line)
	}
	requireEvidenceMessage(t, diag, `later assignment replaces "value" before the earlier value is read`)
}

func TestDeadAssignmentWarningReportsOverwriteBeforeLaterNestedCapture(t *testing.T) {
	result := Check(`
local value = "old"
value = "new"
local make_observer = function()
    return function()
        return value
    end
end
return make_observer()()
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDeadAssignment)
	if diag.Position.Line != 2 {
		t.Fatalf("diagnostic line = %d, want original assignment line 2", diag.Position.Line)
	}
	requireEvidenceMessage(t, diag, `later assignment replaces "value" before the earlier value is read`)
}

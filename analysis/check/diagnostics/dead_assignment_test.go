package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDeadAssignmentWarningIsOptInAndEvidenceBacked(t *testing.T) {
	result := runDiagnosticsResult(t, `
local value = 1
value = 2
return value
`)
	if diags := ProduceWithConfig(result, Config{}); len(diags) != 0 {
		t.Fatalf("default diagnostics = %#v, want dead-assignment disabled by default", diags)
	}

	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one dead-assignment warning", diags)
	}
	d := diags[0]
	if d.Code != CodeDeadAssignment || d.Severity != diagnostic.SeverityWarning {
		t.Fatalf("diagnostic code/severity = %s/%s, want %s/warning", d.Code, d.Severity, CodeDeadAssignment)
	}
	if !strings.Contains(d.Message, "overwritten") ||
		!diagnosticHasLabel(d, labelDeadAssignment) ||
		!diagnosticHasLabel(d, labelOverwrite) {
		t.Fatalf("diagnostic = %#v, want overwrite message with dead-write and overwrite labels", d)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 1 {
		t.Fatalf("evidence = %#v, want one overwrite evidence frame", evidence)
	}
	if evidence[0].Message != `later assignment replaces "value" before the earlier value is read` {
		t.Fatalf("evidence = %#v, want overwrite evidence", evidence)
	}
	if d.Help != "Remove this assignment, or read `value` before the later overwrite." {
		t.Fatalf("help = %q, want direct remediation", d.Help)
	}
}

func TestDeadAssignmentWarningCanBeSeverityRemapped(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
value = 2
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one dead-assignment diagnostic", diags)
	}
	if diags[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("severity = %s, want hint", diags[0].Severity)
	}
}

func TestDeadAssignmentWarningReportsAllArmOverwriteFrontier(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
if test then
	value = 2
else
	value = 3
end
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one all-arm dead-assignment warning", diags)
	}
	d := diags[0]
	if d.Code != CodeDeadAssignment ||
		!diagnosticHasLabel(d, labelDeadAssignment) ||
		diagnosticLabelCount(d, labelOverwrite) != 2 {
		t.Fatalf("diagnostic = %#v, want dead-write label and two overwrite labels", d)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 2 {
		t.Fatalf("evidence = %#v, want two replacement writes", evidence)
	}
}

func TestDeadAssignmentWarningReportsNestedAllArmOverwriteFrontier(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
if test then
	local other = test
	if other then
		value = 2
	else
		value = 3
	end
else
	value = 4
end
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one nested all-arm dead-assignment warning", diags)
	}
	if !diagnosticHasLabel(diags[0], labelDeadAssignment) ||
		diagnosticLabelCount(diags[0], labelOverwrite) != 3 {
		t.Fatalf("labels = %#v, want dead-write label and three overwrite labels", diags[0].Labels)
	}
	evidence := diags[0].Explanation.Evidence()
	if len(evidence) != 3 {
		t.Fatalf("evidence = %#v, want three replacement writes", evidence)
	}
}

func TestDeadAssignmentWarningReportsMixedBranchAndCommonOverwriteFrontier(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
if test then
	value = 2
end
value = 3
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %#v, want initial and branch overwrite warnings", diags)
	}
	d := diags[0]
	if !diagnosticHasLabel(d, labelDeadAssignment) ||
		diagnosticLabelCount(d, labelOverwrite) != 2 {
		t.Fatalf("first diagnostic labels = %#v, want dead-write label and two overwrite labels", d.Labels)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 2 {
		t.Fatalf("first diagnostic evidence = %#v, want two replacement writes", evidence)
	}
}

func TestDeadAssignmentWarningReportsOverwriteOrExitBeforeRead(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
if test then
	return
end
value = 2
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one mixed exit/overwrite dead-assignment warning", diags)
	}
	d := diags[0]
	if d.Code != CodeDeadAssignment || d.Message != `assignment to "value" is discarded before it is read` {
		t.Fatalf("diagnostic = %#v, want mixed overwrite/exit message", d)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 2 {
		t.Fatalf("evidence = %#v, want return exit and later assignment evidence", evidence)
	}
	if !diagnosticEvidenceContains(evidence, `later assignment replaces "value" before the earlier value is read`) ||
		!diagnosticEvidenceContains(evidence, `control can leave before "value" is read`) {
		t.Fatalf("evidence = %#v, want replacement and exit evidence", evidence)
	}
	if !diagnosticHasLabel(d, labelDeadAssignment) ||
		!diagnosticHasLabel(d, labelOverwrite) ||
		!diagnosticHasLabel(d, labelExitBeforeRead) {
		t.Fatalf("labels = %#v, want dead-write, overwrite, and exit labels", d.Labels)
	}
	if d.Help != "Remove this assignment, or read `value` before every later overwrite or exit." {
		t.Fatalf("help = %q, want mixed replacement/exit remediation", d.Help)
	}
}

func TestDeadAssignmentWarningRendersMixedOverwriteExitTrace(t *testing.T) {
	src := strings.TrimLeft(`
local value = 1
if test then
    return
end
value = 2
return value
`, "\n")
	diags := ProduceWithConfig(runDiagnosticsResult(t, src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one mixed exit/overwrite dead-assignment warning", diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.dead.assignment]: assignment to "value" is discarded before it is read
 --> diagnostics_test.lua:1:7
  |
1 | local value = 1
  |       ↑ dead assignment

because:
  1. proven: control can leave before "value" is read
 --> diagnostics_test.lua:3:5
  |
  |     ↓ exit before read
3 |     return
  2. proven: later assignment replaces "value" before the earlier value is read
 --> diagnostics_test.lua:5:1
  |
  | ↓ overwriting assignment
5 | value = 2

help: Remove this assignment, or read ` + "`value`" + ` before every later overwrite or exit.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestDeadAssignmentWarningDoesNotDuplicatePureExitWithoutReplacement(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
if test then
	return
end
return
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want pure never-read exit case left to unused-local/unused-write diagnostics", diags)
	}
}

func TestDeadAssignmentWarningRespectsReadsAndConditionalBypass(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "rhs self read",
			src: `
local value = 1
value = value + 1
return value
`,
		},
		{
			name: "intervening read",
			src: `
local value = 1
local seen = value
value = 2
return seen, value
`,
		},
		{
			name: "lvalue key read",
			src: `
local value = 1
local t = {}
t[value] = 2
value = 3
return value
`,
		},
		{
			name: "closure read between writes is conservative",
			src: `
local value = 1
local fn = function()
	return value
end
value = 2
return fn, value
`,
		},
		{
			name: "conditional overwrite can be bypassed",
			src: `
local value = 1
if test then
	value = 2
end
return value
`,
		},
		{
			name: "branch read before later overwrite",
			src: `
local value = 1
if test then
	local seen = value
end
value = 2
return value
`,
		},
		{
			name: "branch read before same-arm overwrite",
			src: `
local value = 1
local seen = nil
if test then
	seen = value
	value = 2
else
	value = 3
end
return seen, value
`,
		},
		{
			name: "nested branch read before same-leaf overwrite",
			src: `
local value = 1
local seen = nil
if test then
	if test then
		seen = value
		value = 2
	else
		value = 3
	end
else
	value = 4
end
return seen, value
`,
		},
		{
			name: "while overwrite can be skipped",
			src: `
local value = 1
while test do
	value = 2
end
return value
`,
		},
		{
			name: "repeat read before overwrite",
			src: `
local value = 1
repeat
	local seen = value
	value = 2
until test
return value
`,
		},
		{
			name: "repeat break before overwrite",
			src: `
local value = 1
repeat
	if test then
		break
	end
	value = 2
until test
return value
`,
		},
		{
			name: "goto bypasses overwrite candidate",
			src: `
local value = 1
goto done
value = 2
::done::
return value
`,
		},
		{
			name: "mutually exclusive writes",
			src: `
local value
if test then
	value = 1
else
	value = 2
end
return value
`,
		},
		{
			name: "same statement duplicate write is ambiguous",
			src: `
local value = 0
value, value = 1, 2
return value
`,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			diags := ProduceWithConfig(runDiagnosticsResult(t, tt.src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
				CodeDeadAssignment: diagnostic.Enable(),
			}}})
			if len(diags) != 0 {
				t.Fatalf("diagnostics = %#v, want no dead-assignment warning", diags)
			}
		})
	}
}

func TestDeadAssignmentWarningUsesCFGReachability(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
goto overwrite
do
	local seen = value
end
::overwrite::
value = 2
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one warning because the source-order read is unreachable", diags)
	}
	if diags[0].Code != CodeDeadAssignment {
		t.Fatalf("diagnostic = %#v, want dead-assignment warning", diags[0])
	}
}

func TestDeadAssignmentWarningReportsRepeatOverwrite(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local value = 1
repeat
	value = 2
until test
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want repeat body overwrite to be recognized as must-execute", diags)
	}
}

func TestDeadAssignmentWarningReportsParameterSlotWritesButSkipsGlobals(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
function update(value)
	value = 1
	value = 2
	return value
end
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("parameter diagnostics = %#v, want one overwritten parameter-slot write", diags)
	}

	globalDiags := ProduceWithConfig(runDiagnosticsResult(t, `
value = 1
value = 2
return value
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeDeadAssignment: diagnostic.Enable(),
	}}})
	if len(globalDiags) != 0 {
		t.Fatalf("global diagnostics = %#v, want globals skipped by the local dead-assignment pass", globalDiags)
	}
}

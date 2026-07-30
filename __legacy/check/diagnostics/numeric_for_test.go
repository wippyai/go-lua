package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestNumericForReportsStringInit(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = "one", 10 do
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeNumericForOperand || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "initial value") || !strings.Contains(d.Message, `"one"`) {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticHasLabel(d, "initial value") {
		t.Fatalf("labels = %#v, want initial-value focus label", d.Labels)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 1 || evidence[0].Message != `initial value has literal value "one"` {
		t.Fatalf("evidence = %#v, want one concrete operand-type fact", evidence)
	}
	if d.Help != "Use a number for the numeric for initial value, or convert it before the loop." {
		t.Fatalf("help = %q", d.Help)
	}
}

func TestNumericForDoesNotTrustExplicitAnyCastInit(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = ("one" :: any), 10 do
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want explicit-any numeric-for operand error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeNumericForOperand || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "initial value") || !strings.Contains(d.Message, `"one"`) {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticHasLabel(d, "initial value") {
		t.Fatalf("labels = %#v, want initial-value focus label", d.Labels)
	}
	if got := d.Explanation.String(); !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "assigned value comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path shows assigned value is number") {
		t.Fatalf("explanation = %q, want explicit-any boundary and missing-proof evidence", got)
	}
}

func TestNumericForReportsStringLimitAndStep(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = 1, "ten", "one" do
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeNumericForOperand || !strings.Contains(diags[0].Message, "limit") {
		t.Fatalf("first diagnostic = %#v, want limit numeric-for operand", diags[0])
	}
	if diags[1].Code != CodeNumericForOperand || !strings.Contains(diags[1].Message, "step") {
		t.Fatalf("second diagnostic = %#v, want step numeric-for operand", diags[1])
	}
}

func TestNumericForAcceptsNumbersAndDefaultStep(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = 1, 10 do
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestNumericForSkipsUnknownAndPartlyNumericUnion(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(value, mixed: number | string)
			for i = value, mixed do
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unknown and partly numeric union", diags)
	}
}

func TestNumericForReportsNonNumericUnionWithNeverArm(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(value: string | never)
			for i = value, 10 do
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want non-numeric reachable union arm error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeNumericForOperand || !strings.Contains(d.Message, "initial value") ||
		!strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want initial-value numeric-for operand error for string | never", d)
	}
}

func TestNumericForReportsNonNumericAliasOperands(t *testing.T) {
	diags := runDiagnostics(t, `
		type Label = string
		type MaybeLabel = Label?

		function f(init: Label, limit: MaybeLabel)
			for i = init, limit do
			end
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want aliased init and optional aliased limit errors: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeNumericForOperand || !strings.Contains(diags[0].Message, "initial value") {
		t.Fatalf("first diagnostic = %#v, want aliased initial-value numeric-for operand error", diags[0])
	}
	if diags[1].Code != CodeNumericForOperand || !strings.Contains(diags[1].Message, "limit") {
		t.Fatalf("second diagnostic = %#v, want optional aliased limit numeric-for operand error", diags[1])
	}
}

func TestNumericForSkipsPartlyNumericAliasUnion(t *testing.T) {
	diags := runDiagnostics(t, `
		type Counterish = number | string

		function f(value: Counterish)
			for i = value, 10 do
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want partly numeric alias union left to runtime", diags)
	}
}

func TestNumericForSkipsPureNeverOperandAsUnreachable(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(value: never)
			for i = value, 10 do
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want no numeric-for operand error for unreachable never operand", diags)
	}
}

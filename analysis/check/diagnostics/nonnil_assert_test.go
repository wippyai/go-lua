package diagnostics

import "testing"

func TestNonNilAssertOnProvablyNilReports(t *testing.T) {
	src := `local function f(): string
		local x: nil = nil
		return x!
	end`
	diags := runDiagnosticsWithGlobals(t, src, []string{"test", "type", "value"})
	if !containsDiagnosticMessage(diagnosticMessages(diags), "asserted non-nil") {
		t.Fatalf("expected non-nil assertion diagnostic, got %v", diagnosticMessages(diags))
	}
}

func TestNonNilAssertOnFlowNarrowedNilReports(t *testing.T) {
	src := `local function f(x: string?): string
		if x == nil then
			return x!
		end
		return x
	end`
	diags := runDiagnosticsWithGlobals(t, src, []string{"test", "type", "value"})
	if !containsDiagnosticMessage(diagnosticMessages(diags), "asserted non-nil") {
		t.Fatalf("expected non-nil assertion diagnostic, got %v", diagnosticMessages(diags))
	}
}

func TestNonNilAssertOnOptionalDoesNotReport(t *testing.T) {
	src := `local function f(x: string?): string
		return x!
	end`
	diags := runDiagnosticsWithGlobals(t, src, []string{"test", "type", "value"})
	if containsDiagnosticMessage(diagnosticMessages(diags), "asserted non-nil") {
		t.Fatalf("did not expect non-nil assertion diagnostic, got %v", diagnosticMessages(diags))
	}
}

func TestNonNilAssertOnConcreteValueDoesNotReport(t *testing.T) {
	src := `local function f(x: string): string
		return x!
	end`
	diags := runDiagnosticsWithGlobals(t, src, []string{"test", "type", "value"})
	if containsDiagnosticMessage(diagnosticMessages(diags), "asserted non-nil") {
		t.Fatalf("did not expect non-nil assertion diagnostic, got %v", diagnosticMessages(diags))
	}
}

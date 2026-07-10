package diagnostic

import "testing"

func TestPolicyAppliesPerCodeRules(t *testing.T) {
	input := []Diagnostic{
		{Code: Code("type.assignment"), Severity: SeverityError, Message: "bad assignment"},
		{Code: Code("lint.unused"), Severity: SeverityWarning, Message: "unused local"},
		{Code: Code("type.member.missing"), Severity: SeverityError, Message: "missing member"},
	}
	policy := Policy{Rules: map[Code]Rule{
		Code("type.assignment"): OverrideSeverity(SeverityHint),
		Code("lint.unused"):     Disable(),
	}}

	got := policy.Apply(input)
	if len(got) != 2 {
		t.Fatalf("Apply returned %d diagnostics, want 2: %#v", len(got), got)
	}
	if got[0].Code != Code("type.assignment") || got[0].Severity != SeverityHint {
		t.Fatalf("first diagnostic = %#v, want assignment remapped to hint", got[0])
	}
	if got[1].Code != Code("type.member.missing") || got[1].Severity != SeverityError {
		t.Fatalf("second diagnostic = %#v, want unchanged missing-member error", got[1])
	}
	if input[0].Severity != SeverityError {
		t.Fatalf("Apply mutated input severity to %s", input[0].Severity)
	}
}

func TestPolicyRuleCanDisableAndCarrySeverity(t *testing.T) {
	rule := Disable().WithSeverity(SeverityHint)
	got, ok := (Policy{Rules: map[Code]Rule{Code("lint.dead_assignment"): rule}}).ApplyOne(Diagnostic{
		Code:     Code("lint.dead_assignment"),
		Severity: SeverityWarning,
	})
	if ok {
		t.Fatalf("ApplyOne returned %#v/true, want disabled diagnostic", got)
	}
}

func TestPolicyEnabledSupportsOptInCodes(t *testing.T) {
	code := Code("lint.unused.local")
	if (Policy{}).Enabled(code, false) {
		t.Fatal("zero policy enabled opt-in code")
	}
	if !(Policy{Rules: map[Code]Rule{code: Enable()}}).Enabled(code, false) {
		t.Fatal("Enable rule did not enable opt-in code")
	}
	if (Policy{Rules: map[Code]Rule{code: Disable()}}).Enabled(code, true) {
		t.Fatal("Disable rule did not disable default-enabled code")
	}
}

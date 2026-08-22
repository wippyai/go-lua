package diagnostic

import (
	"testing"

	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/domain/composite"
)

// The advisory-tier gating laws.
//
// The advisory tier is advice a consumer opts into, and the error tier is the
// analyzer's own verdict on a program. The tier is derived from the declared
// default severity rather than declared beside it, so the config gate and the
// severity a reader sees cannot disagree.
//
// What makes the tier config-gated is the policy: it is an allow-list, so a
// consumer that names nothing receives nothing. These laws state that gate
// over the sealed table itself rather than over a copied list of codes, so a
// row added to the advisory tier is gated the moment it is declared.

// advisoryTierRows partitions the sealed declaration table by publication
// tier. Nothing here spells a code: the partition is the table's own.
func advisoryTierRows(t *testing.T, table schemadiag.Table) (advisory, errors []*schemadiag.Entry) {
	t.Helper()
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK {
			t.Fatalf("sealed declaration row %d unavailable", position)
		}
		switch entry.Tier() {
		case schemadiag.TierAdvisory:
			advisory = append(advisory, entry)
		case schemadiag.TierError:
			errors = append(errors, entry)
		default:
			t.Fatalf("row %q publishes no tier", entry.Code().String())
		}
	}
	if len(advisory) == 0 || len(errors) == 0 {
		t.Fatalf("sealed table partitions into %d advisory and %d error rows; both tiers must be populated for this law to say anything", len(advisory), len(errors))
	}
	return advisory, errors
}

// TestAdvisoryTierIsOffUntilNamedLaw states the default: a policy that names
// nothing enables nothing. This is what "default-off" means mechanically - the
// tier is not suppressed by a filter that could be forgotten, it is simply
// never reached by a consumer that did not ask for it.
func TestAdvisoryTierIsOffUntilNamedLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	advisory, errorTier := advisoryTierRows(t, fixture.declarations)
	silent := DiagnosticPolicy{}
	for _, entry := range append(append([]*schemadiag.Entry(nil), advisory...), errorTier...) {
		if severity, enabled := silent.EnabledFor(fixture.declarations, entry.Code()); enabled {
			t.Fatalf("empty policy enabled %q at severity %d", entry.Code().String(), severity)
		}
	}
}

// TestErrorTierPolicyEnablesNoAdviceLaw states the separation the two tiers
// exist for: a consumer that asked for the analyzer's verdicts and no advice
// receives no advice. A tier that leaked into an error-only policy would make
// the opt-in decorative.
func TestErrorTierPolicyEnablesNoAdviceLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	advisory, errorTier := advisoryTierRows(t, fixture.declarations)
	policy := DiagnosticPolicy{}
	for _, entry := range errorTier {
		policy.Enabled = append(policy.Enabled, entry.Code())
	}
	for _, entry := range advisory {
		if _, enabled := policy.EnabledFor(fixture.declarations, entry.Code()); enabled {
			t.Fatalf("error-tier policy enabled advisory code %q", entry.Code().String())
		}
	}
	for _, entry := range errorTier {
		severity, enabled := policy.EnabledFor(fixture.declarations, entry.Code())
		if !enabled || severity != entry.DefaultSeverity() {
			t.Fatalf("error-tier policy did not enable %q at its declared severity: enabled=%t severity=%d", entry.Code().String(), enabled, severity)
		}
	}
}

// TestAdvisoryTierTogglesOnByNameLaw states the other half: naming an advisory
// code enables it, at the severity its row declares. The tier is a default a
// consumer opts into, never a set of codes the analyzer withholds.
func TestAdvisoryTierTogglesOnByNameLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	advisory, _ := advisoryTierRows(t, fixture.declarations)
	for _, entry := range advisory {
		policy := DiagnosticPolicy{Enabled: []DiagnosticCode{entry.Code()}}
		severity, enabled := policy.EnabledFor(fixture.declarations, entry.Code())
		if !enabled || severity != entry.DefaultSeverity() {
			t.Fatalf("named advisory code %q was not enabled at its declared severity: enabled=%t severity=%d want=%d", entry.Code().String(), enabled, severity, entry.DefaultSeverity())
		}
		if !severity.Available() || severity == FindingSeverityError {
			t.Fatalf("advisory code %q defaults to severity %d, which is not an advisory severity", entry.Code().String(), severity)
		}
	}
}

// TestAdvisorySeverityRefinementStaysDeclaredLaw states the limit of the
// config gate: a policy may move an enabled row within the declared severity
// vocabulary and can never invent a level outside it.
func TestAdvisorySeverityRefinementStaysDeclaredLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	advisory, _ := advisoryTierRows(t, fixture.declarations)
	entry := advisory[0]
	refined := DiagnosticPolicy{
		Enabled:  []DiagnosticCode{entry.Code()},
		Severity: map[DiagnosticCode]FindingSeverity{entry.Code(): FindingSeverityWarning},
	}
	severity, enabled := refined.EnabledFor(fixture.declarations, entry.Code())
	if !enabled || severity != FindingSeverityWarning {
		t.Fatalf("declared severity refinement of %q lost: enabled=%t severity=%d", entry.Code().String(), enabled, severity)
	}
	invented := DiagnosticPolicy{
		Enabled:  []DiagnosticCode{entry.Code()},
		Severity: map[DiagnosticCode]FindingSeverity{entry.Code(): FindingSeverityInvalid},
	}
	if _, enabled := invented.EnabledFor(fixture.declarations, entry.Code()); enabled {
		t.Fatalf("policy enabled %q at a severity outside the declared vocabulary", entry.Code().String())
	}
}

// TestDeclaredNotComposedCodeIsNeverEnabledLaw states the gate that keeps
// silence from being an answer. A code the register names has no producer, so
// naming it in a policy must not yield a clean empty report for a family
// nothing collected: the collectable gate refuses it, and the register says
// which surface owes the judgment.
func TestDeclaredNotComposedCodeIsNeverEnabledLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	declared := composite.DiagnosticsDeclaredNotComposed()
	if len(declared) == 0 {
		t.Fatal("declared-not-composed register is empty; this law has nothing to state")
	}
	for _, row := range declared {
		code := DiagnosticCode(row.Code)
		if diagnosticCollectable(fixture.declarations, code) {
			t.Fatalf("register names %q as uncomposed while the sealed table installs a producer for it", row.Code.String())
		}
		status, answer := composite.DiagnosticCodeAnswer(fixture.compilation, row.Code)
		if status != composite.DiagnosticCodeDeclared || answer.Owner == "" || answer.Reason == "" {
			t.Fatalf("register row %q answers status=%d owner=%q reason=%q", row.Code.String(), status, answer.Owner, answer.Reason)
		}
	}
}

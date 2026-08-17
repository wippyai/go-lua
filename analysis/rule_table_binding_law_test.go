package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// TestProgramBindingDerivesEveryRuleFromTheTable is the hot-side drift law.
// The binding sequence is one pass over the sealed rule table, so every rule
// the table declares is bound, registered, and reachable by its role, and no
// role outside the table has a capability. A rule added to the table alone is
// therefore fully wired.
func TestProgramBindingDerivesEveryRuleFromTheTable(t *testing.T) {
	plan, _, compileDiagnostics := fixtureCompile(t, "advice/always-true-guard")
	if plan.state == nil || plan.state.binding == nil || plan.state.binding.Rules() == nil {
		t.Fatalf("rule table binding compile diagnostics=%+v", compileDiagnostics)
	}
	binding := plan.state.binding
	if composite.RuleCount() == 0 {
		t.Fatal("rule table published no rules")
	}
	links := make(map[programartifact.RuleRole]bool, len(composite.LinkRoles()))
	for _, role := range composite.LinkRoles() {
		links[role] = true
	}
	for position := 0; position < composite.RuleCount(); position++ {
		role, roleOK := composite.RuleRoleAt(position)
		if !roleOK {
			t.Fatalf("table position %d has no role", position)
		}
		capability, capabilityOK := binding.Rules().Capability(role)
		if !capabilityOK {
			t.Fatalf("role %d bound no sealed capability", role)
		}
		if capability.Link() != links[role] || capability.Mounted() == links[role] {
			t.Fatalf("role %d bound the wrong lane: mounted=%t link=%t", role, capability.Mounted(), capability.Link())
		}
		// The capability inverse is the same table: a bound capability
		// classifies back to exactly the rule that owns it.
		if got := binding.Rules().DiagnosticForCapability(capability); got != composite.DiagnosticRuleForRole(role) {
			t.Fatalf("capability of role %d classified as %s", role, got)
		}
		if links[role] {
			if _, catalogOK := binding.Rules().LinkCatalog(role); !catalogOK {
				t.Fatalf("link role %d published no occurrence catalog", role)
			}
			continue
		}
		if _, catalogOK := binding.Rules().LinkCatalog(role); catalogOK {
			t.Fatalf("mounted role %d published a link occurrence catalog", role)
		}
	}
	if _, ok := binding.Rules().Capability(programartifact.RuleRoleInvalid); ok {
		t.Fatal("the invalid role resolved a capability")
	}
	if binding.Rules().Attach(programartifact.RuleRoleInvalid, nil, plan.state.sourceID, plan.state.sourceID, plan.state.sourceID) {
		t.Fatal("the invalid role admitted an occurrence")
	}
}

// TestProgramBindingFailureNamesItsRuleFromTheTable states that the binding
// boundary spells a rule failure with the table's own name rather than a
// parallel enum arm.
func TestProgramBindingFailureNamesItsRuleFromTheTable(t *testing.T) {
	for position := 0; position < composite.RuleCount(); position++ {
		role, roleOK := composite.RuleRoleAt(position)
		if !roleOK {
			t.Fatalf("table position %d has no role", position)
		}
		diagnostic := composite.DiagnosticRuleForRole(role)
		failure := programBindingFailureForRule(diagnostic)
		if failure == ProgramBindingFailureNone {
			t.Fatalf("role %d has no binding failure ordinal", role)
		}
		if failure.String() != "rule/"+diagnostic.String() {
			t.Fatalf("binding failure of role %d = %q, want the table name", role, failure.String())
		}
	}
	if ProgramBindingFailureNone.String() != "none" || ProgramBindingFailureInput.String() != "input" {
		t.Fatal("the binding boundary lost its own phase names")
	}
}

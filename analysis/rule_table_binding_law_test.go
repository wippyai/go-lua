package analysis

import (
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/composite"
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
	links := make(map[schema.Key]bool, len(composite.LinkKeys()))
	for _, key := range composite.LinkKeys() {
		links[key] = true
	}
	for position := 0; position < composite.RuleCount(); position++ {
		key, keyOK := composite.RuleKeyAt(position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		capability, capabilityOK := binding.Rules().CapabilityByKey(key)
		if !capabilityOK {
			t.Fatalf("key %q bound no sealed capability", key)
		}
		if capability.Link() != links[key] || capability.Mounted() == links[key] {
			t.Fatalf("key %q bound the wrong lane: mounted=%t link=%t", key, capability.Mounted(), capability.Link())
		}
		if got := binding.Rules().DiagnosticForCapability(capability); got != composite.DiagnosticRuleForKey(key) {
			t.Fatalf("capability of key %q classified as %s", key, got)
		}
		if links[key] {
			if _, catalogOK := binding.Rules().LinkCatalogByKey(key); !catalogOK {
				t.Fatalf("link key %q published no occurrence catalog", key)
			}
			continue
		}
		if _, catalogOK := binding.Rules().LinkCatalogByKey(key); catalogOK {
			t.Fatalf("mounted key %q published a link occurrence catalog", key)
		}
	}
	if _, ok := binding.Rules().CapabilityByKey(""); ok {
		t.Fatal("the empty key resolved a capability")
	}
	if program, ok := binding.Rules().ProgramRuleByKey(""); ok || program.Available() {
		t.Fatal("the empty key published a construction primitive")
	}
}

// TestProgramBindingFailureNamesItsRuleFromTheTable states that the binding
// boundary spells a rule failure with the table's own name rather than a
// parallel enum arm.
func TestProgramBindingFailureNamesItsRuleFromTheTable(t *testing.T) {
	for position := 0; position < composite.RuleCount(); position++ {
		key, keyOK := composite.RuleKeyAt(position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		diagnostic := composite.DiagnosticRuleForKey(key)
		failure := anadiag.ProgramBindingFailureForRule(diagnostic)
		if failure == anadiag.ProgramBindingFailureNone {
			t.Fatalf("key %q has no binding failure ordinal", key)
		}
		if failure.String() != "rule/"+diagnostic.String() {
			t.Fatalf("binding failure of key %q = %q, want the table name", key, failure.String())
		}
	}
	if anadiag.ProgramBindingFailureNone.String() != "none" || anadiag.ProgramBindingFailureInput.String() != "input" {
		t.Fatal("the binding boundary lost its own phase names")
	}
}

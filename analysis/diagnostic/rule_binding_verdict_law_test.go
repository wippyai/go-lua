package diagnostic

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
)

// A per-rule binding verdict exists to say which rule refused. The rule table
// owns both the ordinal and the name, so a verdict carrying an ordinal renders
// the key that rule was declared under. The laws below state that: no declared
// rule collapses onto the anonymous verdict, and nothing but the absent rule
// renders as unknown.

// TestPerRuleBindingVerdictNamesItsRule states the naming half. Every rule the
// sealed table declares projects onto a distinct per-rule verdict spelled as
// its owner declared it.
func TestPerRuleBindingVerdictNamesItsRule(t *testing.T) {
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	count := composite.RuleCount(compilation)
	if count == 0 {
		t.Fatal("the sealed table declares no rule; the law measures nothing")
	}
	seen := make(map[ProgramBindingFailure]struct{}, count)
	for position := 0; position < count; position++ {
		key, keyOK := composite.RuleKeyAt(compilation, position)
		if !keyOK {
			t.Fatalf("rule position %d publishes no key", position)
		}
		classification := composite.DiagnosticRuleForKey(compilation, key)
		if classification == composite.DiagnosticRuleUnknown {
			t.Fatalf("rule %q classifies as unknown", key)
		}
		verdict := ProgramBindingFailureFromBind(composite.BindFailure{Stage: composite.BindStageRule, Rule: classification})
		if verdict != ProgramBindingFailureForRule(classification) {
			t.Fatalf("rule %q projects onto verdict %q", key, verdict)
		}
		if got, want := verdict.String(), "rule/"+string(key); got != want {
			t.Fatalf("the verdict for rule %q renders as %q", key, got)
		}
		if _, duplicate := seen[verdict]; duplicate {
			t.Fatalf("rule %q shares its verdict with an earlier rule", key)
		}
		seen[verdict] = struct{}{}
	}
}

// TestOnlyTheAbsentRuleRendersUnknown states the converse. A refusal that
// reached a rule always publishes that rule's ordinal, so an ordinal that names
// no table row still spells itself rather than erasing the evidence; only the
// absent rule is unknown.
func TestOnlyTheAbsentRuleRendersUnknown(t *testing.T) {
	if got := composite.DiagnosticRuleUnknown.String(); got != "unknown" {
		t.Fatalf("the absent rule renders as %q", got)
	}
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	beyond := composite.DiagnosticRule(composite.RuleCount(compilation) + 1)
	got := beyond.String()
	if got == "unknown" {
		t.Fatalf("rule ordinal %d beyond the sealed table erased its ordinal", beyond)
	}
	if !strings.Contains(got, "#") {
		t.Fatalf("rule ordinal %d renders as %q, which names no ordinal", beyond, got)
	}
}

// TestRuleLessBindingRefusalIsNotAPerRuleVerdict states the boundary half: a
// pass that never reached a rule must not be published as a rule refusal,
// because such a verdict would carry the anonymous ordinal and name nothing.
func TestRuleLessBindingRefusalIsNotAPerRuleVerdict(t *testing.T) {
	for _, stage := range []composite.BindStage{composite.BindStageTable, composite.BindStageSeal, composite.BindStageInput} {
		verdict := ProgramBindingFailureFromBind(composite.BindFailure{Stage: stage})
		if verdict >= ProgramBindingFailureForRule(composite.DiagnosticRuleUnknown) {
			t.Fatalf("bind stage %q projected onto the per-rule tail as %q", stage, verdict)
		}
	}
}

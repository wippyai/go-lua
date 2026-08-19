package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
)

// The helpers below recover one rule's own implementation from the sealed
// table so a domain law can state itself against the exact bound rule.
// Production wiring drives the table and never needs them.
//
// MISSING ISSUANCE: domain/composite publishes no typed handle for a bound
// rule. The generic accessor that resolved one (RuleHandleByKey over the
// binding's own rule cell) was removed with the construction transaction, and
// nothing replaced it: RuleBinding publishes the erased engine.ProgramRule and
// the slot capability only. callsite.HotRule.MountedPublicationCandidates and
// MountedSelectedCallEffectStage are therefore unreachable from any package
// that can assemble a committed program - they have no production caller
// either - so every law below cannot be stated until composite republishes a
// typed rule handle.
func selectedEffectRule(t testing.TB, binding *composite.ProgramBinding) *callsite.HotRule {
	t.Helper()
	return callsiteEffectRule(t, binding, "effect-selected")
}

func opaqueEffectRule(t testing.TB, binding *composite.ProgramBinding) *callsite.HotRule {
	t.Helper()
	return callsiteEffectRule(t, binding, "effect-opaque")
}

func bodyEffectRule(t testing.TB, binding *composite.ProgramBinding) *callsite.BodyHotRule {
	t.Helper()
	if binding == nil || binding.Rules() == nil {
		t.Fatal("sealed rule binding")
	}
	t.Fatal("domain/composite publishes no typed rule handle: the effect-body rule cannot be reached to state this law")
	return nil
}

func callsiteEffectRule(t testing.TB, binding *composite.ProgramBinding, key string) *callsite.HotRule {
	t.Helper()
	if binding == nil || binding.Rules() == nil {
		t.Fatal("sealed rule binding")
	}
	t.Fatalf("domain/composite publishes no typed rule handle: the %q rule cannot be reached to state this law", key)
	return nil
}

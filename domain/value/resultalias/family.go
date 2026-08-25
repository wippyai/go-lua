package resultalias

import (
	"github.com/wippyai/go-lua/analysis/engine"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only the three peer types this package already speaks, so
// the composition that supplies the authority record satisfies it structurally
// and neither side learns the other's shape.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PackSchema() *packdomain.Schema
}

// InstallFamily is the generated lane's bind arm for this rule. It resolves
// the three cold schemas the declaration names, proves the two owned ones were
// issued by this binding, and claims the rule's sealed ordinal against the
// Value Factor it writes.
//
// The one fence that lives here rather than in the fold is that both owners
// belong to THIS binding. It is answered once, at install, because it is a
// property of the composition and not of any invocation; the sealed judgment
// then carries the Link agreement the three schemas are admissible under.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	if binding == nil || slot == nil || !slot.Available() {
		return false
	}
	values, calls, packs := authorities.ValueAuthority(), authorities.CallAuthority(), authorities.PackSchema()
	if values == nil || calls == nil || packs == nil || !values.MatchesBinding(binding) || !calls.MatchesBinding(binding) {
		return false
	}
	installer, installerOK := NewFamilyInstaller(values.Schema(), calls.Algebra(), packs)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[valuedomain.DenseCoordinate](binding, slot, values.FactorRef(), installer)
}

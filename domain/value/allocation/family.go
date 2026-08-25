package allocation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only the one peer type this package already speaks, so the
// composition that supplies the authority record satisfies it structurally and
// neither side learns the other's shape.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
}

// InstallFamily is the generated lane's bind arm for this rule. It resolves the
// one cold schema the declaration names, proves that owner was issued by this
// binding, and claims the rule's sealed ordinal against the Value Factor it
// writes.
//
// The one fence that lives here rather than in the fold is that the owner
// belongs to THIS binding. It is answered once, at install, because it is a
// property of the composition and not of any invocation; the sealed judgment
// then carries the schema every receipt it answers for is authenticated
// against.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	if binding == nil || slot == nil || !slot.Available() {
		return false
	}
	values := authorities.ValueAuthority()
	if values == nil || !values.MatchesBinding(binding) {
		return false
	}
	installer, installerOK := NewFamilyInstaller(values.Schema())
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[valuedomain.DenseCoordinate](binding, slot, values.FactorRef(), installer)
}

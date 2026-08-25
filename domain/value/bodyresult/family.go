package bodyresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only the two peer types this package already speaks, so
// the composition that supplies the authority record satisfies it structurally
// and neither side learns the other's shape.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
}

// InstallFamily is the generated lane's bind arm for this rule. It resolves
// the two cold schemas the declaration names, proves both were issued by this
// binding, and claims the rule's sealed ordinal against the Value Factor it
// writes.
//
// The one fence that lives here rather than in the fold is that both owners
// belong to THIS binding. It is answered once, at install, because it is a
// property of the composition and not of any invocation; the sealed judgment
// then carries the Link agreement the two schemas are admissible under.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	if binding == nil || slot == nil || !slot.Available() {
		return false
	}
	values, calls := authorities.ValueAuthority(), authorities.CallAuthority()
	if values == nil || calls == nil || !values.MatchesBinding(binding) || !calls.MatchesBinding(binding) {
		return false
	}
	installer, installerOK := NewFamilyInstaller(values.Schema(), calls.Algebra())
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[valuedomain.DenseCoordinate](binding, slot, values.FactorRef(), installer)
}

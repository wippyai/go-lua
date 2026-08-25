package publicationfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only peer types this package already speaks, so the
// composition that supplies the authority record satisfies it structurally and
// neither side learns the other's shape.
type ruleAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
	ValueSchema() *valuedomain.Schema
	PackSchema() *packdomain.Schema
}

// InstallFamily is the generated lane's bind arm for this rule. It resolves
// the five sealed schemas the emitted installer is sealed against, proves they
// were issued for this binding and agree on one Link, and claims the rule's
// sealed ordinal against the Heap Factor it writes.
//
// This is the whole of what a family cutover still authors: how an authority
// record is reached is the composition's knowledge and not the rule's, so it
// cannot be a function of the declaration the family is emitted from.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	if binding == nil || slot == nil || !slot.Available() {
		return false
	}
	owner, values, calls, effects := authorities.HeapAuthority(), authorities.ValueAuthority(), authorities.CallAuthority(), authorities.EffectAuthority()
	if owner == nil || values == nil || calls == nil || effects == nil ||
		!owner.MatchesBinding(binding) || !values.MatchesBinding(binding) ||
		!calls.MatchesBinding(binding) || !effects.MatchesBinding(binding) {
		return false
	}
	valueSchema, packSchema := authorities.ValueSchema(), authorities.PackSchema()
	if valueSchema == nil || packSchema == nil || values.Schema() != valueSchema ||
		!owner.Schema().Valid() || !valueSchema.Valid() ||
		calls.Algebra() == nil || !calls.Algebra().Valid() ||
		effects.Algebra() == nil || !effects.Algebra().Valid() ||
		!valueSchema.OwnsHeapSchema(owner.Schema()) {
		return false
	}
	// The one fence that lives here rather than in the fold is that every
	// owner belongs to THIS Link. It is answered once, at install, because it
	// is a property of the composition and not of any invocation.
	linkOwner := calls.Algebra().LinkOwner()
	if !linkOwner.Available() || !valueSchema.LinkOwner().Matches(linkOwner) ||
		!owner.Schema().LinkOwner().Matches(linkOwner) ||
		!effects.Algebra().LinkOwner().Matches(linkOwner) ||
		!packSchema.LinkOwner().Matches(linkOwner) {
		return false
	}
	installer, installerOK := NewFamilyInstaller(owner.Schema(), calls.Algebra(), valueSchema, effects.Algebra(), packSchema)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[heapdomain.DenseCoordinate](binding, slot, owner.FactorRef(), installer)
}

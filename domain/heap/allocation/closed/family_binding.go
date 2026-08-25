package closed

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// familyAuthorities is this package's own statement of the authorities its
// emitted family binds against: the axis it writes and the axis its operand
// vector is read from. It names only peer types this package already speaks,
// so the composition that supplies the record satisfies it structurally and
// neither side learns the other's shape.
type familyAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
	ValueAuthority() *valueowner.HotOwner
}

// InstallFamily is the generated lane's bind arm for this rule. It resolves
// the two axis schemas the emitted installer is sealed against and claims the
// rule's sealed ordinal against the Factor it writes.
//
// This is the whole of what the cutover authors: how an authority record is
// reached is the composition's knowledge and not the rule's, so it cannot be a
// function of the declaration the family is emitted from.
func InstallFamily[A familyAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	heaps := authorities.HeapAuthority()
	values := authorities.ValueAuthority()
	if heaps == nil || values == nil || !heaps.Schema().Valid() {
		return false
	}
	installer, installerOK := NewFamilyInstaller(heaps.Schema(), values.Schema())
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[heapdomain.DenseCoordinate](binding, slot, heaps.FactorRef(), installer)
}

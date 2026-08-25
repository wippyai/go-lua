package empty

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// familyAuthorities is this package's own statement of the authority its
// emitted family binds against: the axis it writes. It names only a peer type
// this package already speaks, so the composition that supplies the record
// satisfies it structurally and neither side learns the other's shape.
type familyAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
}

// InstallFamily is the generated lane's bind arm for this rule. It resolves the
// one axis schema the emitted installer is sealed against and claims the rule's
// sealed ordinal against the Factor it writes.
//
// This is the whole of what the cutover authors: how an authority record is
// reached is the composition's knowledge and not the rule's, so it cannot be a
// function of the declaration the family is emitted from.
func InstallFamily[A familyAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	owner := authorities.HeapAuthority()
	if owner == nil || !owner.Schema().Valid() {
		return false
	}
	installer, installerOK := NewFamilyInstaller(owner.Schema())
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[heapdomain.DenseCoordinate](binding, slot, owner.FactorRef(), installer)
}

package formal

import (
	"github.com/wippyai/go-lua/analysis/engine"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only peer types this package already speaks, so the
// composition that supplies the authority record satisfies it structurally
// and neither side learns the other's shape.
type ruleAuthorities interface {
	PlacementAuthority() *placementowner.HotOwner
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PackSchema() *packdomain.Schema
}

// InstallFamily is Formal's one generated RuleFamily claimant. It
// authenticates the schemas the emitted installer is sealed against against
// the mounted authorities that issued them, then claims this rule's sealed
// ordinal on the Factor it writes.
//
// The Link fence is Call's: Value, Pack and the Target Contract are all
// admitted only through the Link owner Call's algebra names, which is the one
// join the formal reduction reads its ownership rows under.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	placement := authorities.PlacementAuthority()
	values := authorities.ValueAuthority()
	calls := authorities.CallAuthority()
	packs := authorities.PackSchema()
	if placement == nil || values == nil || calls == nil || packs == nil ||
		!placement.MatchesBinding(binding) || !values.MatchesBinding(binding) || !calls.MatchesBinding(binding) {
		return false
	}
	placementSchema := placement.Schema()
	valueSchema := values.Schema()
	algebra := calls.Algebra()
	if !placementSchema.Valid() || valueSchema == nil || !valueSchema.Valid() || algebra == nil || !algebra.Valid() ||
		!valueSchema.OwnsHeapSchema(placementSchema.Heap()) || !valueSchema.LinkOwner().Matches(algebra.LinkOwner()) ||
		!packs.LinkOwner().Available() || !packs.LinkOwner().Matches(algebra.LinkOwner()) {
		return false
	}
	installer, installerOK := NewFamilyInstaller(placementSchema, algebra, valueSchema, packs)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[placementdomain.DenseCoordinate](binding, slot, placement.FactorRef(), installer)
}

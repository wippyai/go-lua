package returnescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only peer types this package already speaks, so the
// composition that supplies the authority record satisfies it structurally and
// neither side learns the other's shape.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	ValueSchema() *valuedomain.Schema
	PlacementSchema() placementdomain.Schema
}

// InstallFamily is ReturnEscape's one generated RuleFamily claimant. It
// authenticates the two schemas the emitted installer is sealed against
// against the mounted authorities that issued them, then claims this rule's
// sealed ordinal on the Factor it writes.
//
// This is the whole of what a family cutover still authors. How an authority
// record is reached, and which issuer a schema must match, are the
// composition's knowledge and not the rule's, so neither is a function of the
// declaration the family is emitted from.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	placement := authorities.PlacementAuthority()
	values := authorities.ValueAuthority()
	placementSchema := authorities.PlacementSchema()
	valueSchema := authorities.ValueSchema()
	if placement == nil || values == nil || valueSchema == nil || !placementSchema.Valid() || !valueSchema.Valid() ||
		!placement.Schema().Valid() || !values.Schema().Valid() ||
		placementSchema.ContentID() != placement.Schema().ContentID() || values.Schema() != valueSchema {
		return false
	}
	installer, installerOK := NewFamilyInstaller(placementSchema, valueSchema)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[placementdomain.DenseCoordinate](binding, slot, placement.FactorRef(), installer)
}

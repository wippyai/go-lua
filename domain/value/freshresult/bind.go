package freshresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only peer types this package already speaks, so the
// composition that supplies the authority record satisfies it structurally and
// neither side learns the other's shape.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	ValueSchema() *valuedomain.Schema
}

// InstallFamily is the fresh-result rule's one generated RuleFamily claimant.
// It authenticates the two schemas the emitted installer is sealed against
// against the mounted authorities that issued them, then claims this rule's
// sealed ordinal on the Factor it writes.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	values := authorities.ValueAuthority()
	calls := authorities.CallAuthority()
	valueSchema := authorities.ValueSchema()
	if values == nil || calls == nil || valueSchema == nil {
		return false
	}
	// The Call algebra is the authority's own; asking the authority record for
	// a second handle on it would be a second statement of which algebra this
	// Link sealed.
	callAlgebra := calls.Algebra()
	if callAlgebra == nil || !valueSchema.Valid() || !callAlgebra.Valid() || values.Schema() != valueSchema {
		return false
	}
	installer, installerOK := NewFamilyInstaller(valueSchema, callAlgebra)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[valuedomain.DenseCoordinate](binding, slot, values.FactorRef(), installer)
}

package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type ruleAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PackSchema() *packdomain.Schema
}

// InstallFamily is the freeze rule's one generated RuleFamily claimant. It
// authenticates the four schemas the emitted installer is sealed against
// against the mounted authorities that issued them, then claims this rule's
// sealed ordinal on the Factor it writes.
//
// This is the whole of what a family cutover still authors. How an authority
// record is reached, and which issuer a schema must match, are the
// composition's knowledge and not the rule's.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	heapAuthority := authorities.HeapAuthority()
	values := authorities.ValueAuthority()
	calls := authorities.CallAuthority()
	packs := authorities.PackSchema()
	if heapAuthority == nil || values == nil || calls == nil || packs == nil ||
		!heapAuthority.MatchesBinding(binding) || !values.MatchesBinding(binding) || !calls.MatchesBinding(binding) {
		return false
	}
	heapSchema := heapAuthority.Schema()
	valueSchema := values.Schema()
	algebra := calls.Algebra()
	if !heapSchema.Valid() || valueSchema == nil || !valueSchema.Valid() || algebra == nil || !algebra.Valid() ||
		!valueSchema.OwnsHeapSchema(heapSchema) || !valueSchema.LinkOwner().Matches(algebra.LinkOwner()) ||
		!packs.LinkOwner().Available() || !packs.LinkOwner().Matches(algebra.LinkOwner()) {
		return false
	}
	installer, installerOK := NewFamilyInstaller(heapSchema, algebra, valueSchema, packs)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[heapdomain.DenseCoordinate](binding, slot, heapAuthority.FactorRef(), installer)
}

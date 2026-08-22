package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// FactorSlotCapability is the sealed binding's authority for one canonical
// Factor. It lets a mounted artifact name a transported Factor directly,
// without selecting an arbitrary Rule that happens to write that Factor and
// later recovering the Factor from the Rule again.
type FactorSlotCapability struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	ordinal   uint64
	available bool
}

// Available reports whether this capability names a Factor row of the exact
// sealed binding that issued it. The verdict is sealed by the issuer: a sealed
// binding is terminal, so the row geometry the issuer proved cannot change
// underneath the capability afterwards.
func (capability FactorSlotCapability) Available() bool { return capability.available }

func (capability FactorSlotCapability) completeRow() bool {
	if capability.state == nil || capability.authority == nil ||
		capability.state.phase != schemaBindingSealed ||
		capability.state.authority != capability.authority ||
		capability.state.schema == nil || capability.ordinal >= uint64(len(capability.state.factors)) {
		return false
	}
	factor := capability.state.factors[capability.ordinal]
	return factorRowAvailable(factor) && factor.schemaFactorBindingState() == capability.state &&
		factor.schemaFactorSchema() == capability.state.schema && factor.schemaFactorOrdinal() == capability.ordinal
}

// FactorCapabilityForSemantic resolves one owner-issued Factor semantic in an
// already sealed binding. The semantic must be a member of that exact Schema;
// no Rule role or domain lane participates in the resolution.
func FactorCapabilityForSemantic(binding *SchemaBinding, semantic identity.SemanticKey) (FactorSlotCapability, bool) {
	state := bindingState(binding)
	if state == nil || state.phase != schemaBindingSealed || state.authority == nil ||
		state.schema == nil || !semantic.Available() {
		return FactorSlotCapability{}, false
	}
	ordinal, ok := state.schema.factorOrdinalOf(compositionKeyOf(semantic))
	capability := FactorSlotCapability{state: state, authority: state.authority, ordinal: ordinal}
	capability.available = ok && capability.completeRow()
	return capability, capability.available
}

func (capability FactorSlotCapability) semantic(state *schemaBindingState, authority *schemaBindingAuthority) (composition.Key, bool) {
	if !capability.Available() || capability.state != state || capability.authority != authority {
		return composition.Key{}, false
	}
	semantic := state.schema.factorSemanticAt(capability.ordinal)
	return semantic, semantic.Available()
}

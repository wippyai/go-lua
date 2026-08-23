package store

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free Program-storage Placement Rule shape.
// Value's exact source read and Placement's selected route read are declared
// together with the route write, so the engine owns their dependency and
// target correspondence.
type SchemaFragment struct {
	slot          *engine.RuleSlot[placementdomain.Fact, valuedomain.StorageTransfer]
	input         engine.SchemaInput
	valueRead     engine.SchemaReadSlot[valuedomain.Value]
	placementRead engine.SchemaReadSlot[placementdomain.Fact]
	carry         engine.SchemaCarrySlot[placementdomain.Fact]
	write         engine.SchemaWriteSlot[placementdomain.Fact]
	semantic      identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placementdomain.Fact, valuedomain.StorageTransfer] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records one Value exact-read/selected Placement route-write
// shape for a storage transfer occurrence.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, owner *placementowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placementdomain.Fact, valuedomain.StorageTransfer](builder, engine.SchemaRuleSpec[placementdomain.Fact]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	valueRead, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), input)
	if !ok {
		return nil, false
	}
	placementRead, ok := engine.SchemaSelectedRead[placementdomain.Fact](slot, owner.ExactRead(), input, valueRead.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, owner.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaRouteWrite(slot, owner.ExactWrite(), placementRead)
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, valueRead: valueRead, placementRead: placementRead, carry: carry, write: write, semantic: semantic}, true
}

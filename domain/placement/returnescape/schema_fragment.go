package returnescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free return-escape Rule shape. Value's root
// exact read authenticates the mounted boundary input; the dependent selected
// Value read carries the heterogeneous root/member rows, while Placement's
// selected read supplies the route-write predecessor geometry.
type SchemaFragment struct {
	slot          *engine.RuleSlot[placement.Fact, operand]
	input         engine.SchemaInput
	valueAnchor   engine.SchemaReadSlot[valuedomain.Value]
	valueRead     engine.SchemaReadSlot[valuedomain.Value]
	placementRead engine.SchemaReadSlot[placement.Fact]
	carry         engine.SchemaCarrySlot[placement.Fact]
	write         engine.SchemaWriteSlot[placement.Fact]
	semantic      identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placement.Fact, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the one-entry exact Value-read/selected Placement
// route-write Rule. The selected read and route write are declared together so
// the engine owns their dependency and target correspondence.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, owner *placementowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placement.Fact, operand](builder, engine.SchemaRuleSpec[placement.Fact]{
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
	valueAnchor, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), input)
	if !ok {
		return nil, false
	}
	valueRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, valueAnchor.Ref())
	if !ok {
		return nil, false
	}
	placementRead, ok := engine.SchemaSelectedRead[placement.Fact](slot, owner.ExactRead(), input, valueAnchor.Ref(), valueRead.Ref())
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
	return &SchemaFragment{slot: slot, input: input, valueAnchor: valueAnchor, valueRead: valueRead, placementRead: placementRead, carry: carry, write: write, semantic: semantic}, true
}

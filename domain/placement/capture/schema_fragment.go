package capture

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free mounted closure-capture Rule shape.
// The closure allocation's exact Placement is the parent containment value;
// the selected Value read resolves every captured outer storage cell; the
// selected Placement read supplies the routed write predecessors.
type SchemaFragment struct {
	slot       *engine.RuleSlot[placementdomain.Fact, operand]
	input      engine.SchemaInput
	parent     engine.SchemaReadSlot[placementdomain.Fact]
	values     engine.SchemaReadSlot[valuedomain.Value]
	placements engine.SchemaReadSlot[placementdomain.Fact]
	carry      engine.SchemaCarrySlot[placementdomain.Fact]
	write      engine.SchemaWriteSlot[placementdomain.Fact]
	semantic   identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placementdomain.Fact, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the exact parent/selected-source/selected-route
// geometry. Dependency edges are canonicalized by the engine from the parent
// and Value selected-read refs; the hot implementation cannot substitute a
// different predecessor order.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, owner *placementowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placementdomain.Fact, operand](builder, engine.SchemaRuleSpec[placementdomain.Fact]{
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
	parent, ok := engine.SchemaRead[placementdomain.Fact](slot, owner.ExactRead(), input)
	if !ok {
		return nil, false
	}
	valueRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, parent.Ref())
	if !ok {
		return nil, false
	}
	placements, ok := engine.SchemaSelectedRead[placementdomain.Fact](slot, owner.ExactRead(), input, parent.Ref(), valueRead.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, owner.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaRouteWrite(slot, owner.ExactWrite(), placements)
	if !ok {
		return nil, false
	}
	return &SchemaFragment{
		slot: slot, input: input, parent: parent, values: valueRead,
		placements: placements, carry: carry, write: write,
		semantic: semantic,
	}, true
}

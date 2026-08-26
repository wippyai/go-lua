package suspension

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free Value-aware Link Rule shape. The
// liveness catalog supplies the mounted operand; Call contributes the exact
// boundary fact that decides whether the subject crosses a yield at all,
// Value contributes an exact anchor followed by the neutral-to-atom selected
// read, Placement contributes the exact routed write surface, and no
// engine-level suspension policy is encoded here.
type SchemaFragment struct {
	slot          *engine.RuleSlot[placementdomain.Fact, operand]
	input         engine.SchemaInput
	callRead      engine.SchemaReadSlot[calldomain.Value]
	valueAnchor   engine.SchemaReadSlot[valuedomain.Value]
	valueRead     engine.SchemaReadSlot[valuedomain.Value]
	placementRead engine.SchemaReadSlot[placementdomain.Fact]
	carry         engine.SchemaCarrySlot[placementdomain.Fact]
	write         engine.SchemaWriteSlot[placementdomain.Fact]
	route         bool
	semantic      identity.SemanticKey
}

// DeclareSchema records the Call-gated, Value-selected, Placement-routed
// suspension shape. Value is a required principal: there is no direct-root
// compatibility shape. Call is a required principal too: the boundary a
// liveness row is anchored at is only a yield boundary when the call solved
// there can suspend, and no static approximation of that answer exists.
//
// A selected read is not a zero-predecessor read in the engine schema. The
// exact anchor is deliberately only structural: it gives the staged Value
// read a valid predecessor while the hot locator still chooses the complete
// authenticated source set for each operand. The anchor therefore does not
// authorize missing Value evidence and must not be used to narrow routes.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, calls *callowner.SchemaFragment, owner *placementowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
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
	callRead, ok := engine.SchemaRead[calldomain.Value](slot, calls.ExactRead(), input)
	if !ok {
		return nil, false
	}
	valueAnchor, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), input)
	if !ok {
		return nil, false
	}
	valueRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, callRead.Ref(), valueAnchor.Ref())
	if !ok {
		return nil, false
	}
	placementRead, ok := engine.SchemaSelectedRead[placementdomain.Fact](slot, owner.ExactRead(), input, callRead.Ref(), valueAnchor.Ref(), valueRead.Ref())
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
	return &SchemaFragment{slot: slot, input: input, callRead: callRead, valueAnchor: valueAnchor, valueRead: valueRead, placementRead: placementRead, carry: carry, write: write, route: true, semantic: semantic}, true
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placementdomain.Fact, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

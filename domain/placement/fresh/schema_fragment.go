package fresh

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

// SchemaFragment is Placement/fresh's callback-free zero-input Link Rule
// surface. The owner contributes the exact-write Factor form; fresh-root
// identity and Stack policy stay in this package's hot implementation.
type SchemaFragment struct {
	slot     *engine.RuleSlot[placement.Placement, operand]
	write    engine.SchemaWriteSlot[placement.Placement]
	semantic identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placement.Placement, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records Placement/fresh's zero-input exact-write Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, owner *placementowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placement.Placement, operand](builder, engine.SchemaRuleSpec[placement.Placement]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 0, Output: owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, owner.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, write: write, semantic: semantic}, true
}

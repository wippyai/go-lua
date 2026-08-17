package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is Value/bootstrap's callback-free Rule surface. Host
// mapping and the bootstrap checker remain hot Binding responsibilities.
type SchemaFragment struct {
	slot     *engine.RuleSlot[value.Value, identity.ContentID]
	write    engine.SchemaWriteSlot[value.Value]
	semantic identity.SemanticKey
	evidence identity.SemanticKey
}

// DeclareSchema records the zero-input Value bootstrap Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence identity.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, identity.ContentID](builder, engine.SchemaRuleSpec[value.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 0,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, owner.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, write: write, semantic: semantic, evidence: evidence}, true
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, identity.ContentID] {
	return fragment.slot
}

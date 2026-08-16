package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// SchemaFragment is Value/bootstrap's callback-free Rule surface. Host
// mapping and the bootstrap checker remain hot Binding responsibilities.
type SchemaFragment struct {
	slot     *engine.RuleSlot[value.Value, keyspace.ContentID]
	write    engine.SchemaWriteSlot[value.Value]
	semantic engine.SemanticKey
	evidence engine.SemanticKey
}

// DeclareSchema records the zero-input Value bootstrap Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence engine.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !distinct(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, keyspace.ContentID](builder, engine.SchemaRuleSpec[value.Value]{
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

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, keyspace.ContentID] {
	return fragment.slot
}

func distinct(keys ...engine.SemanticKey) bool {
	for index, key := range keys {
		if !key.Available() {
			return false
		}
		for _, prior := range keys[:index] {
			if prior == key {
				return false
			}
		}
	}
	return true
}

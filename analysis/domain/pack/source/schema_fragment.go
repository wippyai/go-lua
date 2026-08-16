package source

import (
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// SchemaFragment is Pack/source's callback-free Rule surface.
type SchemaFragment struct {
	slot     *engine.RuleSlot[packdomain.Value, packdomain.Source]
	write    engine.SchemaWriteSlot[packdomain.Value]
	semantic engine.SemanticKey
	evidence engine.SemanticKey
}

// DeclareSchema records the zero-input Pack source Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence engine.SemanticKey, owner *packowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !distinct(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[packdomain.Value, packdomain.Source](builder, engine.SchemaRuleSpec[packdomain.Value]{
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

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[packdomain.Value, packdomain.Source] {
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

package ingress

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// SchemaFragment is Heap-ingress's callback-free zero-input Rule surface.
type SchemaFragment struct {
	slot     *engine.RuleSlot[heapdomain.Value, source.Root]
	write    engine.SchemaWriteSlot[heapdomain.Value]
	semantic engine.SemanticKey
	evidence engine.SemanticKey
}

// DeclareSchema records the WorldZero ingress Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence engine.SemanticKey, owner *heapowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !distinct(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, source.Root](builder, engine.SchemaRuleSpec[heapdomain.Value]{
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

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, source.Root] {
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

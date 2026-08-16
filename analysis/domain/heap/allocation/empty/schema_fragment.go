package empty

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// SchemaFragment is Heap-empty's callback-free transformed Rule surface.
type SchemaFragment struct {
	slot      *engine.RuleSlot[heapdomain.Value, source.Root]
	input     engine.SchemaInput
	read      engine.SchemaReadSlot[heapdomain.Value]
	carry     engine.SchemaCarrySlot[heapdomain.Value]
	write     engine.SchemaWriteSlot[heapdomain.Value]
	semantic  engine.SemanticKey
	transform engine.SemanticKey
	evidence  engine.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, source.Root] {
	return fragment.slot
}

// DeclareSchema records Heap empty allocation's one-input transformed-carry
// Rule with one exact Heap read and write.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, transform, evidence engine.SemanticKey, owner *heapowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !distinct(semantic, operandFamily, transform, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, source.Root](builder, engine.SchemaRuleSpec[heapdomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	read, ok := engine.SchemaRead[heapdomain.Value](slot, owner.ExactRead(), input)
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarry(slot, input, owner.Ref(), transform)
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, owner.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, read: read, carry: carry, write: write, semantic: semantic, transform: transform, evidence: evidence}, true
}

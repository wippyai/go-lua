package suspension

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// EvidenceSchemaFragment is the dedicated Link-rule shape for the
// Heap-aligned suspension-evidence Factor. It intentionally mirrors the
// existing suspension Placement rule but names the independent evidence
// output receipt, so Placement class is never a source for this producer.
type EvidenceSchemaFragment struct {
	slot         *engine.RuleSlot[Evidence, operand]
	input        engine.SchemaInput
	valueAnchor  engine.SchemaReadSlot[valuedomain.Value]
	valueRead    engine.SchemaReadSlot[valuedomain.Value]
	evidenceRead engine.SchemaReadSlot[Evidence]
	carry        engine.SchemaCarrySlot[Evidence]
	write        engine.SchemaWriteSlot[Evidence]
	route        bool
}

func DeclareEvidenceSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, owner *EvidenceFactorFragment) (*EvidenceSchemaFragment, bool) {
	if builder == nil || values == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) || owner.Ref() == (engine.FactorRef[Evidence]{}) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[Evidence, operand](builder, engine.SchemaRuleSpec[Evidence]{
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
	evidenceRead, ok := engine.SchemaSelectedRead[Evidence](slot, owner.ExactRead(), input, valueAnchor.Ref(), valueRead.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, owner.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaRouteWrite(slot, owner.ExactWrite(), evidenceRead)
	if !ok {
		return nil, false
	}
	return &EvidenceSchemaFragment{slot: slot, input: input, valueAnchor: valueAnchor, valueRead: valueRead, evidenceRead: evidenceRead, carry: carry, write: write, route: true}, true
}

func (fragment *EvidenceSchemaFragment) RuleSlot() *engine.RuleSlot[Evidence, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

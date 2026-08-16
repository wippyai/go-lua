package callsite

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

type schemaFragment[O any] struct {
	slot     *engine.RuleSlot[effectfactor.Value, O]
	input    engine.SchemaInput
	callRead engine.SchemaReadSlot[calldomain.Value]
	write    engine.SchemaWriteSlot[effectfactor.Value]
	semantic engine.SemanticKey
	evidence engine.SemanticKey
}

// SelectedSchemaFragment and OpaqueSchemaFragment are disjoint cold
// capabilities despite their identical incidence. A hot binder therefore
// cannot attach selected semantics to the opaque semantic row or vice versa.
type SelectedSchemaFragment struct{ core *schemaFragment[hotOperand] }
type OpaqueSchemaFragment struct{ core *schemaFragment[hotOperand] }

func (fragment *SelectedSchemaFragment) RuleSlot() *engine.RuleSlot[effectfactor.Value, hotOperand] {
	if fragment == nil || fragment.core == nil {
		return nil
	}
	return fragment.core.slot
}
func (fragment *OpaqueSchemaFragment) RuleSlot() *engine.RuleSlot[effectfactor.Value, hotOperand] {
	if fragment == nil || fragment.core == nil {
		return nil
	}
	return fragment.core.slot
}
func (fragment *BodySchemaFragment) RuleSlot() *engine.RuleSlot[effectfactor.Value, hotBodyOperand] {
	if fragment == nil || fragment.core == nil {
		return nil
	}
	return fragment.core.slot
}

func DeclareSelectedSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence engine.SemanticKey, calls *callowner.SchemaFragment, effects *effectowner.SchemaFragment) (*SelectedSchemaFragment, bool) {
	core, ok := declareSchema[hotOperand](builder, semantic, operandFamily, evidence, calls, effects)
	if !ok {
		return nil, false
	}
	return &SelectedSchemaFragment{core: core}, true
}

func DeclareOpaqueSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence engine.SemanticKey, calls *callowner.SchemaFragment, effects *effectowner.SchemaFragment) (*OpaqueSchemaFragment, bool) {
	core, ok := declareSchema[hotOperand](builder, semantic, operandFamily, evidence, calls, effects)
	if !ok {
		return nil, false
	}
	return &OpaqueSchemaFragment{core: core}, true
}

func declareSchema[O any](builder *engine.SchemaBuilder, semantic, operandFamily, evidence engine.SemanticKey, calls *callowner.SchemaFragment, effects *effectowner.SchemaFragment) (*schemaFragment[O], bool) {
	if builder == nil || calls == nil || effects == nil || !engine.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[effectfactor.Value, O](builder, engine.SchemaRuleSpec[effectfactor.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence}, Output: effects.Ref(),
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
	write, ok := engine.SchemaWrite(slot, effects.ExactWrite())
	if !ok {
		return nil, false
	}
	return &schemaFragment[O]{slot: slot, input: input, callRead: callRead, write: write, semantic: semantic, evidence: evidence}, true
}

// BodySchemaFragment is the callback-free interprocedural Call-body Rule
// shape. Its Effect read is a selected projection dependent on the Call read.
type BodySchemaFragment struct {
	core       *schemaFragment[hotBodyOperand]
	effectRead engine.SchemaReadSlot[effectfactor.Value]
}

// DeclareBodySchema records BodyCall's exact Call-read, dependent selected
// Effect-read, and exact Effect-write incidence.
func DeclareBodySchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence engine.SemanticKey, calls *callowner.SchemaFragment, effects *effectowner.SchemaFragment) (*BodySchemaFragment, bool) {
	core, ok := declareSchema[hotBodyOperand](builder, semantic, operandFamily, evidence, calls, effects)
	if !ok {
		return nil, false
	}
	effectRead, ok := engine.SchemaSelectedRead[effectfactor.Value](core.slot, effects.ExactRead(), core.input, core.callRead.Ref())
	if !ok {
		return nil, false
	}
	return &BodySchemaFragment{core: core, effectRead: effectRead}, true
}

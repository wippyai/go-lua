package callsite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
)

type schemaFragment struct {
	slot     *engine.RuleSlot[effectfactor.Value, effectfactor.MountedCall]
	input    engine.SchemaInput
	callRead engine.SchemaReadSlot[calldomain.Value]
	write    engine.SchemaWriteSlot[effectfactor.Value]
	semantic identity.SemanticKey
}

func (fragment *BodySchemaFragment) RuleSlot() *engine.RuleSlot[effectfactor.Value, effectfactor.MountedCall] {
	if fragment == nil || fragment.core == nil {
		return nil
	}
	return fragment.core.slot
}

func declareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, calls *callowner.SchemaFragment, effects *effectowner.SchemaFragment) (*schemaFragment, bool) {
	if builder == nil || calls == nil || effects == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[effectfactor.Value, effectfactor.MountedCall](builder, engine.SchemaRuleSpec[effectfactor.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: effects.Ref(),
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
	return &schemaFragment{slot: slot, input: input, callRead: callRead, write: write, semantic: semantic}, true
}

// BodySchemaFragment is the callback-free interprocedural Call-body Rule
// shape. Its Effect read is a selected projection dependent on the Call read.
type BodySchemaFragment struct {
	core       *schemaFragment
	effectRead engine.SchemaReadSlot[effectfactor.Value]
}

// DeclareBodySchema records BodyCall's exact Call-read, dependent selected
// Effect-read, and exact Effect-write incidence.
func DeclareBodySchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, calls *callowner.SchemaFragment, effects *effectowner.SchemaFragment) (*BodySchemaFragment, bool) {
	core, ok := declareSchema(builder, semantic, operandFamily, calls, effects)
	if !ok {
		return nil, false
	}
	effectRead, ok := engine.SchemaSelectedRead[effectfactor.Value](core.slot, effects.ExactRead(), core.input, core.callRead.Ref())
	if !ok {
		return nil, false
	}
	return &BodySchemaFragment{core: core, effectRead: effectRead}, true
}

// Package targetresult declares Static's selected ordinary Target-call result
// projection.  The operand is Value's existing mounted CallResultSlot; Static
// contributes only its own typed-fact output.
package targetresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// SchemaFragment is the callback-free cold shape of the mounted result rule.
// It has no Value or Heap factor read: the mounted result row already carries
// the exact Value coordinate to which Static writes.
type SchemaFragment struct {
	slot     *engine.RuleSlot[staticdomain.TypeFact, valuedomain.MountedCallResultSlot]
	input    engine.SchemaInput
	callRead engine.SchemaReadSlot[calldomain.Value]
	write    engine.SchemaWriteSlot[staticdomain.TypeFact]
	semantic identity.SemanticKey
}

// RuleSlot returns the exact cold rule declaration for composition.
func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[staticdomain.TypeFact, valuedomain.MountedCallResultSlot] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records one exact Call read and one Static exact write.  The
// output factor is supplied solely by Static's owner fragment.
func DeclareSchema(
	builder *engine.SchemaBuilder,
	semantic, operandFamily identity.SemanticKey,
	statics *staticowner.SchemaFragment,
	calls *callowner.SchemaFragment,
) (*SchemaFragment, bool) {
	if builder == nil || statics == nil || calls == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[staticdomain.TypeFact, valuedomain.MountedCallResultSlot](builder, engine.SchemaRuleSpec[staticdomain.TypeFact]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1, Output: statics.Ref(),
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
	write, ok := engine.SchemaWrite(slot, statics.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, callRead: callRead, write: write, semantic: semantic}, true
}

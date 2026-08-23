// Package field declares Static's direct-field projection rule. The rule is
// deliberately narrow: its operand is Heap/index's existing mounted exact
// read geometry, while the only fact it reads and writes is Static's own
// TypeFact factor.
package field

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
)

// SchemaFragment is the callback-free cold shape of Static's direct-field
// transfer. The predecessor input carries Static's complete factor state;
// receiverRead selects the one receiver coordinate from that same state.
// The output remains Static-owned and is written at Index.Result's existing
// Value coordinate.
type SchemaFragment struct {
	slot         *engine.RuleSlot[staticdomain.TypeFact, heapindex.Index]
	input        engine.SchemaInput
	receiverRead engine.SchemaReadSlot[staticdomain.TypeFact]
	carry        engine.SchemaCarrySlot[staticdomain.TypeFact]
	write        engine.SchemaWriteSlot[staticdomain.TypeFact]
	semantic     identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[staticdomain.TypeFact, heapindex.Index] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// ReceiverRead is exposed only as the typed cold capability needed by the
// owner-local binder; callers cannot use it to construct a Static coordinate.
func (fragment *SchemaFragment) ReceiverRead() engine.SchemaReadSlot[staticdomain.TypeFact] {
	if fragment == nil {
		return engine.SchemaReadSlot[staticdomain.TypeFact]{}
	}
	return fragment.receiverRead
}

// DeclareSchema records one Static exact read, one ordinary Static carry, and
// one exact Static write. No Value or Heap factor read is declared: Index is
// already the sealed mounted geometry carrying the receiver/result Value
// coordinates and exact-key slot.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, statics *staticowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || statics == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[staticdomain.TypeFact, heapindex.Index](builder, engine.SchemaRuleSpec[staticdomain.TypeFact]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: statics.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	receiverRead, ok := engine.SchemaRead[staticdomain.TypeFact](slot, statics.ExactRead(), input)
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, statics.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, statics.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, receiverRead: receiverRead, carry: carry, write: write, semantic: semantic}, true
}

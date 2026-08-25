package containment

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

// SchemaFragment is the callback-free mounted-point containment Rule shape.
// Both predecessor reads are complete-vector summaries. The selected
// Placement read is the sole routed-write address source; its dependencies
// keep the two summary planes together in the cold schema.
type SchemaFragment struct {
	slot             *engine.RuleSlot[placement.Fact, Operand]
	input            engine.SchemaInput
	placementSummary engine.SchemaReadSlot[placement.Fact]
	heapSummary      engine.SchemaReadSlot[heapdomain.Value]
	routes           engine.SchemaReadSlot[placement.Fact]
	carry            engine.SchemaCarrySlot[placement.Fact]
	write            engine.SchemaWriteSlot[placement.Fact]
	placementRef     engine.FactorRef[placement.Fact]
	heapRef          engine.FactorRef[heapdomain.Value]
	semantic         identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placement.Fact, Operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the exact summary-read/selected-child-read/routed-write
// geometry. Dependencies are canonicalized by the engine from both summaries,
// so hot code cannot attach a different route predecessor.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, placementPrincipal *placementowner.SchemaFragment, heapPrincipal *heapowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || placementPrincipal == nil || heapPrincipal == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placement.Fact, Operand](builder, engine.SchemaRuleSpec[placement.Fact]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: placementPrincipal.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	placementSummary, ok := engine.SchemaRead[placement.Fact](slot, placementPrincipal.FoldSummaryRead(), input)
	if !ok {
		return nil, false
	}
	heapSummary, ok := engine.SchemaRead[heapdomain.Value](slot, heapPrincipal.SummaryRead(), input)
	if !ok {
		return nil, false
	}
	routes, ok := engine.SchemaSelectedRead[placement.Fact](slot, placementPrincipal.ExactRead(), input, placementSummary.Ref(), heapSummary.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, placementPrincipal.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaRouteWrite(slot, placementPrincipal.ExactWrite(), routes)
	if !ok {
		return nil, false
	}
	return &SchemaFragment{
		slot: slot, input: input, placementSummary: placementSummary, heapSummary: heapSummary,
		routes: routes, carry: carry, write: write, placementRef: placementPrincipal.Ref(),
		heapRef: heapPrincipal.Ref(), semantic: semantic,
	}, true
}

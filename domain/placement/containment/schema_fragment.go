package containment

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

// SchemaFragment is the callback-free Link-lane containment Rule shape. The
// two exact predecessor reads authenticate the parent coordinate and its
// complete Heap relation; the selected Placement read is the sole routed-write
// address source.
type SchemaFragment struct {
	slot            *engine.RuleSlot[placement.Placement, operand]
	input           engine.SchemaInput
	parentPlacement engine.SchemaReadSlot[placement.Placement]
	parentHeap      engine.SchemaReadSlot[heapdomainValue]
	routes          engine.SchemaReadSlot[placement.Placement]
	carry           engine.SchemaCarrySlot[placement.Placement]
	write           engine.SchemaWriteSlot[placement.Placement]
	placementRef    engine.FactorRef[placement.Placement]
	heapRef         engine.FactorRef[heapdomainValue]
	semantic        identity.SemanticKey
}

// heapdomainValue is a local alias used only to keep the field declarations
// readable while retaining Heap's actual Value type at every engine boundary.
// It is an alias (not a wrapper or second factor vocabulary).
type heapdomainValue = heapdomain.Value

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placement.Placement, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the exact parent-read/selected-child-read/routed-write
// geometry. Dependencies are canonicalized by the engine from the two exact
// reads, so hot code cannot attach a different route predecessor.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, placementPrincipal *placementowner.SchemaFragment, heapPrincipal *heapowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || placementPrincipal == nil || heapPrincipal == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placement.Placement, operand](builder, engine.SchemaRuleSpec[placement.Placement]{
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
	parentPlacement, ok := engine.SchemaRead[placement.Placement](slot, placementPrincipal.ExactRead(), input)
	if !ok {
		return nil, false
	}
	parentHeap, ok := engine.SchemaRead[heapdomainValue](slot, heapPrincipal.ExactRead(), input)
	if !ok {
		return nil, false
	}
	routes, ok := engine.SchemaSelectedRead[placement.Placement](slot, placementPrincipal.ExactRead(), input, parentPlacement.Ref(), parentHeap.Ref())
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
		slot: slot, input: input, parentPlacement: parentPlacement, parentHeap: parentHeap,
		routes: routes, carry: carry, write: write, placementRef: placementPrincipal.Ref(),
		heapRef: heapPrincipal.Ref(), semantic: semantic,
	}, true
}

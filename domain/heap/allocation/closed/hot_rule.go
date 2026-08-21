package closed

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Heap closed allocation's typed receipt-native Rule issuer. The
// operand is constructor-issued source.Closed; hot callbacks never invoke
// NewClosed, FieldOrigin, or any Link/Flow topology query.
type HotRule struct {
	implementation *heapowner.RuleImplementation[source.Closed]
	catalog        *allocationcatalog.Catalog
	heapOwner      *heapowner.HotOwner
	heap           heapdomain.Schema
	values         *valuedomain.Schema
}

// Implementation resolves the typed receipt only after SchemaBinding seals.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[source.Closed], bool) {
	if rule == nil || rule.heapOwner == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.heapOwner, rule.implementation)
	return rule.implementation, ok
}

// BindHot binds the exact heterogeneous Heap/Value read surface, ordinary
// carry, and exact Heap write through the two owner-issued FactorRefs.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, heapOwner *heapowner.HotOwner, valueOwner *valueowner.HotOwner, catalog *allocationcatalog.Catalog) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || heapOwner == nil || !heapOwner.MatchesBinding(binding) || valueOwner == nil || !valueOwner.MatchesBinding(binding) || valueOwner.Schema() == nil || !heapOwner.Schema().Valid() ||
		catalog == nil || !catalog.FencedTo(heapOwner.Schema(), valueOwner.Schema()) ||
		fragment.valueSummary.Kind() != engine.SchemaFormReadSummary {
		return nil, false
	}
	heapSchema, values := heapOwner.Schema(), valueOwner.Schema()
	// The sealed key/class projection is this Rule's one atom quotient. It is
	// built once per binding beside the schemas it is fenced to, never per
	// transfer row.
	projection, projectionOK := keymatch.NewSelectorProjection(heapSchema, values)
	if !projectionOK {
		return nil, false
	}
	rule := &HotRule{catalog: catalog, heapOwner: heapOwner, heap: heapSchema, values: values}
	var runtimeHeapRead engine.Read[engine.OrderedCells[heapdomain.Value]]
	var runtimeValueRead engine.Read[engine.OrderedCells[valuedomain.Value]]
	implementation, runtimeHeapRead, runtimeValueRead, ok := heapowner.BindExactAndSummaryReadAndCarry[source.Closed, heapdomain.Value, valuedomain.Value, engine.OrderedCells[valuedomain.Value]](
		heapOwner, fragment.slot, fragment.heapRead, heapOwner.FactorRef(), fragment.valueRead, valueOwner.FactorRef(), fragment.valueSummary,
		fragment.carry, fragment.write, engine.HotRuleSpec[heapdomain.Value, source.Closed]{
			OperandContent: func(candidate source.Closed) (source.Closed, [32]byte, bool) {
				return hotClosedContent(heapSchema, values, candidate)
			},
			OperandResolver: rule.resolveOperand,
			Fold: func(frame engine.Frame[heapdomain.Value, source.Closed]) engine.RuleResult[heapdomain.Value] {
				operand, operandOK := engine.Operand(frame)
				if !operandOK || !operand.FencedTo(heapSchema, values) {
					return engine.RuleResult[heapdomain.Value]{}
				}
				heapCells, heapOK := engine.ReadValue(frame, runtimeHeapRead)
				valueCells, valueOK := engine.ReadValue(frame, runtimeValueRead)
				if !heapOK || !valueOK || heapCells.Count() != 1 {
					return engine.RuleResult[heapdomain.Value]{}
				}
				predecessor, present, available := heapCells.At(0)
				if !available {
					return engine.RuleResult[heapdomain.Value]{}
				}
				if !present {
					return engine.NoCandidate(frame)
				}
				next, normal, resultOK := resultClosed(heapSchema, values, projection, operand, predecessor, valueCells)
				if !resultOK {
					return engine.RuleResult[heapdomain.Value]{}
				}
				if !normal {
					return engine.NoCandidate(frame)
				}
				return engine.Staged(frame, next)
			},
		}, engine.HotCarrySpec[heapdomain.Value, source.Closed]{
			Apply: func(operand source.Closed, prior heapdomain.Value) (heapdomain.Value, bool) {
				if !operand.FencedTo(heapSchema, values) {
					return heapdomain.Value{}, false
				}
				return heapSchema.Age(prior, operand.Key())
			},
		}, func(operand source.Closed) (uint64, bool) {
			index, ok := heapSchema.KeyIndex(operand.Key())
			return uint64(index), ok && index >= 0
		}, func(operand source.Closed) (uint64, bool) {
			index, ok := heapSchema.KeyIndex(operand.Key())
			return uint64(index), ok && index >= 0
		})
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (source.Closed, bool) {
	if rule == nil || rule.catalog == nil {
		return source.Closed{}, false
	}
	mount, mountOK := rule.catalog.ForMount(coords.Mount)
	closed, ok := mount.ClosedForOccurrence(coords.Occurrence)
	return closed, mountOK && ok && closed.FencedTo(rule.heap, rule.values)
}

func hotClosedContent(heapSchema heapdomain.Schema, values *valuedomain.Schema, candidate source.Closed) (source.Closed, [32]byte, bool) {
	if !candidate.FencedTo(heapSchema, values) {
		return source.Closed{}, [32]byte{}, false
	}
	id, ok := candidate.ID()
	if !ok || [32]byte(id) == ([32]byte{}) {
		return source.Closed{}, [32]byte{}, false
	}
	return candidate, [32]byte(id), true
}

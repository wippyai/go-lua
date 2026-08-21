package allocation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is Value/allocation's receipt-native transformed-carry Rule issuer.
// It retains only Value owner's typed issuer; operand provenance and transform
// callbacks remain package-private.
type HotRule struct {
	implementation *valueowner.RuleImplementation[operand]
	catalog        *allocationcatalog.Catalog
	owner          *valueowner.HotOwner
}

// BindHot binds the exact allocation fragment through Value's one-carry
// receipt lane. No legacy Rule or generic slot fallback is available.
func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner, heap heapdomain.Schema, catalog *allocationcatalog.Catalog) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		catalog == nil || !catalog.FencedTo(heap, owner.Schema()) ||
		!fragment.semantic.Available() || !fragment.transform.Available() ||
		!identity.DistinctKeys(fragment.semantic, fragment.transform) {
		return nil, false
	}
	schema := owner.Schema()
	rule := &HotRule{catalog: catalog, owner: owner}
	implementation, ok := valueowner.BindCarryRule(owner, fragment.slot, fragment.carry, fragment.write, engine.HotRuleSpec[value.Value, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return allocationOperandContentForSchema(schema, candidate)
		},
		Fold: func(frame engine.Frame[value.Value, operand]) engine.RuleResult[value.Value] {
			allocation, operandOK := engine.Operand(frame)
			if !operandOK || allocation.result == nil {
				return engine.RuleResult[value.Value]{}
			}
			fresh, freshOK := allocation.result.Fresh()
			if !freshOK {
				return engine.RuleResult[value.Value]{}
			}
			return engine.Staged(frame, fresh)
		},
		OperandResolver: rule.resolveOperand,
	}, engine.HotCarrySpec[value.Value, operand]{
		Apply: func(allocation operand, prior value.Value) (value.Value, bool) {
			if allocation.result == nil {
				return value.Value{}, false
			}
			return allocation.result.Age(prior)
		},
	}, func(allocation operand) (uint64, bool) {
		index, indexOK := schema.CoordinateIndex(allocation.coordinate)
		return uint64(index), indexOK
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.catalog == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return operand{}, false
	}
	mount, mountOK := rule.catalog.ForMount(coords.Mount)
	key, ok := mount.KeyForOccurrence(coords.Occurrence)
	if !mountOK || !ok {
		return operand{}, false
	}
	return allocationOperandForSchema(rule.owner.Schema(), key)
}

// Implementation returns the typed pending issuer until the shared binding
// seals, after which Value owner resolves it to the exact engine receipt.
func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

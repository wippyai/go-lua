package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// HotRule is Heap bootstrap's zero-input rule. Its operand is the owner-issued
// Heap Key; the complete immutable bootstrap Value is a sealed Heap Schema row
// and is read directly by the fold.
type HotRule struct {
	implementation *heapowner.RuleImplementation[heapdomain.Key]
	owner          *heapowner.HotOwner
}

// BindHot attaches the exact write implementation without reopening Host,
// Target, Link, or the BootEntry denominator.
func BindHot(fragment *SchemaFragment, owner *heapowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || !owner.Schema().Valid() ||
		!fragment.semantic.Available() {
		return nil, false
	}
	schema := owner.Schema()
	rule := &HotRule{owner: owner}
	implementation, ok := heapowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[heapdomain.Value, heapdomain.Key]{
		OperandContent: func(key heapdomain.Key) (heapdomain.Key, [32]byte, bool) {
			contentID, contentOK := schema.BootRootID(key)
			if !contentOK {
				return heapdomain.Key{}, [32]byte{}, false
			}
			return key, [32]byte(contentID), true
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[heapdomain.Value, heapdomain.Key]) engine.RuleResult[heapdomain.Value] {
			key, operandOK := engine.Operand(frame)
			value, valueOK := schema.BootValue(key)
			if !operandOK || !valueOK {
				return engine.RuleResult[heapdomain.Value]{}
			}
			return engine.Staged(frame, value)
		},
	}, func(key heapdomain.Key) (uint64, bool) {
		index, indexOK := schema.KeyIndex(key)
		return uint64(index), indexOK && index >= 0 && key.Kind() == heapdomain.RootBoot
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (heapdomain.Key, bool) {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() {
		return heapdomain.Key{}, false
	}
	return rule.owner.Schema().KeyForBootID(coords.Occurrence)
}

// Count implements the Link occurrence denominator directly from Heap's
// sealed BootRoot rows.
func (rule *HotRule) Count() int {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() {
		return 0
	}
	return rule.owner.Schema().BootCount()
}

// IDAt projects Heap's canonical BootRoot identity order for Link admission.
func (rule *HotRule) IDAt(index int) (identity.ContentID, bool) {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() {
		return identity.ContentID{}, false
	}
	return rule.owner.Schema().BootIDAt(index)
}

func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[heapdomain.Key], bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

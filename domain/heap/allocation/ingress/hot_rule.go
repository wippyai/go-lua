package ingress

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// HotRule is Heap allocation ingress's typed receipt-native rule vertical.
// It retains only Heap owner's opaque issuer; no legacy Rule, Composition,
// output slot, or private Heap coordinate crosses this package boundary.
type HotRule struct {
	implementation *heapowner.RuleImplementation[source.Root]
	owner          *heapowner.HotOwner
	catalog        *allocationcatalog.Catalog
}

// BindHot installs the zero-input exact-write ingress semantics for the exact
// cold Rule fragment. Heap owner alone attaches the output Factor binding.
func BindHot(fragment *SchemaFragment, owner *heapowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || !owner.Schema().Valid() ||
		!fragment.semantic.Available() {
		return nil, false
	}
	implementation, ok := heapowner.BindExactWriteRule(owner, fragment.slot, fragment.write, engine.HotRuleSpec[heapdomain.Value, source.Root]{
		// Root.New issued the complete cold classification receipt. Member,
		// fold path authenticates that receipt in O(1).
		OperandContent: func(operand source.Root) (source.Root, [32]byte, bool) {
			return ingressContent(owner.Schema(), operand)
		},
		Fold: func(frame engine.Frame[heapdomain.Value, source.Root]) engine.RuleResult[heapdomain.Value] {
			operand, operandOK := engine.Operand(frame)
			_, value, resultOK := ingressResult(owner.Schema(), operand)
			if !operandOK || !resultOK {
				return engine.RuleResult[heapdomain.Value]{}
			}
			return engine.Staged(frame, value)
		},
	}, func(operand source.Root) (uint64, bool) {
		index, ok := owner.Schema().KeyIndex(operand.Key())
		return uint64(index), ok && index >= 0
	})
	if !ok || implementation == nil {
		return nil, false
	}
	rule := &HotRule{implementation: implementation, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (source.Root, bool) {
	if rule == nil || rule.owner == nil || rule.catalog == nil {
		return source.Root{}, false
	}
	mount, mountOK := rule.catalog.ForMount(coords.Mount)
	root, ok := mount.RootForOccurrence(coords.Occurrence)
	return root, mountOK && ok && root.FencedTo(rule.owner.Schema())
}

// AttachCatalog attaches the exact Link-local allocation occurrence issuer to
// ingress. It is a cold substitution only; the ingress transfer remains
// receipt-native and never reconstructs an allocation from an artifact row.
func (rule *HotRule) AttachCatalog(catalog *allocationcatalog.Catalog) bool {
	if rule == nil || rule.owner == nil || catalog == nil || !catalog.FencedToHeap(rule.owner.Schema()) {
		return false
	}
	rule.catalog = catalog
	return true
}

func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[source.Root], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func ingressContent(schema heapdomain.Schema, operand source.Root) (source.Root, [32]byte, bool) {
	id, ok := operand.ID()
	if !ok || !operand.FencedTo(schema) {
		return source.Root{}, [32]byte{}, false
	}
	return operand, [32]byte(id), true
}

func ingressResult(schema heapdomain.Schema, operand source.Root) (heapdomain.Key, heapdomain.Value, bool) {
	if !schema.Valid() || !operand.FencedTo(schema) {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	value, ok := schema.EmptyObject(operand.Key())
	return operand.Key(), value, ok
}
